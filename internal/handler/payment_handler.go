package handler

import (
	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/gin-gonic/gin"
)

func registerPaymentRoutes(rg *gin.RouterGroup) {
	rg.POST("/payments", notImplemented)
	rg.GET("/payments/:id", notImplemented)
	rg.POST("/payments/:id/capture", notImplemented)
	rg.POST("/payments/:id/cancel", notImplemented)
	rg.POST("/payments/:id/refund", notImplemented)
}

func notImplemented(c *gin.Context) {
	middleware.AbortWithError(c, errs.NewNotImplementedError())
}
