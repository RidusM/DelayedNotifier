package handler

import (
	"net/http"

	_ "delayednotifier/docs" // required for Swagger

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           Notification Service API
// @version         1.0
// @description     API for working with notifications
// @termsOfService  http://swagger.io/terms/
// @contact.name    RidusM
// @contact.email   stormkillpeople@gmail.com
// @license.name    MIT-0
// @license.url     https://github.com/aws/mit-0
// @host            localhost:8080
// @BasePath        /
func (h *NotifyHandler) setupRoutes() {
	h.router.GET("/health", h.Health)

	users := h.router.Group("/users")
	{
		users.POST("", h.RegisterUser)
		users.POST("/:user_id/link-token", h.GenerateLinkToken)
	}

	notify := h.router.Group("/notify")
	{
		notify.POST("", h.CreateNotification)
		notify.GET("/:id", h.GetStatus)
		notify.DELETE("/:id", h.CancelNotification)
	}

	h.router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})
	h.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
