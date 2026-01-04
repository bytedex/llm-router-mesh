# Internal Telemetry & Config

**Target Audience:** AI Agents implementing configuration and observability.

## Telemetry Package (`internal/telemetry`)
- **Use Case:** Give visibility into the black-box of LLM/Agent operations.
- **Implementation Guidelines:**
  - Use `go.opentelemetry.io/otel`.
  - Expose Metrics:
    - `llm_cost_saved_usd`: Counter tracking cost avoided via cache hits or routing to cheaper models.
    - `mcp_request_duration_ms`: Histogram of tool execution latency.
    - `cache_hit_ratio`: Gauge of cache efficiency.
  - Export data in OTLP format to be scraped by Prometheus.

## Config Package (`internal/config`)
- **Use Case:** Provide environment-aware configurations.
- **Implementation Guidelines:**
  - Do not hardcode API keys or Redis URLs.
  - Read from `config.yaml` or `.env` using standard Go libraries (e.g., `viper`).
  - Structure must cleanly represent rate limits, routing rules, and downstream server addresses.
