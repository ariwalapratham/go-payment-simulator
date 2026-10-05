package handler

import (
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

// AdminMerchantHandler serves /v1/admin/merchants.
type AdminMerchantHandler struct {
	merchants *service.MerchantService
	log       zerolog.Logger
}

// NewAdminMerchantHandler wires /v1/admin/merchants.
func NewAdminMerchantHandler(merchants *service.MerchantService, log *zerolog.Logger) *AdminMerchantHandler {
	h := &AdminMerchantHandler{merchants: merchants, log: zerolog.Nop()}
	if log != nil {
		h.log = *log
	}
	return h
}

// Create handles POST /v1/admin/merchants. api_key and webhook_secret appear only on 201.
func (h *AdminMerchantHandler) Create(c *gin.Context) {
	log := h.reqLog(c)

	var req model.CreateMerchantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("create merchant invalid json")
		middleware.AbortWithError(c, errs.ValidationError(err))
		return
	}
	if h.merchants == nil {
		notImplemented(c)
		return
	}

	created, err := h.merchants.Create(c.Request.Context(), req.Name, req.WebhookURL)
	if err != nil {
		log.Warn().Err(err).Msg("create merchant failed")
		middleware.AbortWithError(c, httpMerchantErr(err))
		return
	}

	log.Info().Str("merchant_id", created.PublicID.String()).Msg("create merchant")
	c.JSON(http.StatusCreated, model.MerchantCreatedResponse{
		ID:            created.PublicID.String(),
		Name:          created.Name,
		WebhookURL:    created.WebhookURL,
		APIKey:        created.APIKey,
		WebhookSecret: created.WebhookSecret,
		CreatedAt:     created.CreatedAt,
	})
}

// List handles GET /v1/admin/merchants. Secrets are omitted.
func (h *AdminMerchantHandler) List(c *gin.Context) {
	log := h.reqLog(c)
	if h.merchants == nil {
		notImplemented(c)
		return
	}

	merchants, err := h.merchants.List(c.Request.Context())
	if err != nil {
		log.Warn().Err(err).Msg("list merchants failed")
		middleware.AbortWithError(c, httpMerchantErr(err))
		return
	}

	data := make([]model.MerchantResponse, 0, len(merchants))
	for i := range merchants {
		data = append(data, toMerchantResponse(merchants[i]))
	}
	log.Info().Int("count", len(data)).Msg("list merchants")
	c.JSON(http.StatusOK, model.MerchantListResponse{Data: data})
}

// Get handles GET /v1/admin/merchants/:id.
func (h *AdminMerchantHandler) Get(c *gin.Context) {
	log := h.reqLog(c)

	id, err := parseMerchantPublicID(c)
	if err != nil {
		log.Warn().Str("merchant_id", c.Param("id")).Msg("get merchant invalid id")
		middleware.AbortWithError(c, err)
		return
	}
	if h.merchants == nil {
		notImplemented(c)
		return
	}

	merchant, err := h.merchants.Get(c.Request.Context(), id)
	if err != nil {
		log.Warn().Err(err).Str("merchant_id", id.String()).Msg("get merchant failed")
		middleware.AbortWithError(c, httpMerchantErr(err))
		return
	}

	log.Info().Str("merchant_id", merchant.PublicID.String()).Msg("get merchant")
	c.JSON(http.StatusOK, toMerchantResponse(merchant))
}

// Update handles PATCH /v1/admin/merchants/:id.
func (h *AdminMerchantHandler) Update(c *gin.Context) {
	log := h.reqLog(c)

	id, err := parseMerchantPublicID(c)
	if err != nil {
		log.Warn().Str("merchant_id", c.Param("id")).Msg("update merchant invalid id")
		middleware.AbortWithError(c, err)
		return
	}

	var req model.UpdateMerchantRequest
	err = c.ShouldBindJSON(&req)
	if err != nil {
		log.Warn().Err(err).Str("merchant_id", id.String()).Msg("update merchant invalid json")
		middleware.AbortWithError(c, errs.ValidationError(err))
		return
	}
	if h.merchants == nil {
		notImplemented(c)
		return
	}

	merchant, err := h.merchants.Update(c.Request.Context(), id, req.Name, req.WebhookURL, req.WebhookURL != nil)
	if err != nil {
		log.Warn().Err(err).Str("merchant_id", id.String()).Msg("update merchant failed")
		middleware.AbortWithError(c, httpMerchantErr(err))
		return
	}

	log.Info().Str("merchant_id", merchant.PublicID.String()).Msg("update merchant")
	c.JSON(http.StatusOK, toMerchantResponse(merchant))
}

