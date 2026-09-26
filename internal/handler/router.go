package handler

import (
	"net/http"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/server"
	"github.com/gin-gonic/gin"
)

func NewRouter(s *server.Server, mw *middleware.Middlewares) *gin.Engine {
	if s.Config.Primary.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		mw.RequestID,
		mw.Tracing.NewRelicMiddleware(),
		mw.Tracing.EnhanceTracing(),
		mw.Recovery,
		mw.Logger,
		mw.Errors,
	)

	v1 := r.Group("/v1")
	v1.GET("/health", health)
	registerPaymentRoutes(v1)
	registerRefundRoutes(v1)

	return r
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
