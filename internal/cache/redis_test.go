package cache

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func redisClient(t *testing.T) *redis.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: requires Redis (use -short to skip)")
	}
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping: Redis not available: %v", err)
	}
	return client
}

func TestRedisCache_GetMiss(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	c := NewRedisCache(client, 10*time.Second)
	ctx := context.Background()

	val, err := c.Get(ctx, "llm:cache:nonexistent-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil {
		t.Fatalf("expected nil for missing key, got %s", val)
	}
}

func TestRedisCache_SetAndGet(t *testing.T) {
	client := redisClient(t)
	defer client.Close()

	c := NewRedisCache(client, 10*time.Second)
	ctx := context.Background()
	key := "llm:cache:test-set-get"

	defer client.Del(ctx, key)

	err := c.Set(ctx, key, []byte(`{"response":"hello"}`))
	if err != nil {
		t.Fatalf("set error: %v", err)
	}

	val, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if string(val) != `{"response":"hello"}` {
		t.Fatalf("got %s, want {\"response\":\"hello\"}", val)
	}
}

func TestNoopCache_AlwaysMisses(t *testing.T) {
	c := NoopCache{}
	ctx := context.Background()

	val, err := c.Get(ctx, "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil {
		t.Fatal("noop cache should always return nil")
	}

	err = c.Set(ctx, "anything", []byte("data"))
	if err != nil {
		t.Fatalf("noop set should not error: %v", err)
	}
}
