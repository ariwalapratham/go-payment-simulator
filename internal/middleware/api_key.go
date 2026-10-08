package middleware

import (
	"context"
	"strings"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// HeaderAPIKey is the merchant API key header for payments, refunds, and /v1/merchant/me.
const HeaderAPIKey = "X-Api-Key" //nolint:gosec // G101: HTTP header name, not a secret

// APIKeyLookup resolves a plaintext merchant API key to the merchant public id.
type APIKeyLookup func(ctx context.Context, apiKey string) (uuid.UUID, error)

// RequireAPIKey reads X-Api-Key, looks up the merchant, and stores the public id
// in context. The plaintext key is never logged. X-Merchant-Id is rejected.
func RequireAPIKey(lookup APIKeyLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader(HeaderMerchantID)) != "" {
			AbortWithError(c, errs.NewBadRequestError("X-Merchant-Id is not accepted; use X-Api-Key", true, nil, nil, nil))
			return
		}
		key := strings.TrimSpace(c.GetHeader(HeaderAPIKey))
		if key == "" {
			AbortWithError(c, errs.NewUnauthorizedError("API key is required", true))
			return
		}
		if lookup == nil {
			AbortWithError(c, errs.NewUnauthorizedError("invalid API key", true))
			return
		}
		id, err := lookup(c.Request.Context(), key)
		if err != nil {
			AbortWithError(c, errs.NewUnauthorizedError("invalid API key", true))
			return
		}
		c.Set(ctxMerchantID, id)
		c.Next()
	}
}
