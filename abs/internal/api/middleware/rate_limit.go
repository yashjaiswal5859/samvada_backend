// =============================================================================
// Samvada Platform — ABS Token Bucket Rate Limiting Middleware (Phase 6.2)
// =============================================================================

package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/samvada/abs/internal/infrastructure/redis"
)

type memoryLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

var memLimiter = &memoryLimiter{
	buckets: make(map[string]*bucket),
}

// RateLimit creates rate limiting middleware with requests per minute per IP / token
func RateLimit(rdb *redis.Client, maxReqPerMin int) gin.HandlerFunc {
	if maxReqPerMin <= 0 {
		maxReqPerMin = 100
	}

	return func(c *gin.Context) {
		// Identify client by Bearer Token, API Key, or Client IP
		identifier := c.GetHeader("Authorization")
		if identifier == "" {
			identifier = c.GetHeader("X-API-Key")
		}
		if identifier == "" {
			identifier = c.ClientIP()
		}

		// Security: Hash identifier to avoid leaking raw Bearer JWTs or API keys in Redis keyspace
		h := sha256.Sum256([]byte(identifier))
		hashedKey := hex.EncodeToString(h[:8])
		rateKey := fmt.Sprintf("samvada:ratelimit:%s:%d", hashedKey, time.Now().Unix()/60)

		// 1. Try Redis rate limiting
		if rdb != nil && rdb.Raw() != nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
			defer cancel()

			count, err := rdb.Raw().Incr(ctx, rateKey).Result()
			if err == nil {
				if count == 1 {
					_ = rdb.Raw().Expire(ctx, rateKey, 65*time.Second)
				}

				remaining := maxReqPerMin - int(count)
				if remaining < 0 {
					remaining = 0
				}

				c.Header("X-RateLimit-Limit", strconv.Itoa(maxReqPerMin))
				c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

				if count > int64(maxReqPerMin) {
					c.Header("Retry-After", "60")
					c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
						"success": false,
						"error": gin.H{
							"code":    "RATE_LIMIT_EXCEEDED",
							"message": fmt.Sprintf("Rate limit exceeded. Maximum %d requests per minute allowed.", maxReqPerMin),
						},
					})
					return
				}

				c.Next()
				return
			}
		}

		// 2. In-memory fallback
		memLimiter.mu.Lock()
		// Evict stale buckets if memory map exceeds threshold to prevent DDoS memory exhaustion
		if len(memLimiter.buckets) > 5000 {
			for k, v := range memLimiter.buckets {
				if time.Since(v.lastReset) > 2*time.Minute {
					delete(memLimiter.buckets, k)
				}
			}
		}

		b, exists := memLimiter.buckets[hashedKey]
		now := time.Now()
		if !exists || now.Sub(b.lastReset) > time.Minute {
			b = &bucket{tokens: maxReqPerMin - 1, lastReset: now}
			memLimiter.buckets[hashedKey] = b
			memLimiter.mu.Unlock()
			c.Header("X-RateLimit-Limit", strconv.Itoa(maxReqPerMin))
			c.Header("X-RateLimit-Remaining", strconv.Itoa(maxReqPerMin-1))
			c.Next()
			return
		}

		if b.tokens <= 0 {
			memLimiter.mu.Unlock()
			c.Header("X-RateLimit-Limit", strconv.Itoa(maxReqPerMin))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "RATE_LIMIT_EXCEEDED",
					"message": fmt.Sprintf("Rate limit exceeded. Maximum %d requests per minute allowed.", maxReqPerMin),
				},
			})
			return
		}

		b.tokens--
		remaining := b.tokens
		memLimiter.mu.Unlock()

		c.Header("X-RateLimit-Limit", strconv.Itoa(maxReqPerMin))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Next()
	}
}
