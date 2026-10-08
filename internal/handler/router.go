package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/server"
	"github.com/gin-gonic/gin"
)

const readyzTimeout = 2 * time.Second

// NewRouter mounts /v1 routes and the shared middleware chain.
func NewRouter(
	s *server.Server,
	mw *middleware.Middlewares,
	payments *PaymentHandler,
	refunds *RefundHandler,
	admin *AdminMerchantHandler,
	merchants *MerchantHandler,
) *gin.Engine {
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

	r.GET("/healthz", healthz)
	r.GET("/readyz", readyz(s))

	v1 := r.Group("/v1")
	v1.GET("/health", healthz)
	registerAdminMerchantRoutes(v1, mw, admin)
	registerMerchantMeRoutes(v1, mw, merchants)
	registerPaymentRoutes(v1, mw, payments, refunds)
	registerRefundRoutes(v1, mw, refunds)

	return r
}

// healthz is GET /healthz and GET /v1/health (FR32). No dependency checks.
func healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// readyz is GET /readyz (FR33). 200 if the database answers, 503 otherwise.
func readyz(s *server.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		if s == nil || s.DB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "error": "database unreachable"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), readyzTimeout)
		defer cancel()
		if err := s.DB.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "error": "database unreachable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
