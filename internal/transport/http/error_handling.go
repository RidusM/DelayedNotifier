package httpt

import (
	"errors"
	"net/http"

	"delayednotifier/internal/entity"

	"github.com/gin-gonic/gin"
	"github.com/wb-go/wbf/logger"
)

func (h *NotifyHandler) handleServiceError(c *gin.Context, op string, err error) {
	ctx := c.Request.Context()
	log := h.log.Ctx(ctx).With("op", op, "error", err)

	switch {
	case errors.Is(err, entity.ErrNotificationNotFound):
		log.LogAttrs(ctx, logger.WarnLevel, "notification not found")
		h.respondError(c, http.StatusNotFound, "not_found",
			"Notification not found", nil)

	case errors.Is(err, entity.ErrNotificationAlreadySent):
		log.LogAttrs(ctx, logger.WarnLevel, "notification already sent")
		h.respondError(c, http.StatusConflict, "already_sent",
			"Cannot cancel: notification has already been sent", nil)

	case errors.Is(err, entity.ErrNotificationCancelled):
		log.LogAttrs(ctx, logger.WarnLevel, "notification already cancelled")
		h.respondError(c, http.StatusConflict, "already_cancelled",
			"Notification is already cancelled", nil)

	case errors.Is(err, entity.ErrInvalidData):
		log.LogAttrs(ctx, logger.WarnLevel, "invalid data")
		h.respondError(c, http.StatusBadRequest, "invalid_data",
			"Invalid input data", nil)

	case errors.Is(err, entity.ErrRecipientNotFound):
		log.LogAttrs(ctx, logger.WarnLevel, "recipient not found")
		h.respondError(c, http.StatusNotFound, "recipient_not_found",
			"Recipient identifier not found for this user", nil)

	case errors.Is(err, entity.ErrConflictingData):
		log.LogAttrs(ctx, logger.WarnLevel, "conflicting data")
		h.respondError(c, http.StatusConflict, "conflict",
			"Data conflict occurred", nil)

	default:
		log.LogAttrs(ctx, logger.ErrorLevel, "internal server error")
		h.respondError(c, http.StatusInternalServerError, "internal_error",
			"Internal server error occurred", nil)
	}
}
