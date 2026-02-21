package cache

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultTimeout = 200 * time.Millisecond
	defaultTTL     = 10 * time.Minute
	keyPrefix      = "url:"
)

// Cache wraps an optional Redis client for URL lookups.
// If client is nil, Get returns (_, false) and Set is a no-op.
type Cache struct {
	client *redis.Client
}

// New returns a cache that uses the given Redis client.
// Pass nil to get a no-op cache (e.g. when Redis is unavailable).
func New(client *redis.Client) *Cache {
	return &Cache{client: client}
}

// Get returns the long URL for the given code, or ("", false) on miss/error.
func (c *Cache) Get(code string) (string, bool) {
	if c.client == nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	val, err := c.client.Get(ctx, keyPrefix+code).Result()
	if err == redis.Nil {
		return "", false
	}
	if err != nil {
		log.Println("redis get failed:", err)
		return "", false
	}
	return val, true
}

// Set stores the long URL for the given code. TTL uses defaultTTL.
func (c *Cache) Set(code, longURL string) {
	if c.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	if err := c.client.Set(ctx, keyPrefix+code, longURL, defaultTTL).Err(); err != nil {
		log.Println("redis set failed:", err)
	}
}
