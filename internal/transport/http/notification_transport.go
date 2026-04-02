package httpt

import (
	"net/http"

	"delayednotifier/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/wb-go/wbf/logger"
)

const _maxRequestBodySize = 1 << 20

type NotifyHandler struct {
	svc    *service.NotifyService
	log    logger.Logger
	router *gin.Engine
}

func NewNotifyHandler(
	svc *service.NotifyService,
	log logger.Logger,
) *NotifyHandler {
	h := &NotifyHandler{
		svc: svc,
		log: log,
	}

	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, _maxRequestBodySize)
	})

	router.Use(h.requestIDMiddleware())
	router.Use(h.loggingMiddleware())
	router.Use(gin.Recovery())

	h.router = router

	h.router.LoadHTMLGlob("web/*.html")
	h.router.Static("/static", "./web")

	h.setupRoutes()

	return h
}

func (h *NotifyHandler) Engine() *gin.Engine {
	return h.router
}
