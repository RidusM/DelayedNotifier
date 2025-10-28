package httpt

import (
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title           Order Service API
// @version         1.0
// @description     API для управления заказами
// @termsOfService  http://swagger.io/terms/
// @contact.name    RidusM
// @contact.email   stormkillpeople@gmail.com
// @license.name    MIT-0
// @license.url     https://github.com/aws/mit-0
// @host            localhost:8080
// @BasePath        /
func (h *OrderHandler) setupRoutes() {
	h.router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	h.router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})

	orders := h.router.Group("/orders")
	{
		orders.GET("/:order_uid", h.getOrderHandler)
	}

	h.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}