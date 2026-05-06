package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wb-go/wbf/logger"
)

func (h *NotifyHandler) requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := logger.GenerateRequestID()
		ctx := logger.SetRequestID(c.Request.Context(), reqID)
		c.Request = c.Request.WithContext(ctx)

		c.Header("X-Request-ID", reqID)

		c.Next()
	}
}

func (h *NotifyHandler) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		method := c.Request.Method
		path := c.Request.URL.Path

		h.log.LogAttrs(c.Request.Context(), logger.InfoLevel, "HTTP request processed",
			logger.String("method", method),
			logger.String("path", path),
			logger.Int("status", statusCode),
			logger.Duration("duration", latency),
			logger.String("client_ip", c.ClientIP()),
		)
	}
}

func (h *NotifyHandler) baseCORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().
			Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Request-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, DELETE")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