// RotateKey handles POST /v1/admin/merchants/:id/rotate-key. The new api_key appears once.
func (h *AdminMerchantHandler) RotateKey(c *gin.Context) {
	log := h.reqLog(c)

	id, err := parseMerchantPublicID(c)
	if err != nil {
		log.Warn().Str("merchant_id", c.Param("id")).Msg("rotate merchant key invalid id")
		middleware.AbortWithError(c, err)
		return
	}
	if h.merchants == nil {
		notImplemented(c)
		return
	}

	publicID, apiKey, err := h.merchants.RotateAPIKey(c.Request.Context(), id)
	if err != nil {
		log.Warn().Err(err).Str("merchant_id", id.String()).Msg("rotate merchant key failed")
		middleware.AbortWithError(c, httpMerchantErr(err))
		return
	}

	log.Info().Str("merchant_id", publicID.String()).Msg("rotate merchant key")
	c.JSON(http.StatusOK, model.RotateMerchantKeyResponse{
		ID:     publicID.String(),
		APIKey: apiKey,
	})
}

func (h *AdminMerchantHandler) reqLog(c *gin.Context) zerolog.Logger {
	return h.log.With().Str("request_id", middleware.GetRequestID(c)).Logger()
}

func registerAdminMerchantRoutes(rg *gin.RouterGroup, mw *middleware.Middlewares, h *AdminMerchantHandler) {
	if h == nil {
		h = NewAdminMerchantHandler(nil, nil)
	}
	g := rg.Group("/admin")
	g.Use(mw.AdminAuth)
	g.POST("/merchants", h.Create)
	g.GET("/merchants", h.List)
	g.GET("/merchants/:id", h.Get)
	g.PATCH("/merchants/:id", h.Update)
	g.POST("/merchants/:id/rotate-key", h.RotateKey)
}

func parseMerchantPublicID(c *gin.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return uuid.Nil, errs.NewBadRequestError("invalid merchant id", true, nil, nil, nil)
	}
	return id, nil
}

func toMerchantResponse(m service.Merchant) model.MerchantResponse {
	return model.NewMerchantResponse(m.PublicID.String(), m.Name, m.WebhookURL, m.CreatedAt)
}

func httpMerchantErr(err error) error {
	switch {
	case errors.Is(err, service.ErrMerchantNotFound):
		return errs.NewNotFoundError("merchant not found", true, nil)
	case errors.Is(err, service.ErrInvalidMerchantName):
		return errs.NewBadRequestError("name is required", true, nil, nil, nil)
	case errors.Is(err, service.ErrInvalidWebhookURL):
		return errs.NewBadRequestError("webhook_url must be a public http or https URL", true, nil, nil, nil)
	case errors.Is(err, service.ErrWebhookURLTooLong):
		return errs.NewBadRequestError("webhook_url is too long", true, nil, nil, nil)
	case errors.Is(err, service.ErrMerchantNameTooLong):
		return errs.NewBadRequestError("name is too long", true, nil, nil, nil)
	case errors.Is(err, service.ErrNoMerchantUpdate):
		return errs.NewBadRequestError("at least one field is required", true, nil, nil, nil)
	case errors.Is(err, service.ErrMerchantKeyRotated):
		return errs.NewConflictError("merchant_key_rotated", "merchant api key was rotated concurrently")
	default:
		return sqlerr.HandleError(err)
	}
}
