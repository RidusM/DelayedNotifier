package handler

import (
	"errors"
	"net/http"

	"delayednotifier/internal/entity"

	"github.com/gin-gonic/gin"
)

func (h *NotifyHandler) handleServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, entity.ErrDataNotFound):
		h.respondError(c, http.StatusNotFound, "not_found",
			"Data not found", err)
	case errors.Is(err, entity.ErrInvalidData):
		h.respondError(c, http.StatusBadRequest, "invalid_data",
			"Invalid input data", err)
	case errors.Is(err, entity.ErrConflictingData):
		h.respondError(c, http.StatusConflict, "conflict",
			"Data conflict occurred", err)
	case errors.Is(err, entity.ErrNotificationAlreadySent):
		h.respondError(c, http.StatusConflict, "already_sent",
			"Cannot cancel: notification has already been sent", err)
	case errors.Is(err, entity.ErrNotificationCancelled):
		h.respondError(c, http.StatusConflict, "already_cancelled",
			"Notification is already cancelled", err)
	case errors.Is(err, entity.ErrRecipientNotFound):
		h.respondError(c, http.StatusNotFound, "recipient_not_found",
			"Recipient identifier not found for this user", err)
	default:
		h.respondError(c, http.StatusInternalServerError, "internal_error",
			"Internal server error occurred", err)
	}
}
