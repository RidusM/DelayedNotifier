package httpt

import (
	"github.com/gin-gonic/gin"
	"github.com/ridusm/delayednotifier/pkg/logger"
	"github.com/ridusm/delayednotifier/pkg/metric"
)

type Handler struct {
	svc     *service.Notification
	log     logger.Logger
	metrics metric.HTTP
	router  *gin.Engine
}

func NewNotificationHandler(
	svc *service.Notification,
	log logger.Logger,
	metrics metric.HTTP,
) *Handler {
	h := &Handler{
		svc:     svc,
		log:     log,
		metrics: metrics,
	}

	router := gin.New()

	router.Use(h.requestIDMiddleware())
	router.Use(h.loggingMiddleware())
	router.Use(gin.Recovery())

	h.router = router

	h.router.LoadHTMLGlob("web/*.html")
	h.router.Static("/static", "./web")

	h.setupRoutes()

	return h
}

func (h *OrderHandler) Engine() *gin.Engine {
	return h.router
}