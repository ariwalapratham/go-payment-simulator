package middleware

import (
	"crypto/sha256"
	"crypto/subtle"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/gin-gonic/gin"
)

// HeaderAdminKey is the shared operator secret header for /v1/admin routes.
const HeaderAdminKey = "X-Admin-Key"

// AdminAuth requires X-Admin-Key to match the configured operator secret.
// An empty expected key rejects every request (fail closed).
func AdminAuth(expectedKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expectedKey == "" {
			AbortWithError(c, errs.NewUnauthorizedError("invalid admin key", true))
			return
		}
		provided := c.GetHeader(HeaderAdminKey)
		if !secretEqual(provided, expectedKey) {
			AbortWithError(c, errs.NewUnauthorizedError("invalid admin key", true))
			return
		}
		c.Next()
	}
}

// secretEqual compares via SHA-256 so unequal lengths do not short-circuit
// subtle.ConstantTimeCompare (which would leak the expected key length).
func secretEqual(provided, expected string) bool {
	sumProvided := sha256.Sum256([]byte(provided))
	sumExpected := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(sumProvided[:], sumExpected[:]) == 1
}
