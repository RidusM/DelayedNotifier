package httpt

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/ridusm/delayednotifier/pkg/logger"
)

const (
	_defaultContextTimeout = 500 * time.Millisecond
)

// @Summary Получить статус уведомления
// @Description Возвращает статус уведомления по уникальному идентификатору
// @Tags Notification
// @Accept json
// @Produce json
// @Param id path string true "Уникальный идентификатор уведомления"
// @Success 200 {object} entity.Notification "Успешный ответ с статусом уведомления"
// @Failure 400 {object} httpt.ErrorResponse "Неверный формат notify_id"
// @Failure 404 {object} httpt.ErrorResponse "Уведомление не найдено"
// @Failure 500 {object} httpt.ErrorResponse "Внутренняя ошибка сервера"
// @Router /notify/{id} [get]
func (h *Handler) getNotifyHandler(c *gin.Context) {
	const op = "transport.getNotifyHandler"

	log := h.log.Ctx(c.Request.Context())
	notifyUIDStr := c.Param("notify_uid")

	notifyUID, err := uuid.Parse(notifyUIDStr)
	if err != nil {
		h.handleInvalidUUID(c, op, notifyUIDStr)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), _defaultContextTimeout)
	defer cancel()

	notify, err := h.svc.GetNotify(ctx, notifyUID)
	if err != nil {
		h.handleServiceError(c, err, op)
		return
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notify retrieved successfully",
		logger.String("notify_uid", notifyUIDStr),
	)

	c.JSON(http.StatusOK, notify)
}

func (h *Handler) postNotifyHandler(c *gin.Context) {
	const op = "transport.postNotifyHandler"

	log := h.log.Ctx(c.Request.Context())
	notifyUIDStr := c.Param("notify_uid")

	notifyUID, err := uuid.Parse(notifyUIDStr)
	if err != nil {
		h.handleInvalidUUID(c, op, notifyUIDStr)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), _defaultContextTimeout)
	defer cancel()

	notify, err := h.svc.CreateNotify(ctx, notifyUID)
	if err != nil {
		h.handleServiceError(c, err, op)
		return
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notify retrieved successfully",
		logger.String("notify_uid", notifyUIDStr),
	)

	c.JSON(http.StatusOK, notify)
}

func (h *Handler) deleteNotifyHandler(c *gin.Context) {
	const op = "transport.http.deleteNotifyHandler"

	log := h.log.Ctx(c.Request.Context())
	notifyUIDStr := c.Param("notify_uid")

	notifyUID, err := uuid.Parse(notifyUIDStr)
	if err != nil{
		h.handleInvalidUUID(c, op, notifyUIDStr)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), _defaultContextTimeout)
	defer cancel()

	notify, err := h.svc.DeleteNotify(ctx, notifyUID)
	if err != nil{
		h.handleServiceError(c, err, op)
		return
	}

	log.LogAttrs(ctx, logger.InfoLevel, "notify deleted successfully",
		logger.String("notify_uid", notifyUIDStr),
	)

	c.JSON(http.StatusOK, notify)
}