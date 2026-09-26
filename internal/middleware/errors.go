package middleware

import (
	"errors"

	"github.com/ariwalapratham/go-payment-simulator/internal/errs"
	"github.com/ariwalapratham/go-payment-simulator/internal/sqlerr"
	"github.com/gin-gonic/gin"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Written() || len(c.Errors) == 0 {
			return
		}

		writeResolvedError(c, c.Errors.Last().Err)
	}
}

func AbortWithError(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}

func writeResolvedError(c *gin.Context, err error) {
	httpErr := resolveError(err)
	c.JSON(httpErr.Status, errs.NewErrorResponse(httpErr, GetRequestID(c)))
}

func resolveError(err error) *errs.HTTPError {
	var httpErr *errs.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr
	}

	mapped := sqlerr.HandleError(err)
	if errors.As(mapped, &httpErr) {
		return httpErr
	}

	return errs.NewInternalServerError()
}
