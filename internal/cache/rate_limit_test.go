package cache

import (
	"context"
	"testing"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	ctx := context.Background()
	client.Del(ctx, "llm:ratelimit:test-allow")

	rl := NewRateLimiter(client, map[string]RateLimit{
		"test-allow": {MaxTokens: 5, RefillRate: 0.001},
	})

	for i := 0; i < 5; i++ {
		ok, err := rl.Allow(ctx, "test-allow")
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if !ok {
			t.Fatalf("request %d: should be allowed", i)
		}
	}
}

func TestRateLimiter_DeniesOverLimit(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	ctx := context.Background()
	client.Del(ctx, "llm:ratelimit:test-deny")

	rl := NewRateLimiter(client, map[string]RateLimit{
		"test-deny": {MaxTokens: 2, RefillRate: 0.001},
	})

	rl.Allow(ctx, "test-deny")
	rl.Allow(ctx, "test-deny")

	ok, err := rl.Allow(ctx, "test-deny")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("third request should be denied")
	}
}

func TestRateLimiter_AllowsUnknownProvider(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	rl := NewRateLimiter(client, map[string]RateLimit{})

	ok, err := rl.Allow(context.Background(), "unconfigured-provider")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("unconfigured provider should be allowed (no limit)")
	}
}
