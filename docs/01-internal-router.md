# Internal Router Package (`internal/router`)

**Target Audience:** AI Agents implementing routing logic.

## Responsibility
The `router` package is the "Brain" of the gateway. It never touches raw network connections, and it never touches the cache directly. Its only job is to evaluate a string (prompt or tool name) and return a routing decision.

## LLM Router (`llm_router.go`)
- **Use Case:** Evaluate a prompt's complexity. 
- **Implementation Guidelines:**
  - Simple keyword heuristics (e.g., "format", "fix typo") -> route to cheap model (e.g., `llama-3`).
  - Complex reasoning heuristics (e.g., "architecture", "plan", "debug context") -> route to frontier model (e.g., `claude-3.5-sonnet`).
  - Can be extended to use a fast, local lightweight model to classify intents later.

## MCP Router (`mcp_router.go`)
- **Use Case:** Evaluate a requested tool name or description and map it to a specific MCP server.
- **Implementation Guidelines:**
  - If the LLM requests `execute_sql`, the router maps this to the `postgres-mcp-server`.
  - If the LLM requests `read_repo`, the router maps this to the `github-mcp-server`.
  - Avoid broadcasting tool requests to all servers to prevent context bloat and rate limits.
