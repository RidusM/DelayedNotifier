package httpt

import (
	"context"
	"delayednotifier/internal/entity"
	"delayednotifier/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/wb-go/wbf/logger"
)

type NotifyService interface {
	Create(ctx context.Context, req CreateNotificationRequest) (*entity.Notification, error)
	GetStatus(ctx context.Context, id uuid.UUID) (*entity.Notification, error)
	Cancel(ctx context.Context, id uuid.UUID) error
}

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
