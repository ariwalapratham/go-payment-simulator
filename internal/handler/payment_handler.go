package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/ariwalapratham/go-payment-simulator/internal/sqlerr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type PaymentHandler struct {
	payments *service.PaymentService
	log      zerolog.Logger
}

// NewPaymentHandler wires POST/GET /v1/payments plus capture and cancel.
func NewPaymentHandler(payments *service.PaymentService, log *zerolog.Logger) *PaymentHandler {
	h := &PaymentHandler{payments: payments, log: zerolog.Nop()}
	if log != nil {
		h.log = *log
	}
	return h
}

// Create handles POST /v1/payments. 201 on first insert, 200 on idempotent replay.
func (h *PaymentHandler) Create(c *gin.Context) {
	log := h.reqLog(c)
	merchantID := middleware.MerchantIDFrom(c)
	idempotencyKey := middleware.IdempotencyKeyFrom(c)

	var req model.CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Str("merchant_id", merchantID.String()).Msg("create payment invalid json")
		middleware.AbortWithError(c, errs.ValidationError(err))
		return
	}
	if err := validateCreateRequest(req); err != nil {
		log.Warn().Err(err).
			Str("merchant_id", merchantID.String()).
			Int64("amount", req.Amount).
			Str("currency", req.Currency).
			Msg("create payment invalid body")
		middleware.AbortWithError(c, err)
		return
	}

	if h.payments == nil {
		notImplemented(c)
		return
	}

	payment, created, err := h.payments.Create(c.Request.Context(), merchantID, idempotencyKey, req.Amount, req.Currency)
	if err != nil {
		log.Warn().Err(err).
			Str("merchant_id", merchantID.String()).
			Str("idempotency_key", idempotencyKey).
			Msg("create payment failed")
		middleware.AbortWithError(c, httpPaymentErr(err))
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	resp := toPaymentResponse(payment)
	log.Info().
		Str("merchant_id", merchantID.String()).
		Str("payment_id", resp.ID).
		Str("idempotency_key", idempotencyKey).
		Int64("amount", resp.Amount).
		Str("currency", resp.Currency).
		Bool("replay", !created).
		Int("status", status).
		Msg("create payment")
	c.JSON(status, resp)
}

// Get handles GET /v1/payments/:id for the merchant authenticated by X-Api-Key.
func (h *PaymentHandler) Get(c *gin.Context) {
	h.loadPayment(c, "get payment", h.payments.Get)
}

// Capture handles POST /v1/payments/:id/capture. 200 CAPTURED, 409 if not AUTHORIZED.
func (h *PaymentHandler) Capture(c *gin.Context) {
	h.loadPayment(c, "capture payment", h.payments.Capture)
}

// Cancel handles POST /v1/payments/:id/cancel. 200 CANCELLED, 409 if not PENDING or AUTHORIZED.
func (h *PaymentHandler) Cancel(c *gin.Context) {
	h.loadPayment(c, "cancel payment", h.payments.Cancel)
}

func (h *PaymentHandler) loadPayment(
	c *gin.Context,
	action string,
	fn func(context.Context, uuid.UUID, uuid.UUID) (service.Payment, error),
) {
	log := h.reqLog(c)
	merchantID := middleware.MerchantIDFrom(c)

	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Warn().Str("merchant_id", merchantID.String()).Str("payment_id", c.Param("id")).Msg(action + " invalid id")
		middleware.AbortWithError(c, errs.NewBadRequestError("invalid payment id", true, nil, nil, nil))
		return
	}

	if h.payments == nil {
		notImplemented(c)
		return
	}

	payment, err := fn(c.Request.Context(), merchantID, paymentID)
	if err != nil {
		log.Warn().Err(err).
			Str("merchant_id", merchantID.String()).
			Str("payment_id", paymentID.String()).
			Msg(action + " failed")
		middleware.AbortWithError(c, httpPaymentErr(err))
		return
	}

	resp := toPaymentResponse(payment)
	log.Info().
		Str("merchant_id", merchantID.String()).
		Str("payment_id", resp.ID).
		Str("status", resp.Status).
		Msg(action)
	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) reqLog(c *gin.Context) zerolog.Logger {
	return h.log.With().Str("request_id", middleware.GetRequestID(c)).Logger()
}

// registerPaymentRoutes mounts POST/GET /v1/payments plus capture, cancel, and refund.
func registerPaymentRoutes(rg *gin.RouterGroup, mw *middleware.Middlewares, h *PaymentHandler, refunds *RefundHandler) {
	if h == nil {
		h = NewPaymentHandler(nil, nil)
	}
	if refunds == nil {
		refunds = NewRefundHandler(nil, nil)
	}
	g := rg.Group("/payments")
	g.Use(mw.APIKey)
	g.POST("", mw.IdempotencyKey, h.Create)
	g.GET("/:id", h.Get)
	g.POST("/:id/capture", h.Capture)
	g.POST("/:id/cancel", h.Cancel)
	g.POST("/:id/refund", mw.IdempotencyKey, refunds.Create)
}

// validateCreateRequest checks amount > 0 and a currency on the allowlist (FR29–30).
func validateCreateRequest(req model.CreatePaymentRequest) error {
	if req.Amount <= 0 {
		return errs.NewInvalidFieldError("INVALID_AMOUNT", "amount must be a positive integer", "amount")
	}
	if _, ok := model.NormalizeCurrency(req.Currency); !ok {
		return errs.NewInvalidFieldError("INVALID_CURRENCY", "currency is not supported", "currency")
	}
	return nil
}

func toPaymentResponse(p service.Payment) model.PaymentResponse {
	return model.NewPaymentResponse(
		p.PublicID, p.MerchantPublicID, p.Amount, p.Currency, p.Status, p.CreatedAt, p.UpdatedAt,
	)
}

// httpPaymentErr maps service sentinels to the API error envelope (404/422/409).
func httpPaymentErr(err error) error {
	switch {
	case errors.Is(err, service.ErrMerchantNotFound):
		return errs.NewNotFoundError("merchant not found", true, nil)
	case errors.Is(err, service.ErrPaymentNotFound):
		return errs.NewNotFoundError("payment not found", true, nil)
	case errors.Is(err, service.ErrRefundNotFound):
		return errs.NewNotFoundError("refund not found", true, nil)
	case errors.Is(err, service.ErrIdempotencyKeyReused):
		return errs.NewUnprocessableEntityError(
			"idempotency_key_reused_with_different_payload",
			"idempotency key reused with a different payload",
		)
	case errors.Is(err, service.ErrRefundExceedsBalance):
		return errs.NewUnprocessableEntityError(
			"refund_exceeds_balance",
			"refund amount exceeds remaining captured balance",
		)
	case errors.Is(err, model.ErrInvalidTransition):
		return errs.NewConflictError("invalid_state_transition", err.Error())
	default:
		return sqlerr.HandleError(err)
	}
}

// notImplemented is 501 when a handler is constructed without a service (unit router tests).
func notImplemented(c *gin.Context) {
	middleware.AbortWithError(c, errs.NewNotImplementedError())
}
