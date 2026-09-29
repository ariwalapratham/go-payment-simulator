package middleware

import (
	"strings"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderMerchantID     = "X-Merchant-Id"

	ctxMerchantID     = "merchant_id"
	ctxIdempotencyKey = "idempotency_key"
)

func RequireMerchantID() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader(HeaderMerchantID))
		if raw == "" {
			AbortWithError(c, errs.NewBadRequestError("X-Merchant-Id header is required", true, nil, nil, nil))
			return
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			AbortWithError(c, errs.NewBadRequestError("invalid X-Merchant-Id", true, nil, nil, nil))
			return
		}
		c.Set(ctxMerchantID, id)
		c.Next()
	}
}

func RequireIdempotencyKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader(HeaderIdempotencyKey))
		if key == "" {
			AbortWithError(c, errs.NewBadRequestError("Idempotency-Key header is required", true, nil, nil, nil))
			return
		}
		c.Set(ctxIdempotencyKey, key)
		c.Next()
	}
}

func MerchantIDFrom(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ctxMerchantID)
	id, _ := v.(uuid.UUID)
	return id
}

func IdempotencyKeyFrom(c *gin.Context) string {
	v, _ := c.Get(ctxIdempotencyKey)
	key, _ := v.(string)
	return key
}
