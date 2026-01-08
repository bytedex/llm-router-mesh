package cache

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func NewClient(url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing redis url: %w", err)
	}
	return redis.NewClient(opts), nil
}

type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(client *redis.Client, ttl time.Duration) *RedisCache {
	return &RedisCache{client: client, ttl: ttl}
}

// EnsureIndex creates the RediSearch vector index if it doesn't exist
func (c *RedisCache) EnsureIndex(ctx context.Context) error {
	err := c.client.Do(ctx, "FT.CREATE", "llm_cache_idx", "ON", "HASH",
		"PREFIX", "1", "llm:cache:",
		"SCHEMA", "embedding", "VECTOR", "HNSW", "6", "TYPE", "FLOAT32", "DIM", "1536", "DISTANCE_METRIC", "COSINE").Err()
	if err != nil && !strings.Contains(err.Error(), "Index already exists") {
		return err
	}
	return nil
}

// GetSemantic searches for the closest vector. Returns the response if cosine distance < maxDistance.
func (c *RedisCache) GetSemantic(ctx context.Context, embedding []byte, maxDistance float32) ([]byte, error) {
	res, err := c.client.Do(ctx, "FT.SEARCH", "llm_cache_idx", "*=>[KNN 1 @embedding $vec AS score]",
		"PARAMS", "2", "vec", embedding,
		"RETURN", "2", "response", "score",
		"DIALECT", "2").Result()
	if err != nil {
		return nil, err
	}

	arr, ok := res.([]interface{})
	if !ok || len(arr) < 3 {
		return nil, nil // No results
	}

	// arr[0] is total results count. arr[1] is key. arr[2] is fields list.
	fields, ok := arr[2].([]interface{})
	if !ok {
		return nil, nil
	}

	var response []byte
	var score float64
	for i := 0; i < len(fields); i += 2 {
		field := fields[i].(string)
		if field == "response" {
			response = []byte(fields[i+1].(string))
		} else if field == "score" {
			fmt.Sscanf(fields[i+1].(string), "%f", &score)
		}
	}

	if float32(score) > maxDistance {
		return nil, nil // Not similar enough
	}

	return response, nil
}

func (c *RedisCache) SetSemantic(ctx context.Context, key string, embedding []byte, response []byte) error {
	err := c.client.HSet(ctx, key, "embedding", embedding, "response", response).Err()
	if err != nil {
		return err
	}
	return c.client.Expire(ctx, key, c.ttl).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

type NoopCache struct{}

func (NoopCache) EnsureIndex(_ context.Context) error { return nil }
func (NoopCache) GetSemantic(_ context.Context, _ []byte, _ float32) ([]byte, error) { return nil, nil }
func (NoopCache) SetSemantic(_ context.Context, _ string, _ []byte, _ []byte) error { return nil }
