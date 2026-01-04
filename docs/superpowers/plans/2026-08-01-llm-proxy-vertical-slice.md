# LLM Proxy Vertical Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the complete LLM proxy request path end-to-end — HTTP request in, cache check, complexity-based routing, rate limiting, forward to downstream provider, cache response, return to client.

**Architecture:** The gateway accepts OpenAI-compatible `/v1/chat/completions` POST requests. It checks a Redis hash-based cache for exact-match hits, then uses a keyword-based router to classify prompt complexity into tiers (cheap/mid/frontier) and select the appropriate provider. A Redis token-bucket rate limiter prevents upstream throttling. The request is forwarded to the selected OpenAI-compatible backend (Ollama, OpenAI, Gemini OpenAI-compat endpoint), the response is cached, and returned to the client with routing metadata headers.

**Tech Stack:** Go 1.24, `gopkg.in/yaml.v3` (config), `github.com/redis/go-redis/v9` (cache + rate limiting), `net/http` stdlib (HTTP server + reverse proxying), Redis 7 (via Docker)

## Global Constraints

- Go 1.24+ (module already set to 1.24.5)
- Only OpenAI-compatible backends supported in this slice (Ollama, OpenAI, Gemini OpenAI-compat). Anthropic API translation is a future slice.
- Streaming (`stream: true`) is out of scope — non-streaming responses only.
- MCP routing/proxy is untouched — it's a separate vertical slice.
- Telemetry stubs remain as no-ops — instrumentation is a separate slice.
- No external test dependencies — use stdlib `testing` only.
- Redis integration tests are skipped with `-short` flag when Redis is unavailable.

## File Structure

**Create:**
| File | Responsibility |
|------|---------------|
| `internal/domain/types.go` | Shared types (`Provider`, `RoutingDecision`) that cross package boundaries |
| `internal/config/config_test.go` | Config YAML parsing tests |
| `internal/cache/redis_test.go` | Redis cache integration tests |
| `internal/cache/rate_limit_test.go` | Rate limiter integration tests |
| `internal/router/llm_router_test.go` | Router unit tests (no Redis needed) |
| `internal/proxy/llm_proxy_test.go` | Proxy handler tests with mock interfaces |
| `config.example.yaml` | Example config showing all options |
| `docker-compose.yml` | Redis for local development |

**Modify:**
| File | What changes |
|------|-------------|
| `go.mod` | Add `gopkg.in/yaml.v3` and `github.com/redis/go-redis/v9` |
| `internal/config/config.go` | Real YAML parsing with env var expansion, full config struct |
| `internal/cache/redis.go` | Real Redis client, hash-based cache, `NoopCache`, `NewClient` helper |
| `internal/cache/rate_limit.go` | Token bucket via Redis Lua script, per-provider limits |
| `internal/router/llm_router.go` | Keyword-based complexity routing with config-driven provider registry |
| `internal/proxy/llm_proxy.go` | HTTP handler: parse → cache → route → rate-limit → forward → cache-store → respond |
| `cmd/llm-router-mesh/main.go` | Wire all components, HTTP server, health endpoint, graceful shutdown |

**Untouched:**
- `internal/router/mcp_router.go` — future MCP slice
- `internal/proxy/mcp_proxy.go` — future MCP slice
- `internal/telemetry/otel.go` — called as no-op, instrumented in future slice

---

### Task 1: Config, Domain Types & Example Config

**Files:**
- Create: `internal/domain/types.go`
- Create: `config.example.yaml`
- Create: `internal/config/config_test.go`
- Modify: `internal/config/config.go`
- Modify: `go.mod` (add `gopkg.in/yaml.v3`)

**Interfaces:**
- Consumes: nothing (foundational task)
- Produces:
  - `domain.Provider{Name, BaseURL, APIKey, Model string}`
  - `domain.RoutingDecision{Provider Provider, Reason string}`
  - `config.Config` struct with full schema
  - `config.Load(path string) (*Config, error)`

- [ ] **Step 1: Write the config parsing test**

Create `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/arnabdutta/Documents/repos/slow-codex/projects/llm-router-mesh && go test ./internal/config/ -v`

Expected: compilation error — `Load` has wrong signature, types don't exist yet.

