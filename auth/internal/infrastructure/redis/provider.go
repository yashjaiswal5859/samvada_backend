package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ridesafety/auth-service/internal/platform/cache"
)

type redisProvider struct {
	client *redis.Client
}

func NewRedisProvider(host string, port int, password string, db int, poolSize int) cache.CacheProvider {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
		PoolSize: poolSize,
	})
	return &redisProvider{
		client: client,
	}
}

func (p *redisProvider) Get(ctx context.Context, key string) (string, error) {
	val, err := p.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (p *redisProvider) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return p.client.Set(ctx, key, value, expiration).Err()
}

func (p *redisProvider) Delete(ctx context.Context, key string) error {
	return p.client.Del(ctx, key).Err()
}

func (p *redisProvider) Exists(ctx context.Context, key string) (bool, error) {
	val, err := p.client.Exists(ctx, key).Result()
	return val > 0, err
}

func (p *redisProvider) Increment(ctx context.Context, key string) (int64, error) {
	return p.client.Incr(ctx, key).Result()
}

func (p *redisProvider) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return p.client.Expire(ctx, key, expiration).Err()
}

func (p *redisProvider) AcquireLock(ctx context.Context, key string, value string, expiration time.Duration) (bool, error) {
	return p.client.SetNX(ctx, key, value, expiration).Result()
}

func (p *redisProvider) ReleaseLock(ctx context.Context, key string, value string) (bool, error) {
	luaScript := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	res, err := p.client.Eval(ctx, luaScript, []string{key}, value).Result()
	if err != nil {
		return false, err
	}

	if val, ok := res.(int64); ok {
		return val == 1, nil
	}
	return false, nil
}
