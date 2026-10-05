// =============================================================================
// Samvada Platform — ABS Redis Infrastructure Adapter & Cache Hit Monitoring
// =============================================================================

package redis

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/samvada/abs/configs"
)

var (
	cacheHits   uint64
	cacheMisses uint64
)

// GetCacheMetrics returns total hits, total misses, and calculated hit ratio (Phase 6.3 - Line 514)
func GetCacheMetrics() (hits uint64, misses uint64, hitRatio float64) {
	h := atomic.LoadUint64(&cacheHits)
	m := atomic.LoadUint64(&cacheMisses)
	total := h + m
	ratio := 0.0
	if total > 0 {
		ratio = float64(h) / float64(total)
	}
	return h, m, ratio
}

// Client wraps redis.Client with helper methods for caching and ticket nonce verification.
type Client struct {
	rdb *redis.Client
	log *zap.Logger
}

// NewClient initializes a connection pool to Redis.
func NewClient(cfg *configs.RedisConfig, log *zap.Logger) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	log.Info("Connected to Redis successfully",
		zap.String("addr", cfg.Addr),
		zap.Int("db", cfg.DB),
		zap.Int("pool_size", cfg.PoolSize),
	)

	return &Client{rdb: rdb, log: log.Named("redis")}, nil
}

// Raw returns the underlying go-redis client.
func (c *Client) Raw() *redis.Client {
	return c.rdb
}

// Ping checks Redis connectivity.
func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

// Close closes the Redis client.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// Set stores a key-value pair with an expiration TTL.
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

// Get retrieves a string value by key and tracks cache hit/miss statistics.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == nil {
		atomic.AddUint64(&cacheHits, 1)
	} else if err == redis.Nil {
		atomic.AddUint64(&cacheMisses, 1)
	}
	return val, err
}

// Del deletes one or more keys.
func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

// StoreCallTicketNonce records an ephemeral call ticket nonce to prevent replay attacks (TTL 60s).
func (c *Client) StoreCallTicketNonce(ctx context.Context, nonce string, ttl time.Duration) error {
	key := fmt.Sprintf("samvada:ticket:nonce:%s", nonce)
	return c.rdb.Set(ctx, key, "valid", ttl).Err()
}

// VerifyAndConsumeNonce atomically checks if a nonce exists and deletes it so it cannot be reused.
// Returns true if valid and consumed; false if already used or expired.
func (c *Client) VerifyAndConsumeNonce(ctx context.Context, nonce string) (bool, error) {
	key := fmt.Sprintf("samvada:ticket:nonce:%s", nonce)
	// Lua script: if key exists, delete it and return 1, else return 0
	luaScript := `
		if redis.call("EXISTS", KEYS[1]) == 1 then
			redis.call("DEL", KEYS[1])
			return 1
		else
			return 0
		end
	`
	res, err := c.rdb.Eval(ctx, luaScript, []string{key}).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}
