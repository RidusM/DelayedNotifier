package handler

import (
	"context"
	"net/http"

	"delayednotifier/internal/config"
	"delayednotifier/internal/entity"
	"delayednotifier/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/wb-go/wbf/logger"
)

const _maxRequestBodySize = 1 << 20

type NotifyService interface {
	RegisterUser(ctx context.Context, req service.RegisterUserRequest) (*entity.User, error)
	GenerateLinkToken(ctx context.Context, userID uuid.UUID) (string, error)
	LinkTelegramByToken(ctx context.Context, token string, chatID *int64) error
	GetUserByTelegramID(ctx context.Context, chatID *int64) (*entity.User, error)
	CreateNotify(ctx context.Context, req service.CreateNotificationRequest) (uuid.UUID, error)
	GetStatus(ctx context.Context, id uuid.UUID) (*entity.Notification, error)
	Cancel(ctx context.Context, id uuid.UUID) error
}

type NotifyHandler struct {
	svc    NotifyService
	log    logger.Logger
	router *gin.Engine

	botCfg config.TG
}

func NewNotifyHandler(
	svc NotifyService,
	log logger.Logger,
	botCfg config.TG,
) *NotifyHandler {
	h := &NotifyHandler{
		svc:    svc,
		log:    log,
		botCfg: botCfg,
	}

	router := gin.New()

	router.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, _maxRequestBodySize)
	})

	router.Use(h.requestIDMiddleware())
	router.Use(h.loggingMiddleware())
	router.Use(h.baseCORSMiddleware())
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
