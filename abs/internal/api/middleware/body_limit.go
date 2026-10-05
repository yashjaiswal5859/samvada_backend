// =============================================================================
// Samvada Platform — ABS Request Body Size Limiting Middleware (Phase 6.2)
// =============================================================================

package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	// DefaultBodyLimit is 1MB for standard REST JSON payloads
	DefaultBodyLimit int64 = 1 * 1024 * 1024

	// UploadBodyLimit is 10MB for file uploads (KB docs, audio, avatars, batch CSV)
	UploadBodyLimit int64 = 10 * 1024 * 1024
)

// RequestBodyLimit returns a middleware that bounds the size of incoming HTTP bodies.
// If the Content-Length or stream exceeds limitBytes, it halts with HTTP 413.
func RequestBodyLimit(limitBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Fast-path: Check Content-Length header upfront if present
		if c.Request.ContentLength > limitBytes {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":     "PAYLOAD_TOO_LARGE",
				"message":   "Request payload exceeds the allowable size limit",
				"max_bytes": limitBytes,
			})
			return
		}

		// Stream-safe enforcement via MaxBytesReader
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limitBytes)
		c.Next()
	}
}
