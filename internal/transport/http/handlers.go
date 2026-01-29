// nolint: revive,staticcheck
package httpt

import (
	"encoding/json"
	"net/http"
	"time"

	"delayednotifier/internal/entity"
	"delayednotifier/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/wb-go/wbf/logger"
)

// @Summary Создать уведомление
// @Description Планирует отправку уведомления на указанное время
// @Tags Notification
// @Accept json
// @Produce json
// @Param request body CreateNotificationRequest true "Данные уведомления"
// @Success 201 {object} CreateNotificationResponse "Уведомление создано"
// @Failure 400 {object} ErrorResponse "Ошибка валидации"
// @Failure 500 {object} ErrorResponse "Внутренняя ошибка"
// @Router /notify [post]
func (h *NotifyHandler) CreateNotification(c *gin.Context) {
	const op = "transport.http.NotifyHandler.CreateNotification"
	ctx := c.Request.Context()
	log := h.log.Ctx(ctx)

	log.LogAttrs(ctx, logger.InfoLevel, "create notification request received",
		logger.String("op", op),
		logger.String("method", c.Request.Method),
		logger.String("path", c.Request.URL.Path),
		logger.String("remote_addr", c.Request.RemoteAddr),
	)

	var req CreateNotificationRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "invalid request body",
			logger.String("op", op),
			logger.Any("error", err),
		)
		h.respondError(c, http.StatusBadRequest, "invalid_request", "Invalid JSON format", err)
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "invalid user_id",
			logger.String("op", op),
			logger.String("user_id", req.UserID),
		)
		h.respondError(c, http.StatusBadRequest, "invalid_user_id", "User ID must be a valid UUID", err)
		return
	}

	channel := entity.Channel(req.Channel)
	if !isValidChannel(channel) {
		log.LogAttrs(ctx, logger.ErrorLevel, "invalid channel",
			logger.String("op", op),
			logger.String("channel", req.Channel),
		)
		h.respondError(c, http.StatusBadRequest, "invalid_channel",
			"Channel must be one of: telegram, email, sms, push", nil)
		return
	}

	if req.Payload == "" {
		h.respondError(c, http.StatusBadRequest, "empty_payload", "Payload cannot be empty", nil)
		return
	}

	if req.ScheduledAt.IsZero() {
		h.respondError(c, http.StatusBadRequest, "invalid_scheduled_at", "Scheduled time is required", nil)
		return
	}

	serviceReq := service.CreateNotificationRequest{
		UserID:      userID,
		Channel:     channel,
		Payload:     req.Payload,
		ScheduledAt: req.ScheduledAt,
	}

	notification, err := h.svc.Create(ctx, serviceReq)
	if err != nil {
		h.handleServiceError(c, op, err)
		return
	}

	response := CreateNotificationResponse{
		ID:          notification.ID.String(),
		UserID:      notification.UserID.String(),
		Channel:     string(notification.Channel),
		Status:      notification.Status.String(),
		ScheduledAt: notification.ScheduledAt,
		CreatedAt:   notification.CreatedAt,
		Message:     "Notification created successfully",
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notification created successfully",
		logger.String("op", op),
		logger.String("id", response.ID),
		logger.String("channel", response.Channel),
	)

	h.respondJSON(c, http.StatusCreated, response)
}

// @Summary Получить статус уведомления
// @Description Возвращает статус уведомления по уникальному идентификатору
// @Tags Notification
// @Accept json
// @Produce json
// @Param id path string true "Уникальный идентификатор уведомления"
// @Success 200 {object} NotificationStatusResponse "Успешный ответ с статусом уведомления"
// @Failure 400 {object} ErrorResponse "Неверный формат notify_id"
// @Failure 404 {object} ErrorResponse "Уведомление не найдено"
// @Failure 500 {object} ErrorResponse "Внутренняя ошибка сервера"
// @Router /notify/{id} [get]
func (h *NotifyHandler) GetStatus(c *gin.Context) {
	const op = "transport.http.NotifyHandler.GetStatus"
	ctx := c.Request.Context()
	log := h.log.Ctx(ctx)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "invalid notification id",
			logger.String("op", op),
			logger.String("id", idStr),
		)
		h.respondError(c, http.StatusBadRequest, "invalid_id", "Invalid notification ID format", err)
		return
	}

	log.LogAttrs(ctx, logger.InfoLevel, "get status request received",
		logger.String("op", op),
		logger.String("id", id.String()),
	)

	notification, err := h.svc.GetStatus(ctx, id)
	if err != nil {
		h.handleServiceError(c, op, err)
		return
	}

	response := NotificationStatusResponse{
		ID:          notification.ID.String(),
		UserID:      notification.UserID.String(),
		Channel:     string(notification.Channel),
		Status:      notification.Status.String(),
		Payload:     notification.Payload,
		ScheduledAt: notification.ScheduledAt,
		SentAt:      notification.SentAt,
		RetryCount:  notification.RetryCount,
		LastError:   notification.LastError,
		CreatedAt:   notification.CreatedAt,
	}

	log.LogAttrs(ctx, logger.InfoLevel, "status retrieved successfully",
		logger.String("op", op),
		logger.String("id", id.String()),
		logger.String("status", notification.Status.String()),
	)

	h.respondJSON(c, http.StatusOK, response)
}

// @Summary Отменить уведомление
// @Description Отменяет запланированное уведомление
// @Tags Notification
// @Accept json
// @Produce json
// @Param id path string true "Уникальный идентификатор уведомления"
// @Success 200 {object} SuccessResponse "Успешный ответ"
// @Failure 400 {object} ErrorResponse "Неверный формат notify_id"
// @Failure 404 {object} ErrorResponse "Уведомление не найдено"
// @Failure 409 {object} ErrorResponse "Уведомление уже отправлено или отменено"
// @Failure 500 {object} ErrorResponse "Внутренняя ошибка сервера"
// @Router /notify/{id} [delete]
func (h *NotifyHandler) Cancel(c *gin.Context) {
	const op = "transport.http.NotifyHandler.Cancel"
	ctx := c.Request.Context()
	log := h.log.Ctx(ctx)

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		log.LogAttrs(ctx, logger.ErrorLevel, "invalid notification id",
			logger.String("op", op),
			logger.String("id", idStr),
		)
		h.respondError(c, http.StatusBadRequest, "invalid_id", "Invalid notification ID format", err)
		return
	}

	log.LogAttrs(ctx, logger.InfoLevel, "cancel request received",
		logger.String("op", op),
		logger.String("id", id.String()),
	)

	if cancelErr := h.svc.Cancel(ctx, id); cancelErr != nil {
		h.handleServiceError(c, op, cancelErr)
		return
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notification cancelled successfully",
		logger.String("op", op),
		logger.String("id", id.String()),
	)

	response := SuccessResponse{
		Message: "Notification cancelled successfully",
	}
	h.respondJSON(c, http.StatusOK, response)
}

// Health обрабатывает GET /health
func (h *NotifyHandler) Health(c *gin.Context) {
	response := map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	}
	h.respondJSON(c, http.StatusOK, response)
}

func (h *NotifyHandler) respondJSON(c *gin.Context, status int, data interface{}) {
	c.JSON(status, data)
}

func (h *NotifyHandler) respondError(c *gin.Context, status int, code, message string, err error) {
	response := ErrorResponse{
		Error: message,
		Code:  code,
	}
	if err != nil {
		response.Details = err.Error()
	}
	h.respondJSON(c, status, response)
}
