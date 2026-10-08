package handler

import (
	"net/http"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/ariwalapratham/go-payment-simulator/internal/middleware"
	"github.com/ariwalapratham/go-payment-simulator/internal/model"
	"github.com/ariwalapratham/go-payment-simulator/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// MerchantHandler serves GET/PATCH /v1/merchant/me.
type MerchantHandler struct {
	merchants *service.MerchantService
	log       zerolog.Logger
}

// NewMerchantHandler wires GET/PATCH /v1/merchant/me.
func NewMerchantHandler(merchants *service.MerchantService, log *zerolog.Logger) *MerchantHandler {
	h := &MerchantHandler{merchants: merchants, log: zerolog.Nop()}
	if log != nil {
		h.log = *log
	}
	return h
}

// GetMe handles GET /v1/merchant/me. Secrets are omitted.
func (h *MerchantHandler) GetMe(c *gin.Context) {
	log := h.reqLog(c)
	merchantID := middleware.MerchantIDFrom(c)
	if h.merchants == nil {
		notImplemented(c)
		return
	}

	merchant, err := h.merchants.Get(c.Request.Context(), merchantID)
	if err != nil {
		log.Warn().Err(err).Str("merchant_id", merchantID.String()).Msg("get merchant me failed")
		middleware.AbortWithError(c, httpMerchantErr(err))
		return
	}

	log.Info().Str("merchant_id", merchant.PublicID.String()).Msg("get merchant me")
	c.JSON(http.StatusOK, toMerchantResponse(merchant))
}

// PatchMe handles PATCH /v1/merchant/me. Only webhook_url is applied; name is ignored.
func (h *MerchantHandler) PatchMe(c *gin.Context) {
	log := h.reqLog(c)
	merchantID := middleware.MerchantIDFrom(c)

	var req model.UpdateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Str("merchant_id", merchantID.String()).Msg("patch merchant me invalid json")
		middleware.AbortWithError(c, errs.ValidationError(err))
		return
	}
	if req.WebhookURL == nil {
		log.Warn().Str("merchant_id", merchantID.String()).Msg("patch merchant me missing webhook_url")
		middleware.AbortWithError(c, errs.NewBadRequestError("webhook_url is required", true, nil, nil, nil))
		return
	}
	if h.merchants == nil {
		notImplemented(c)
		return
	}

	merchant, err := h.merchants.UpdateWebhookURL(c.Request.Context(), merchantID, req.WebhookURL)
	if err != nil {
		log.Warn().Err(err).Str("merchant_id", merchantID.String()).Msg("patch merchant me failed")
		middleware.AbortWithError(c, httpMerchantErr(err))
		return
	}

	log.Info().Str("merchant_id", merchant.PublicID.String()).Msg("patch merchant me")
	c.JSON(http.StatusOK, toMerchantResponse(merchant))
}

func (h *MerchantHandler) reqLog(c *gin.Context) zerolog.Logger {
	return h.log.With().Str("request_id", middleware.GetRequestID(c)).Logger()
}

func registerMerchantMeRoutes(rg *gin.RouterGroup, mw *middleware.Middlewares, h *MerchantHandler) {
	if h == nil {
		h = NewMerchantHandler(nil, nil)
	}
	g := rg.Group("/merchant")
	g.Use(mw.APIKey)
	g.GET("/me", h.GetMe)
	g.PATCH("/me", h.PatchMe)
}
