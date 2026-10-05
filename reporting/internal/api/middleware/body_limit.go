// =============================================================================
// Samvada Platform — Reporting Engine Request Body Size Limiting (Phase 6.2)
// =============================================================================

package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	// DefaultBodyLimit is 1MB for analytics REST JSON queries
	DefaultBodyLimit int64 = 1 * 1024 * 1024
)

// RequestBodyLimit bounds the maximum bytes an incoming query/payload can consume.
func RequestBodyLimit(limitBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > limitBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":     "PAYLOAD_TOO_LARGE",
				"message":   "Request payload exceeds the allowable size limit",
				"max_bytes": limitBytes,
			})
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limitBytes)
		c.Next()
	}
}
