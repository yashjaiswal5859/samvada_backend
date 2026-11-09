package cache

import (
	"context"
	"time"
)

type CacheProvider interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Increment(ctx context.Context, key string) (int64, error)
	Expire(ctx context.Context, key string, expiration time.Duration) error
	AcquireLock(ctx context.Context, key string, value string, expiration time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string, value string) (bool, error)
}
