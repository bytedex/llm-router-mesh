# Architecture Overview

**Target Audience:** AI Agents & Developers working on the `llm-router-mesh` codebase.

## The Purpose of LLM Router Mesh
This repository acts as a unified proxy gateway. It sits between AI clients (IDE extensions, chat bots, autonomous agents) and downstream systems (LLM APIs and MCP tool servers).

It solves two primary problems:
1. **Token/Cost Bloat:** Dynamically routing prompts to cheaper LLMs when possible, and using semantic caching to skip LLM calls entirely for duplicate queries.
2. **Context Window Explosion:** Preventing an LLM from needing 50+ tool descriptions in its context by strictly routing tool calls to the single relevant MCP server.

## Data Flow
1. **Incoming Request:** Client sends a request (either an LLM prompt or an MCP tool execution).
2. **Proxy Layer (`internal/proxy`):** Intercepts the raw bytes (HTTP for LLMs, Stdio/SSE for MCP).
3. **Cache Check (`internal/cache`):** Checks Redis to see if we have a semantic hit for the prompt. If yes, returns early. Also applies Token Bucket rate limits to ensure downstream stability.
4. **Intent Routing (`internal/router`):** Analyzes the payload to determine the optimal target (e.g., Llama3 vs GPT-4o, or Postgres MCP vs GitHub MCP).
5. **Telemetry (`internal/telemetry`):** Every step is wrapped in OpenTelemetry spans to measure `tokens_saved`, `latency_ms`, and `cache_hit_rate`.

## Future Agent Instructions
When implementing new features, ensure you adhere to the separation of concerns. Do not put business logic (like routing) inside the transport layer (proxy).
