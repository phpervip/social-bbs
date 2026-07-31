package repository

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache abstracts the Redis operations required by the service layer.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	MGet(ctx context.Context, keys ...string) ([]string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	ZAdd(ctx context.Context, key string, score float64, member string) error
	ZRevRangeByScore(ctx context.Context, key, max, min string, offset, count int64) ([]string, error)
	ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error
	Expire(ctx context.Context, key string, ttl time.Duration) error
}

// RedisCache is the go-redis backed Cache implementation.
type RedisCache struct {
	rdb *redis.Client
}

// NewRedisCache wraps a go-redis client.
func NewRedisCache(rdb *redis.Client) *RedisCache { return &RedisCache{rdb: rdb} }

func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	v, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrCacheMiss
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func (c *RedisCache) MGet(ctx context.Context, keys ...string) ([]string, error) {
	vals, err := c.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]string, len(vals))
	for i, v := range vals {
		if v == nil {
			out[i] = ""
			continue
		}
		out[i], _ = v.(string)
	}
	return out, nil
}

func (c *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Del(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

func (c *RedisCache) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, key, value, ttl).Result()
}

func (c *RedisCache) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return c.rdb.ZAdd(ctx, key, redis.Z{Score: score, Member: member}).Err()
}

func (c *RedisCache) ZRevRangeByScore(ctx context.Context, key, max, min string, offset, count int64) ([]string, error) {
	return c.rdb.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{Max: max, Min: min, Offset: offset, Count: count}).Result()
}

func (c *RedisCache) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) error {
	return c.rdb.ZRemRangeByRank(ctx, key, start, stop).Err()
}

func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Expire(ctx, key, ttl).Err()
}