- [ ] **Step 3: Create domain types**

Create `internal/domain/types.go`:

```go
package domain

type Provider struct {
	Name    string
	BaseURL string
	APIKey  string
	Model   string
}

type RoutingDecision struct {
	Provider Provider
	Reason   string
}
```

- [ ] **Step 4: Implement config loading**

Rewrite `internal/config/config.go`:

```go
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
	Enabled bool `yaml:"enabled"`
	TTLSecs int  `yaml:"ttl_seconds"`
}

type ProviderConfig struct {
	Name      string          `yaml:"name"`
	BaseURL   string          `yaml:"base_url"`
	APIKey    string          `yaml:"api_key"`
	Model     string          `yaml:"model"`
	Tier      string          `yaml:"tier"`
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
```

- [ ] **Step 5: Add yaml dependency and run tests**

Run:
```bash
cd /Users/arnabdutta/Documents/repos/slow-codex/projects/llm-router-mesh
go get gopkg.in/yaml.v3
go test ./internal/config/ -v
```

Expected: all 3 tests PASS.

- [ ] **Step 6: Create example config**

Create `config.example.yaml`:

```yaml
port: 8080
redis_url: "redis://localhost:6379"
otel_endpoint: "localhost:4317"

cache:
  enabled: true
  ttl_seconds: 3600

providers:
  # Local Qwen 2.5 via Ollama (OpenAI-compatible API)
  - name: qwen-local
    base_url: "http://localhost:11434/v1"
    model: "qwen2.5:7b"
    tier: cheap
    rate_limit:
      requests_per_minute: 1000

  # Gemini via OpenAI-compatible endpoint
  - name: gemini
    base_url: "https://generativelanguage.googleapis.com/v1beta/openai"
    api_key: "${GEMINI_API_KEY}"
    model: "gemini-2.0-flash"
    tier: mid
    rate_limit:
      requests_per_minute: 500

  # OpenAI GPT-4o
  - name: openai
    base_url: "https://api.openai.com/v1"
    api_key: "${OPENAI_API_KEY}"
    model: "gpt-4o"
    tier: frontier
    rate_limit:
      requests_per_minute: 100

# Keywords checked frontier-first: if "debug" and "format" both appear,
# frontier wins. This is intentional — err toward quality.
routing:
  default_tier: cheap
  complexity_keywords:
    frontier:
      - "architect"
      - "debug"
      - "plan"
      - "design"
      - "analyze"
      - "refactor"
      - "security"
      - "performance"
    mid:
      - "summarize"
      - "explain"
      - "compare"
      - "review"
      - "describe"
    cheap:
      - "format"
      - "typo"
      - "translate"
      - "convert"
      - "list"
```

- [ ] **Step 7: Commit**

```bash
git add internal/domain/types.go internal/config/config.go internal/config/config_test.go config.example.yaml go.mod go.sum
git commit -m "feat: add config YAML parsing, domain types, and example config"
```

---

### Task 2: Redis Cache with Hash-Based Lookup

**Files:**
- Create: `internal/cache/redis_test.go`
- Create: `docker-compose.yml`
- Modify: `internal/cache/redis.go`
- Modify: `go.mod` (add `github.com/redis/go-redis/v9`)

**Interfaces:**
- Consumes: nothing
- Produces:
  - `cache.NewClient(url string) (*redis.Client, error)` — creates shared Redis client
  - `cache.RedisCache` struct implementing `Get(ctx, key) ([]byte, error)` and `Set(ctx, key, value) error`
  - `cache.NoopCache` struct with same method signatures (for disabled cache)
  - `cache.RedisCache.Close() error`

- [ ] **Step 1: Write the cache integration tests**

Create `internal/cache/redis_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/arnabdutta/Documents/repos/slow-codex/projects/llm-router-mesh && go test ./internal/cache/ -v`

Expected: compilation error — `NewClient`, `NewRedisCache`, `NoopCache` not defined yet.

- [ ] **Step 3: Implement the Redis cache**

Rewrite `internal/cache/redis.go`:

```go
package cache

import (
	"context"
	"errors"
	"fmt"
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

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return val, err
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte) error {
	return c.client.Set(ctx, key, value, c.ttl).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

type NoopCache struct{}

func (NoopCache) Get(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (NoopCache) Set(_ context.Context, _ string, _ []byte) error { return nil }
```

