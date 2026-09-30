package handler

import "github.com/gin-gonic/gin"

// registerRefundRoutes mounts GET /v1/refunds/:id (501).
func registerRefundRoutes(rg *gin.RouterGroup) {
	rg.GET("/refunds/:id", notImplemented)
}
