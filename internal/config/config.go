package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port         int              `yaml:"port"`
	RedisURL     string           `yaml:"redis_url"`
	OTelEndpoint string           `yaml:"otel_endpoint"`
	Cache        CacheConfig      `yaml:"cache"`
	Providers    []ProviderConfig `yaml:"providers"`
	Routing      RoutingConfig    `yaml:"routing"`
}

type CacheConfig struct {
	Enabled         bool    `yaml:"enabled"`
	TTLSecs         int     `yaml:"ttl_seconds"`
	SimilarityScore float32 `yaml:"similarity_score"`
	EmbeddingAPIKey string  `yaml:"embedding_api_key"`
}

type ProviderConfig struct {
	Name      string          `yaml:"name"`
	BaseURL   string          `yaml:"base_url"`
	APIKey    string          `yaml:"api_key"`
	Model     string          `yaml:"model"`
	Tier      string          `yaml:"tier"`
	Timeout   int             `yaml:"timeout_ms"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

type RateLimitConfig struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
}

type RoutingConfig struct {
	DefaultTier        string              `yaml:"default_tier"`
	ComplexityKeywords map[string][]string `yaml:"complexity_keywords"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}
