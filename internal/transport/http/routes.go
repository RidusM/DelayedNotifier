package httpt

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           Notification Service API
// @version         1.0
// @description     API для работы с уведомлениями
// @termsOfService  http://swagger.io/terms/
// @contact.name    RidusM
// @contact.email   stormkillpeople@gmail.com
// @license.name    MIT-0
// @license.url     https://github.com/aws/mit-0
// @host            localhost:8080
// @BasePath        /
func (h *Handler) setupRoutes() {
	h.router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	h.router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})

	orders := h.router.Group("/orders")
	{
		orders.GET("/:order_uid", h.getHandler)
	}

	h.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}