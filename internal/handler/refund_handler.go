package handler

import "github.com/gin-gonic/gin"

func registerRefundRoutes(rg *gin.RouterGroup) {
	rg.GET("/refunds/:id", notImplemented)
}
