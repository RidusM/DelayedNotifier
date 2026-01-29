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
	log := h.log.Ctx(ctx)

	switch {
	case errors.Is(err, entity.ErrNotificationNotFound):
		log.LogAttrs(ctx, logger.WarnLevel, "notification not found",
			logger.String("op", op),
			logger.Any("error", err),
		)
		h.respondError(c, http.StatusNotFound, "not_found", "Notification not found", err)

	case errors.Is(err, entity.ErrNotificationAlreadySent):
		log.LogAttrs(ctx, logger.WarnLevel, "notification already sent",
			logger.String("op", op),
			logger.Any("error", err),
		)
		h.respondError(c, http.StatusConflict, "already_sent",
			"Cannot cancel: notification has already been sent", err)

	case errors.Is(err, entity.ErrNotificationCancelled):
		log.LogAttrs(ctx, logger.WarnLevel, "notification already cancelled",
			logger.String("op", op),
			logger.Any("error", err),
		)
		h.respondError(c, http.StatusConflict, "already_cancelled",
			"Notification is already cancelled", err)

	case errors.Is(err, entity.ErrInvalidData):
		log.LogAttrs(ctx, logger.WarnLevel, "invalid data",
			logger.String("op", op),
			logger.Any("error", err),
		)
		h.respondError(c, http.StatusBadRequest, "invalid_data", "Invalid input data", err)

	case errors.Is(err, entity.ErrRecipientNotFound):
		log.LogAttrs(ctx, logger.WarnLevel, "recipient not found",
			logger.String("op", op),
			logger.Any("error", err),
		)
		h.respondError(c, http.StatusBadRequest, "recipient_not_found",
			"Recipient identifier not found for this user", err)

	case errors.Is(err, entity.ErrConflictingData):
		log.LogAttrs(ctx, logger.WarnLevel, "conflicting data",
			logger.String("op", op),
			logger.Any("error", err),
		)
		h.respondError(c, http.StatusConflict, "conflict", "Data conflict occurred", err)

	default:
		log.LogAttrs(ctx, logger.ErrorLevel, "internal server error",
			logger.String("op", op),
			logger.Any("error", err),
		)
		h.respondError(c, http.StatusInternalServerError, "internal_error",
			"Internal server error occurred", err)
	}
}

func isValidChannel(channel entity.Channel) bool {
	return channel.IsValid()
}
