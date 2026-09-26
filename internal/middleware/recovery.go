package middleware

import (
	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

func Recovery(log *zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error().
					Interface("panic", rec).
					Str("request_id", GetRequestID(c)).
					Msg("panic recovered")
				AbortWithError(c, errs.NewInternalServerError())
			}
		}()
		c.Next()
	}
}
