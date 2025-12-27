package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

func NewRateLimiter(client *redis.Client, cfg interface{}) *RateLimiter {
	return &RateLimiter{
		client: client,
		limit:  100,
		window: time.Minute,
	}
}

func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	return true, nil
}
