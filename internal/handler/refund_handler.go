package handler

import (
	"net/http"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type RefundHandler struct {
	refunds *service.RefundService
	log     zerolog.Logger
}

// NewRefundHandler wires POST /v1/payments/:id/refund and GET /v1/refunds/:id.
func NewRefundHandler(refunds *service.RefundService, log *zerolog.Logger) *RefundHandler {
	h := &RefundHandler{refunds: refunds, log: zerolog.Nop()}
	if log != nil {
		h.log = *log
	}
	return h
}

// Create handles POST /v1/payments/:id/refund. 201 on first insert, 200 on idempotent replay.
func (h *RefundHandler) Create(c *gin.Context) {
	log := h.reqLog(c)
	merchantID := middleware.MerchantIDFrom(c)
	idempotencyKey := middleware.IdempotencyKeyFrom(c)

	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Warn().Str("merchant_id", merchantID.String()).Str("payment_id", c.Param("id")).Msg("create refund invalid payment id")
		middleware.AbortWithError(c, errs.NewBadRequestError("invalid payment id", true, nil, nil, nil))
		return
	}

	var req model.CreateRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Str("merchant_id", merchantID.String()).Msg("create refund invalid json")
		middleware.AbortWithError(c, errs.ValidationError(err))
		return
	}
	if req.Amount <= 0 {
		log.Warn().Int64("amount", req.Amount).Str("merchant_id", merchantID.String()).Msg("create refund invalid amount")
		middleware.AbortWithError(c, errs.NewBadRequestError("amount must be greater than 0", true, nil, nil, nil))
		return
	}

	if h.refunds == nil {
		notImplemented(c)
		return
	}

	refund, created, err := h.refunds.Create(c.Request.Context(), merchantID, paymentID, idempotencyKey, req.Amount)
	if err != nil {
		log.Warn().Err(err).
			Str("merchant_id", merchantID.String()).
			Str("payment_id", paymentID.String()).
			Str("idempotency_key", idempotencyKey).
			Msg("create refund failed")
		middleware.AbortWithError(c, httpPaymentErr(err))
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	resp := toRefundResponse(refund)
	log.Info().
		Str("merchant_id", merchantID.String()).
		Str("payment_id", resp.PaymentID).
		Str("refund_id", resp.ID).
		Int64("amount", resp.Amount).
		Str("idempotency_key", idempotencyKey).
		Bool("replay", !created).
		Int("status", status).
		Msg("create refund")
	c.JSON(status, resp)
}

// Get handles GET /v1/refunds/:id for the merchant authenticated by X-Api-Key.
func (h *RefundHandler) Get(c *gin.Context) {
	log := h.reqLog(c)
	merchantID := middleware.MerchantIDFrom(c)

	refundID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Warn().Str("merchant_id", merchantID.String()).Str("refund_id", c.Param("id")).Msg("get refund invalid id")
		middleware.AbortWithError(c, errs.NewBadRequestError("invalid refund id", true, nil, nil, nil))
		return
	}

	if h.refunds == nil {
		notImplemented(c)
		return
	}

	refund, err := h.refunds.Get(c.Request.Context(), merchantID, refundID)
	if err != nil {
		log.Warn().Err(err).
			Str("merchant_id", merchantID.String()).
			Str("refund_id", refundID.String()).
			Msg("get refund failed")
		middleware.AbortWithError(c, httpPaymentErr(err))
		return
	}

	resp := toRefundResponse(refund)
	log.Info().
		Str("merchant_id", merchantID.String()).
		Str("refund_id", resp.ID).
		Str("payment_id", resp.PaymentID).
		Str("status", resp.Status).
		Msg("get refund")
	c.JSON(http.StatusOK, resp)
}

func (h *RefundHandler) reqLog(c *gin.Context) zerolog.Logger {
	return h.log.With().Str("request_id", middleware.GetRequestID(c)).Logger()
}

func toRefundResponse(r service.Refund) model.RefundResponse {
	return model.NewRefundResponse(r.PublicID, r.PaymentPublicID, r.Amount, r.Status, r.CreatedAt)
}

// registerRefundRoutes mounts GET /v1/refunds/:id.
func registerRefundRoutes(rg *gin.RouterGroup, mw *middleware.Middlewares, h *RefundHandler) {
	if h == nil {
		h = NewRefundHandler(nil, nil)
	}
	g := rg.Group("/refunds")
	g.Use(mw.APIKey)
	g.GET("/:id", h.Get)
}