- [ ] **Step 4: Add Redis dependency and create docker-compose**

Run:
```bash
cd /Users/arnabdutta/Documents/repos/slow-codex/projects/llm-router-mesh
go get github.com/redis/go-redis/v9
```

Create `docker-compose.yml`:

```yaml
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 3
```

- [ ] **Step 5: Start Redis and run tests**

Run:
```bash
cd /Users/arnabdutta/Documents/repos/slow-codex/projects/llm-router-mesh
docker-compose up -d
go test ./internal/cache/ -v -run TestRedisCache
go test ./internal/cache/ -v -run TestNoopCache
```

Expected: all 3 tests PASS. If Redis is not running, the Redis tests skip gracefully.

- [ ] **Step 6: Commit**

```bash
git add internal/cache/redis.go internal/cache/redis_test.go docker-compose.yml go.mod go.sum
git commit -m "feat: implement Redis hash-based cache with NoopCache fallback"
```

---

### Task 3: Token Bucket Rate Limiter

**Files:**
- Create: `internal/cache/rate_limit_test.go`
- Modify: `internal/cache/rate_limit.go`

**Interfaces:**
- Consumes: `*redis.Client` from `cache.NewClient()` (Task 2)
- Produces:
  - `cache.RateLimit{MaxTokens int, RefillRate float64}` — per-provider config
  - `cache.NewRateLimiter(client *redis.Client, limits map[string]RateLimit) *RateLimiter`
  - `(*RateLimiter).Allow(ctx context.Context, provider string) (bool, error)`

- [ ] **Step 1: Write the rate limiter tests**

Create `internal/cache/rate_limit_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cache/ -v -run TestRateLimiter`

Expected: compilation error — `RateLimit`, `NewRateLimiter` not defined with new signatures.

- [ ] **Step 3: Implement the token bucket rate limiter**

Rewrite `internal/cache/rate_limit.go`:

```go
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

type RateLimit struct {
	MaxTokens  int
	RefillRate float64
}

type RateLimiter struct {
	client *redis.Client
	limits map[string]RateLimit
}

func NewRateLimiter(client *redis.Client, limits map[string]RateLimit) *RateLimiter {
	return &RateLimiter{client: client, limits: limits}
}

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
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/cache/ -v -run TestRateLimiter`

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cache/rate_limit.go internal/cache/rate_limit_test.go
git commit -m "feat: implement token bucket rate limiter via Redis Lua script"
```

---

### Task 4: Keyword-Based LLM Router

**Files:**
- Create: `internal/router/llm_router_test.go`
- Modify: `internal/router/llm_router.go`

**Interfaces:**
- Consumes:
  - `config.Config` (Task 1) — provider list and routing keywords
  - `domain.Provider`, `domain.RoutingDecision` (Task 1)
- Produces:
  - `router.NewLLMRouter(cfg *config.Config) *LLMRouter`
  - `(*LLMRouter).Route(ctx context.Context, prompt string) (*domain.RoutingDecision, error)`

- [ ] **Step 1: Write the router tests**

Create `internal/router/llm_router_test.go`:

```go
package router

import (
	"context"
	"testing"

	"github.com/arnabdutta/llm-router-mesh/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "local-cheap", BaseURL: "http://localhost:11434/v1", Model: "qwen2.5:7b", Tier: "cheap"},
			{Name: "cloud-mid", BaseURL: "https://mid.example.com/v1", APIKey: "mid-key", Model: "mid-model", Tier: "mid"},
			{Name: "cloud-frontier", BaseURL: "https://frontier.example.com/v1", APIKey: "frontier-key", Model: "gpt-4o", Tier: "frontier"},
		},
		Routing: config.RoutingConfig{
			DefaultTier: "cheap",
			ComplexityKeywords: map[string][]string{
				"frontier": {"architect", "debug", "security"},
				"mid":      {"summarize", "explain"},
				"cheap":    {"format", "typo"},
			},
		},
	}
}

