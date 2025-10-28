package httpt

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ridusm/delayednotifier/internal/entity"
	"github.com/ridusm/delayednotifier/pkg/logger"
)

// пока заглушка с l0
func (h *Handler) handleServiceError(c *gin.Context, err error, op string) {
	log := h.log.Ctx(c.Request.Context())

	log.LogAttrs(c.Request.Context(), logger.ErrorLevel, op+" failed",
		logger.Any("error", err),
		logger.String("remote_addr", c.ClientIP()),
		logger.String("user_agent", c.Request.UserAgent()),
	)

	switch {
	case errors.Is(err, entity.ErrConfigPathNotSet):
		c.JSON(
			http.StatusBadRequest,
			gin.H{"error": "Invalid notification data. Check send data."},
		)
	case errors.Is(err, entity.ErrConfigPathNotSet):
		log.LogAttrs(c.Request.Context(), logger.WarnLevel, "order not found",
			logger.String("order_uid", c.Param("order_uid")),
			logger.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusNotFound, gin.H{"error": "Order not found"})
	case errors.Is(err, context.DeadlineExceeded):
		log.LogAttrs(c.Request.Context(), logger.WarnLevel, "request timeout",
			logger.String("path", c.Request.URL.Path),
			logger.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "Request timed out"})
	default:
		log.LogAttrs(c.Request.Context(), logger.ErrorLevel, "internal server error",
			logger.Any("error", err),
			logger.String("path", c.Request.URL.Path),
			logger.String("client_ip", c.ClientIP()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal service error"})
	}
}

func (h *Handler) handleInvalidUUID(c *gin.Context, op, value string) {
	log := h.log.Ctx(c.Request.Context())

	log.LogAttrs(c.Request.Context(), logger.WarnLevel, "invalid notification UUID format",
		logger.String("op", op),
		logger.String("value", value),
		logger.String("remote_addr", c.ClientIP()),
	)

	c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid notification UUID format"})
}