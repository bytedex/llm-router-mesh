package config

import (
	"os"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
port: 9090
redis_url: "redis://localhost:6380"
otel_endpoint: "localhost:4318"
cache:
  enabled: true
  ttl_seconds: 1800
providers:
  - name: test-model
    base_url: "http://localhost:11434/v1"
    model: "qwen2.5:7b"
    tier: cheap
    rate_limit:
      requests_per_minute: 100
  - name: openai
    base_url: "https://api.openai.com/v1"
    api_key: "sk-test"
    model: "gpt-4o"
    tier: frontier
    rate_limit:
      requests_per_minute: 50
routing:
  default_tier: cheap
  complexity_keywords:
    frontier:
      - "architect"
      - "debug"
    cheap:
      - "format"
`
	f, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Errorf("port: got %d, want 9090", cfg.Port)
	}
	if cfg.RedisURL != "redis://localhost:6380" {
		t.Errorf("redis_url: got %s", cfg.RedisURL)
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("providers: got %d, want 2", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "test-model" {
		t.Errorf("provider[0].name: got %s", cfg.Providers[0].Name)
	}
	if cfg.Providers[1].RateLimit.RequestsPerMinute != 50 {
		t.Errorf("provider[1].rate_limit: got %d", cfg.Providers[1].RateLimit.RequestsPerMinute)
	}
	if cfg.Routing.DefaultTier != "cheap" {
		t.Errorf("default_tier: got %s", cfg.Routing.DefaultTier)
	}
	kw := cfg.Routing.ComplexityKeywords["frontier"]
	if len(kw) != 2 || kw[0] != "architect" {
		t.Errorf("frontier keywords: got %v", kw)
	}
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	t.Setenv("TEST_API_KEY", "expanded-key-123")

	content := `
port: 8080
redis_url: "redis://localhost:6379"
otel_endpoint: "localhost:4317"
cache:
  enabled: false
  ttl_seconds: 60
providers:
  - name: test
    base_url: "http://localhost"
    api_key: "${TEST_API_KEY}"
    model: "m"
    tier: cheap
    rate_limit:
      requests_per_minute: 10
routing:
  default_tier: cheap
  complexity_keywords: {}
`
	f, err := os.CreateTemp("", "config-env-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Providers[0].APIKey != "expanded-key-123" {
		t.Errorf("api_key not expanded: got %s", cfg.Providers[0].APIKey)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
