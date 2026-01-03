# Internal Proxy Package (`internal/proxy`)

**Target Audience:** AI Agents implementing the transport/network layer.

## Responsibility
The `proxy` package handles the raw bytes and protocols. It speaks HTTP/JSON-RPC and manages connections. It relies on the `router` and `cache` to know *what* to do, but this package dictates *how* to do it over the wire.

## LLM Proxy (`llm_proxy.go`)
- **Use Case:** Reverse HTTP proxy for OpenAI/Anthropic SDKs.
- **Implementation Guidelines:**
  - Accepts standard POST requests.
  - Pauses to check `cache.RateLimiter`.
  - Pauses to check `cache.RedisCache` for a semantic hit.
  - Calls `router.LLMRouter.Route()` to pick the model.
  - Rewrites the HTTP payload to point to the selected model and forwards it.

## MCP Proxy (`mcp_proxy.go`)
- **Use Case:** Proxying Model Context Protocol (JSON-RPC 2.0).
- **Implementation Guidelines:**
  - Supports standard input/output (Stdio) byte streams OR Server-Sent Events (SSE).
  - Inspects the JSON-RPC payload for `method: "tools/call"`.
  - Calls `router.MCPRouter.Route()` to find the correct downstream server.
  - Manages connection pools so we don't spin up a new process for every single tool call.
