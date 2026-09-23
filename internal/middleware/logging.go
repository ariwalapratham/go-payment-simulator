package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// Logger logs each request in structured JSON using the zerolog
// logger passed in. It enriches every line with the request id,
// method, path, status, latency, client ip, and user agent.
func Logger(log *zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		status := c.Writer.Status()
		lat := time.Since(start)

		entry := log.With().
			Str("request_id", GetRequestID(c)).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", status).
			Dur("latency", lat).
			Str("ip", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Logger()

		switch {
		case status >= http.StatusInternalServerError:
			entry.Error().Msg("request failed")
		case status >= http.StatusBadRequest:
			entry.Warn().Msg("request completed with client error")
		default:
			entry.Info().Msg("request completed")
		}
	}
}