func TestLLMRouter_FrontierKeyword(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "Help me debug this segfault")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "cloud-frontier" {
		t.Errorf("got provider %s, want cloud-frontier", decision.Provider.Name)
	}
	if decision.Provider.Model != "gpt-4o" {
		t.Errorf("got model %s, want gpt-4o", decision.Provider.Model)
	}
}

func TestLLMRouter_CheapKeyword(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "format this JSON for me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "local-cheap" {
		t.Errorf("got provider %s, want local-cheap", decision.Provider.Name)
	}
}

func TestLLMRouter_MidKeyword(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "Summarize this document")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "cloud-mid" {
		t.Errorf("got provider %s, want cloud-mid", decision.Provider.Name)
	}
}

func TestLLMRouter_DefaultTier(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "local-cheap" {
		t.Errorf("got provider %s, want local-cheap (default)", decision.Provider.Name)
	}
}

func TestLLMRouter_FrontierBeatsOtherTiers(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "format and debug this code")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "cloud-frontier" {
		t.Errorf("got provider %s, want cloud-frontier (frontier should win)", decision.Provider.Name)
	}
}

func TestLLMRouter_CaseInsensitive(t *testing.T) {
	r := NewLLMRouter(testConfig())
	decision, err := r.Route(context.Background(), "EXPLAIN this function")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Provider.Name != "cloud-mid" {
		t.Errorf("got provider %s, want cloud-mid", decision.Provider.Name)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/router/ -v`

Expected: compilation error — `NewLLMRouter` signature changed, `Route` return type changed.

- [ ] **Step 3: Implement the keyword router**

Rewrite `internal/router/llm_router.go`:

```go
package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/arnabdutta/llm-router-mesh/internal/config"
	"github.com/arnabdutta/llm-router-mesh/internal/domain"
)

type LLMRouter struct {
	providers   map[string]domain.Provider
	keywords    map[string][]string
	defaultTier string
}

func NewLLMRouter(cfg *config.Config) *LLMRouter {
	providers := make(map[string]domain.Provider)
	for _, p := range cfg.Providers {
		providers[p.Tier] = domain.Provider{
			Name:    p.Name,
			BaseURL: p.BaseURL,
			APIKey:  p.APIKey,
			Model:   p.Model,
		}
	}
	return &LLMRouter{
		providers:   providers,
		keywords:    cfg.Routing.ComplexityKeywords,
		defaultTier: cfg.Routing.DefaultTier,
	}
}

func (r *LLMRouter) Route(_ context.Context, prompt string) (*domain.RoutingDecision, error) {
	lower := strings.ToLower(prompt)

	for _, tier := range []string{"frontier", "mid", "cheap"} {
		keywords, ok := r.keywords[tier]
		if !ok {
			continue
		}
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				provider, ok := r.providers[tier]
				if !ok {
					continue
				}
				return &domain.RoutingDecision{
					Provider: provider,
					Reason:   fmt.Sprintf("keyword %q matched tier %s", kw, tier),
				}, nil
			}
		}
	}

	provider := r.providers[r.defaultTier]
	return &domain.RoutingDecision{
		Provider: provider,
		Reason:   "no keywords matched, using default tier",
	}, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/router/ -v`

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/router/llm_router.go internal/router/llm_router_test.go
git commit -m "feat: implement keyword-based LLM complexity router"
```

---

### Task 5: LLM Proxy HTTP Handler

**Files:**
- Create: `internal/proxy/llm_proxy_test.go`
- Modify: `internal/proxy/llm_proxy.go`

**Interfaces:**
- Consumes:
  - `domain.Provider`, `domain.RoutingDecision` (Task 1)
  - `Cache` interface (implemented by `cache.RedisCache` / `cache.NoopCache` from Task 2)
  - `RateLimiter` interface (implemented by `cache.RateLimiter` from Task 3)
  - `Router` interface (implemented by `router.LLMRouter` from Task 4)
- Produces:
  - `proxy.Cache` interface: `Get(ctx, key) ([]byte, error)`, `Set(ctx, key, value) error`
  - `proxy.Router` interface: `Route(ctx, prompt) (*domain.RoutingDecision, error)`
  - `proxy.RateLimiter` interface: `Allow(ctx, provider) (bool, error)`
  - `proxy.NewLLMProxy(cache Cache, router Router, rl RateLimiter) *LLMProxy`
  - `(*LLMProxy).ServeHTTP(w, r)` — implements `http.Handler`

