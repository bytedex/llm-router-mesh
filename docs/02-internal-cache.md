# Internal Cache Package (`internal/cache`)

**Target Audience:** AI Agents implementing state, caching, and rate limiting.

## Responsibility
The `cache` package wraps all interactions with Redis. It provides resiliency and massive cost savings.

## Redis Semantic Caching (`redis.go`)
- **Use Case:** Reduce LLM token usage to exactly **0** for duplicate questions.
- **Implementation Guidelines:**
  - When a prompt comes in, generate a fast embedding (or use a hashing mechanism).
  - Search Redis for a similar embedding (Vector Search).
  - If a >95% similarity match is found, return the cached LLM response instantly.

## Rate Limiting (`rate_limit.go`)
- **Use Case:** Protect downstream APIs (like GitHub or OpenAI) from being spammed and banning the gateway IP.
- **Implementation Guidelines:**
  - Use a Token Bucket algorithm stored in Redis.
  - Implement per-provider limits (e.g., `github-mcp: 100 req/min`, `openai: 500 req/min`).
  - If `Allow()` returns false, the Proxy layer should return a HTTP 429 Too Many Requests instantly.
