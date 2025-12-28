package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local max_tokens = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])

local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

if tokens == nil then
    tokens = max_tokens
    last_refill = now
end

local elapsed = now - last_refill
local new_tokens = math.min(max_tokens, tokens + elapsed * refill_rate)

if new_tokens >= 1 then
    new_tokens = new_tokens - 1
    redis.call('HMSET', key, 'tokens', new_tokens, 'last_refill', now)
    redis.call('EXPIRE', key, math.ceil(max_tokens / refill_rate) + 1)
    return 1
else
    redis.call('HMSET', key, 'tokens', new_tokens, 'last_refill', now)
    redis.call('EXPIRE', key, math.ceil(max_tokens / refill_rate) + 1)
    return 0
end
`)

// RateLimit defines the token bucket configuration for a single provider.
type RateLimit struct {
	MaxTokens  int
	RefillRate float64
}

// RateLimiter enforces per-provider token bucket rate limits backed by Redis.
type RateLimiter struct {
	client *redis.Client
	limits map[string]RateLimit
}

// NewRateLimiter creates a new rate limiter for the given per-provider limits.
func NewRateLimiter(client *redis.Client, limits map[string]RateLimit) *RateLimiter {
	return &RateLimiter{client: client, limits: limits}
}

// Allow checks if a request for the given provider is permitted under its
// configured token bucket rate limit. Providers without a configured limit
// are always allowed.
func (rl *RateLimiter) Allow(ctx context.Context, provider string) (bool, error) {
	limit, ok := rl.limits[provider]
	if !ok {
		return true, nil
	}

	now := float64(time.Now().UnixMilli()) / 1000.0
	key := "llm:ratelimit:" + provider

	result, err := tokenBucketScript.Run(ctx, rl.client, []string{key},
		limit.MaxTokens, limit.RefillRate, now).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}