- [ ] **Step 1: Write the proxy handler tests**

Create `internal/proxy/llm_proxy_test.go`:

```go
package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arnabdutta/llm-router-mesh/internal/domain"
)

type mockCache struct {
	store map[string][]byte
}

func (m *mockCache) Get(_ context.Context, key string) ([]byte, error) {
	v := m.store[key]
	return v, nil
}
func (m *mockCache) Set(_ context.Context, key string, value []byte) error {
	m.store[key] = value
	return nil
}

type mockRouter struct {
	decision *domain.RoutingDecision
}

func (m *mockRouter) Route(_ context.Context, _ string) (*domain.RoutingDecision, error) {
	return m.decision, nil
}

type mockRateLimiter struct {
	allowed bool
}

func (m *mockRateLimiter) Allow(_ context.Context, _ string) (bool, error) {
	return m.allowed, nil
}

func TestLLMProxy_CacheHit(t *testing.T) {
	msgs := []message{{Role: "user", Content: "cached question"}}
	key := computeCacheKey(msgs)

	mc := &mockCache{store: map[string][]byte{
		key: []byte(`{"id":"cached","choices":[{"message":{"role":"assistant","content":"cached answer"}}]}`),
	}}

	p := NewLLMProxy(mc, &mockRouter{}, &mockRateLimiter{allowed: true})

	body := `{"messages":[{"role":"user","content":"cached question"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if w.Header().Get("X-Cache") != "HIT" {
		t.Errorf("X-Cache: got %s, want HIT", w.Header().Get("X-Cache"))
	}
}

func TestLLMProxy_CacheMiss_ForwardsToProvider(t *testing.T) {
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "test-model" {
			t.Errorf("downstream model: got %s, want test-model", req.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"live","choices":[{"message":{"role":"assistant","content":"live answer"}}]}`))
	}))
	defer downstream.Close()

	mc := &mockCache{store: make(map[string][]byte)}
	mr := &mockRouter{decision: &domain.RoutingDecision{
		Provider: domain.Provider{Name: "test-provider", BaseURL: downstream.URL, Model: "test-model"},
		Reason:   "test",
	}}

	p := NewLLMProxy(mc, mr, &mockRateLimiter{allowed: true})

	body := `{"messages":[{"role":"user","content":"live question"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status: got %d, want 200", w.Code)
	}
	if w.Header().Get("X-Cache") != "MISS" {
		t.Errorf("X-Cache: got %s, want MISS", w.Header().Get("X-Cache"))
	}
	if w.Header().Get("X-Routed-To") != "test-provider" {
		t.Errorf("X-Routed-To: got %s, want test-provider", w.Header().Get("X-Routed-To"))
	}
	if len(mc.store) != 1 {
		t.Error("response should be cached after a miss")
	}
}

func TestLLMProxy_RateLimited(t *testing.T) {
	mc := &mockCache{store: make(map[string][]byte)}
	mr := &mockRouter{decision: &domain.RoutingDecision{
		Provider: domain.Provider{Name: "p"},
		Reason:   "test",
	}}

	p := NewLLMProxy(mc, mr, &mockRateLimiter{allowed: false})

	body := `{"messages":[{"role":"user","content":"rate limited"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status: got %d, want 429", w.Code)
	}
}

func TestLLMProxy_ProviderDown(t *testing.T) {
	mc := &mockCache{store: make(map[string][]byte)}
	mr := &mockRouter{decision: &domain.RoutingDecision{
		Provider: domain.Provider{Name: "dead", BaseURL: "http://127.0.0.1:1"},
		Reason:   "test",
	}}

	p := NewLLMProxy(mc, mr, &mockRateLimiter{allowed: true})

	body := `{"messages":[{"role":"user","content":"to dead provider"}]}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status: got %d, want 502", w.Code)
	}
}

func TestLLMProxy_MethodNotAllowed(t *testing.T) {
	p := NewLLMProxy(&mockCache{store: make(map[string][]byte)}, &mockRouter{}, &mockRateLimiter{allowed: true})

	req := httptest.NewRequest("GET", "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	p.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want 405", w.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/proxy/ -v`

Expected: compilation error — `Cache`/`Router`/`RateLimiter` interfaces, `NewLLMProxy`, `computeCacheKey`, `message`, `chatRequest` not defined.

- [ ] **Step 3: Implement the proxy handler**

Rewrite `internal/proxy/llm_proxy.go`:

```go
package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/arnabdutta/llm-router-mesh/internal/domain"
)

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte) error
}

type Router interface {
	Route(ctx context.Context, prompt string) (*domain.RoutingDecision, error)
}

type RateLimiter interface {
	Allow(ctx context.Context, provider string) (bool, error)
}

type LLMProxy struct {
	cache       Cache
	router      Router
	rateLimiter RateLimiter
	client      *http.Client
}

func NewLLMProxy(cache Cache, router Router, rateLimiter RateLimiter) *LLMProxy {
	return &LLMProxy{
		cache:       cache,
		router:      router,
		rateLimiter: rateLimiter,
		client:      &http.Client{Timeout: 60 * time.Second},
	}
}

type chatRequest struct {
	Model    string    `json:"model,omitempty"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (p *LLMProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}

	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// 1. Cache check
	cacheKey := computeCacheKey(req.Messages)
	if cached, err := p.cache.Get(ctx, cacheKey); err == nil && cached != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write(cached)
		return
	}

	// 2. Route
	prompt := extractPrompt(req.Messages)
	decision, err := p.router.Route(ctx, prompt)
	if err != nil {
		http.Error(w, "routing failed", http.StatusInternalServerError)
		return
	}

	// 3. Rate limit (fail open on error)
	if allowed, err := p.rateLimiter.Allow(ctx, decision.Provider.Name); err == nil && !allowed {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	// 4. Forward to provider
	respBody, statusCode, err := p.forward(ctx, &req, &decision.Provider)
	if err != nil {
		http.Error(w, "provider unavailable", http.StatusBadGateway)
		return
	}

	// 5. Cache on success
	if statusCode == http.StatusOK {
		p.cache.Set(ctx, cacheKey, respBody)
	}

	// 6. Return
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("X-Routed-To", decision.Provider.Name)
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

func (p *LLMProxy) forward(ctx context.Context, req *chatRequest, provider *domain.Provider) ([]byte, int, error) {
	req.Model = provider.Model

	body, err := json.Marshal(req)
	if err != nil {
		return nil, 0, err
	}

	url := provider.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	return respBody, resp.StatusCode, nil
}

func extractPrompt(msgs []message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return msgs[i].Content
		}
	}
	return ""
}

func computeCacheKey(msgs []message) string {
	data, _ := json.Marshal(msgs)
	h := sha256.Sum256(data)
	return "llm:cache:" + hex.EncodeToString(h[:])
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/proxy/ -v`

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/llm_proxy.go internal/proxy/llm_proxy_test.go
git commit -m "feat: implement LLM proxy HTTP handler with cache/route/forward pipeline"
```

---

### Task 6: Server Wiring, Health Endpoint & Graceful Shutdown

**Files:**
- Modify: `cmd/llm-router-mesh/main.go`

**Interfaces:**
- Consumes:
  - `config.Load(path) (*Config, error)` (Task 1)
  - `cache.NewClient(url) (*redis.Client, error)` (Task 2)
  - `cache.NewRedisCache(client, ttl) *RedisCache` (Task 2)
  - `cache.NoopCache{}` (Task 2)
  - `cache.NewRateLimiter(client, limits) *RateLimiter` (Task 3)
  - `cache.RateLimit{MaxTokens, RefillRate}` (Task 3)
  - `router.NewLLMRouter(cfg) *LLMRouter` (Task 4)
  - `proxy.NewLLMProxy(cache, router, rl) *LLMProxy` (Task 5)
  - `proxy.Cache` interface (Task 5) — for the `var` declaration
  - `telemetry.Setup(endpoint) error` — existing no-op stub
  - `telemetry.Shutdown()` — existing no-op stub
- Produces: running HTTP server on configured port with `/health` and `/v1/chat/completions`

- [ ] **Step 1: Rewrite main.go**

Rewrite `cmd/llm-router-mesh/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arnabdutta/llm-router-mesh/internal/cache"
	"github.com/arnabdutta/llm-router-mesh/internal/config"
	"github.com/arnabdutta/llm-router-mesh/internal/proxy"
	"github.com/arnabdutta/llm-router-mesh/internal/router"
	"github.com/arnabdutta/llm-router-mesh/internal/telemetry"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if err := telemetry.Setup(cfg.OTelEndpoint); err != nil {
		log.Fatalf("failed to initialize telemetry: %v", err)
	}
	defer telemetry.Shutdown()

	redisClient, err := cache.NewClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("failed to create redis client: %v", err)
	}
	defer redisClient.Close()

	var llmCache proxy.Cache
	if cfg.Cache.Enabled {
		ttl := time.Duration(cfg.Cache.TTLSecs) * time.Second
		llmCache = cache.NewRedisCache(redisClient, ttl)
	} else {
		llmCache = cache.NoopCache{}
	}

	limits := make(map[string]cache.RateLimit)
	for _, p := range cfg.Providers {
		if p.RateLimit.RequestsPerMinute > 0 {
			limits[p.Name] = cache.RateLimit{
				MaxTokens:  p.RateLimit.RequestsPerMinute,
				RefillRate: float64(p.RateLimit.RequestsPerMinute) / 60.0,
			}
		}
	}
	rateLimiter := cache.NewRateLimiter(redisClient, limits)

	llmRouter := router.NewLLMRouter(cfg)
	llmProxy := proxy.NewLLMProxy(llmCache, llmRouter, rateLimiter)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/v1/chat/completions", llmProxy)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("received %s, shutting down...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	log.Printf("LLM Router Mesh listening on :%d", cfg.Port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
	log.Println("server stopped")
}
```

- [ ] **Step 2: Verify it compiles**

Run:
```bash
cd /Users/arnabdutta/Documents/repos/slow-codex/projects/llm-router-mesh
go build ./cmd/llm-router-mesh/
```

Expected: compiles with no errors.

- [ ] **Step 3: Run all tests**

Run:
```bash
go test ./... -v -short
```

Expected: all tests PASS. Redis integration tests are skipped with `-short`.

- [ ] **Step 4: Smoke test with live Redis**

Start Redis and run the full suite:
```bash
docker-compose up -d
go test ./... -v
```

Expected: all tests PASS including Redis integration tests.

- [ ] **Step 5: Manual smoke test**

Copy the example config and start the gateway:
```bash
cp config.example.yaml config.yaml
# Edit config.yaml to point to a running Ollama instance or remove providers you don't have
./llm-router-mesh --config config.yaml
```

In another terminal, hit the health endpoint:
```bash
curl http://localhost:8080/health
# Expected: {"status":"ok"}
```

Send a test request (requires a running downstream provider, e.g. Ollama):
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"format this: {a:1,b:2}"}]}'
# Expected: response from the cheap-tier provider
# Headers: X-Cache: MISS, X-Routed-To: qwen-local
```

Send the same request again:
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"messages":[{"role":"user","content":"format this: {a:1,b:2}"}]}'
# Expected: cached response
# Headers: X-Cache: HIT
```

- [ ] **Step 6: Commit**

```bash
git add cmd/llm-router-mesh/main.go
git commit -m "feat: wire LLM proxy pipeline with HTTP server and graceful shutdown"
```

---

## Follow-Up Slices (Out of Scope)

These are not part of this plan but are the natural next steps:

1. **Telemetry Instrumentation** — OTel spans around cache/route/forward, `llm_cost_saved_usd` counter, `cache_hit_ratio` gauge, Prometheus exporter
2. **MCP Proxy Vertical Slice** — JSON-RPC 2.0 parsing, Stdio/SSE transport abstraction, tool-to-server routing
3. **Semantic Caching** — Replace SHA256 hash with embedding-based vector search (requires RediSearch module)
4. **Streaming Support** — Handle `stream: true` by piping SSE chunks from provider to client
5. **Anthropic API Translation** — Convert OpenAI-format requests to Anthropic's Messages API format
6. **Context Pruning** — Strip irrelevant conversation history before forwarding to reduce token usage
