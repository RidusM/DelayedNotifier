// nolint: revive,staticcheck
package httpt

import (
	"fmt"
	"net/http"
	"time"

	"delayednotifier/internal/entity"
	"delayednotifier/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	var req CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_request", "Invalid JSON format", err)
		return
	}

	channel := entity.Channel(req.Channel)
	if !channel.IsValid() {
		h.respondError(c, http.StatusBadRequest, "invalid_channel",
			"Channel must be one of: telegram, email", nil)
		return
	}

	if req.ScheduledAt.IsZero() {
		h.respondError(c, http.StatusBadRequest, "invalid_scheduled_at", "Scheduled time is required", nil)
		return
	}

	serviceReq := service.CreateNotificationRequest{
		Channel:     channel,
		Payload:     req.Payload,
		Recipient:   req.Recipient,
		ScheduledAt: req.ScheduledAt.UTC(),
	}

	notificationID, err := h.svc.Create(ctx, serviceReq)
	if err != nil {
		h.handleServiceError(c, op, err)
		return
	}

	response := CreateNotificationResponse{
		ID:          notificationID,
		Channel:     req.Channel,
		Recipient:   req.Recipient,
		Payload:     req.Payload,
		ScheduledAt: req.ScheduledAt,
		Message:     "Notification created successfully",
	}

	c.Header("Location", fmt.Sprintf("/notify/%s", notificationID))
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

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_id", "Invalid notification ID format", err)
		return
	}

	notification, err := h.svc.GetStatus(ctx, id)
	if err != nil {
		h.handleServiceError(c, op, err)
		return
	}

	response := NotificationStatusResponse{
		ID:          notification.ID.String(),
		Channel:     string(notification.Channel),
		Status:      notification.Status.String(),
		Payload:     notification.Payload,
		ScheduledAt: notification.ScheduledAt,
		SentAt:      notification.SentAt,
		RetryCount:  notification.RetryCount,
		LastError:   notification.LastError,
		CreatedAt:   notification.CreatedAt,
	}

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

	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(c, http.StatusBadRequest, "invalid_id", "Invalid notification ID format", err)
		return
	}

	if cancelErr := h.svc.Cancel(ctx, id); cancelErr != nil {
		h.handleServiceError(c, op, cancelErr)
		return
	}

	response := SuccessResponse{
		Message: "Notification cancelled successfully",
	}
	h.respondJSON(c, http.StatusOK, response)
}

func (h *NotifyHandler) Health(c *gin.Context) {
	response := map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	}
	h.respondJSON(c, http.StatusOK, response)
}

func (h *NotifyHandler) respondJSON(c *gin.Context, status int, data any) {
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
