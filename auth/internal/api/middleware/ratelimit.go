package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ridesafety/auth-service/internal/platform/cache"
	"github.com/ridesafety/auth-service/internal/shared/response"
)

func RateLimiter(cacheProvider cache.CacheProvider, limit int, window time.Duration, keyPrefix string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		key := fmt.Sprintf("auth:rate:%s:%s", keyPrefix, ip)

		ctx := c.Request.Context()
		count, err := cacheProvider.Increment(ctx, key)
		if err != nil {
			c.Next()
			return
		}

		if count == 1 {
			_ = cacheProvider.Expire(ctx, key, window)
		}

		if count > int64(limit) {
			c.Header("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			response.JSON(c, http.StatusTooManyRequests, false, nil, &response.ErrorDetail{
				Code:    "AUTH_429",
				Message: "Too many requests. Please try again later.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
