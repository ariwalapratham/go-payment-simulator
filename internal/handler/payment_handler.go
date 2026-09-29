package handler

import (
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/ariwalapratham/go-payment-simulator/internal/sqlerr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const currencyLen = 3

type PaymentHandler struct {
	payments *service.PaymentService
	log      zerolog.Logger
}

func NewPaymentHandler(payments *service.PaymentService, log *zerolog.Logger) *PaymentHandler {
	h := &PaymentHandler{payments: payments, log: zerolog.Nop()}
	if log != nil {
		h.log = *log
	}
	return h
}

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

func (h *PaymentHandler) Get(c *gin.Context) {
	log := h.reqLog(c)
	merchantID := middleware.MerchantIDFrom(c)

	paymentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Warn().Str("merchant_id", merchantID.String()).Str("payment_id", c.Param("id")).Msg("get payment invalid id")
		middleware.AbortWithError(c, errs.NewBadRequestError("invalid payment id", true, nil, nil, nil))
		return
	}

	if h.payments == nil {
		notImplemented(c)
		return
	}

	payment, err := h.payments.Get(c.Request.Context(), merchantID, paymentID)
	if err != nil {
		log.Warn().Err(err).
			Str("merchant_id", merchantID.String()).
			Str("payment_id", paymentID.String()).
			Msg("get payment failed")
		middleware.AbortWithError(c, httpPaymentErr(err))
		return
	}

	resp := toPaymentResponse(payment)
	log.Info().
		Str("merchant_id", merchantID.String()).
		Str("payment_id", resp.ID).
		Str("status", resp.Status).
		Msg("get payment")
	c.JSON(http.StatusOK, resp)
}

func (h *PaymentHandler) reqLog(c *gin.Context) zerolog.Logger {
	return h.log.With().Str("request_id", middleware.GetRequestID(c)).Logger()
}

func registerPaymentRoutes(rg *gin.RouterGroup, mw *middleware.Middlewares, h *PaymentHandler) {
	if h == nil {
		h = NewPaymentHandler(nil, nil)
	}
	g := rg.Group("/payments")
	g.Use(mw.MerchantID)
	g.POST("", mw.IdempotencyKey, h.Create)
	g.GET("/:id", h.Get)
	g.POST("/:id/capture", notImplemented)
	g.POST("/:id/cancel", notImplemented)
	g.POST("/:id/refund", notImplemented)
}

func validateCreateRequest(req model.CreatePaymentRequest) error {
	if req.Amount <= 0 {
		return errs.NewBadRequestError("amount must be greater than 0", true, nil, nil, nil)
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if len(currency) != currencyLen || !isAlpha(currency) {
		return errs.NewBadRequestError("currency must be a 3-letter code", true, nil, nil, nil)
	}
	return nil
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func toPaymentResponse(p service.Payment) model.PaymentResponse {
	return model.NewPaymentResponse(
		p.PublicID, p.MerchantPublicID, p.Amount, p.Currency, p.Status, p.CreatedAt, p.UpdatedAt,
	)
}

func httpPaymentErr(err error) error {
	switch {
	case errors.Is(err, service.ErrMerchantNotFound):
		return errs.NewNotFoundError("merchant not found", true, nil)
	case errors.Is(err, service.ErrPaymentNotFound):
		return errs.NewNotFoundError("payment not found", true, nil)
	case errors.Is(err, service.ErrIdempotencyKeyReused):
		return errs.NewUnprocessableEntityError(
			"idempotency_key_reused_with_different_payload",
			"idempotency key reused with a different payload",
		)
	case errors.Is(err, model.ErrInvalidTransition):
		return errs.NewConflictError("invalid_state_transition", err.Error())
	default:
		return sqlerr.HandleError(err)
	}
}

func notImplemented(c *gin.Context) {
	middleware.AbortWithError(c, errs.NewNotImplementedError())
}
