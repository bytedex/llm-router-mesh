# LLM Router Mesh 🕸️

A high-performance, distributed gateway for routing LLM (Large Language Model) and MCP (Model Context Protocol) traffic. It acts as an intelligent proxy between your applications and various downstream LLM providers, optimizing for cost, latency, and reliability.

## 🚀 Key Features

* **Intelligent Provider Routing:** Automatically route prompts to different tiers of providers (e.g., Frontier, Mid, Cheap) based on keyword complexity and default configurations.
* **Semantic Vector Caching:** Built-in Redis-backed semantic cache using vector embeddings (`RediSearch`) to serve identical or semantically similar prompts with near-zero latency and cost.
* **Real-time SSE Streaming:** Full support for Server-Sent Events (SSE) streaming, ensuring smooth, chunked token delivery back to the client via `http.Flusher`.
* **Automatic Failover & Circuit Breaking:** Uses `gobreaker` to detect provider degradation (e.g., 5xx errors) and automatically fall back to secondary providers in the routing tier.
* **Token Bucket Rate Limiting:** Lua-script powered Redis rate limiter to prevent aggressive traffic spikes and enforce tenant-level or global limits.
* **MCP Proxy Support:** First-class support for proxying Model Context Protocol (MCP) requests.
* **OpenTelemetry Observability:** Pre-instrumented for metrics and distributed tracing.

---

## 📚 Deep Dive Documentation

For a comprehensive understanding of how specific subsystems work, please refer to our internal engineering docs:

* **[Architecture Overview](docs/00-architecture.md):** The high-level design, flow of requests, and system boundaries.
* **[Internal Router](docs/01-internal-router.md):** How complexity keyword matching and fallback slices are evaluated.
* **[Internal Cache](docs/02-internal-cache.md):** The RediSearch vector implementation, similarity scoring, and cache hydration strategy.
* **[Internal Proxy](docs/03-internal-proxy.md):** Deep dive into the HTTP transport layer, SSE handling, and circuit breaker logic.
* **[Telemetry & Config](docs/04-internal-telemetry-config.md):** Configuration schemas and OpenTelemetry metrics setup.

---

## 🛠️ Initial Setup Guide

### Prerequisites
- **Go 1.21+** installed locally.
- **Redis Stack** (Required for the Vector Search / Semantic Caching features).
- Valid API keys for your configured LLM providers (e.g., OpenAI, Anthropic).

### 1. Running via Docker Compose (Recommended for Dev)

The easiest way to get the entire stack (Router + Redis Stack) running locally is using Docker Compose.

```bash
# 1. Clone the repository
git clone https://github.com/arnabdutta/llm-router-mesh.git
cd llm-router-mesh

# 2. Setup your configuration file
cp config.example.yaml config.yaml
# Edit config.yaml to insert your actual provider API Keys and Embedding keys

# 3. Spin up the infrastructure
docker-compose up -d
```
The router will now be listening for requests at `http://localhost:8080`.

### 2. Running Locally (Bare Metal)

If you prefer to run the Go binary directly on your machine:

```bash
# 1. Ensure you have a Redis Stack instance running locally (port 6379)
docker run -d -p 6379:6379 redis/redis-stack-server:latest

# 2. Setup configuration
cp config.example.yaml config.yaml
# Edit config.yaml with your keys and ensure redis_url points to localhost:6379

# 3. Install dependencies and run
go mod download
go run cmd/llm-router-mesh/main.go
```

### 3. Making your first Request

You can hit the proxy exactly as you would hit the standard OpenAI Chat Completions endpoint:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Explain quantum computing."}],
    "stream": true
  }'
```
The Router Mesh will intercept this, check the semantic cache, apply rate limits, route to the cheapest capable provider, and stream the response back!

---
## 📄 License
MIT License
