// Package redis cung cấp client Redis và implement port.Cache (cache-aside +
// rate limit) cùng health checker.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"document-hub-api/internal/common/port"
)

// Config là tham số kết nối Redis.
type Config struct {
	Addr     string
	Password string
	DB       int
	PoolSize int
}

// New tạo và ping client Redis.
func New(ctx context.Context, cfg Config) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping Redis thất bại: %w", err)
	}
	return client, nil
}

// Cache implement port.Cache.
type Cache struct {
	client *goredis.Client
}

// NewCache bọc client thành port.Cache.
func NewCache(client *goredis.Client) *Cache {
	return &Cache{client: client}
}

// Get trả về giá trị; port.ErrCacheMiss nếu key không tồn tại.
func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return "", port.ErrCacheMiss
	}
	if err != nil {
		return "", fmt.Errorf("redis get %q: %w", key, err)
	}
	return val, nil
}

// Set ghi giá trị kèm TTL.
func (c *Cache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if err := c.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}
	return nil
}

// Del xóa các key.
func (c *Cache) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis del: %w", err)
	}
	return nil
}

// Incr tăng bộ đếm; đặt TTL ở lần tạo đầu tiên (giá trị trả về == 1).
func (c *Cache) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis incr %q: %w", key, err)
	}
	if n == 1 {
		if err := c.client.Expire(ctx, key, ttl).Err(); err != nil {
			return n, fmt.Errorf("redis expire %q: %w", key, err)
		}
	}
	return n, nil
}
