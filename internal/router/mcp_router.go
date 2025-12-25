package router

// MCPRouter inspects tool calls and routes them strictly to the relevant MCP servers
type MCPRouter struct {
}

// NewMCPRouter creates a new MCPRouter
func NewMCPRouter() *MCPRouter {
	return &MCPRouter{}
}

// Route determines the downstream MCP server based on tool intent
func (r *MCPRouter) Route(toolName string) string {
	// TODO: Implement vector/keyword matching for tool descriptions
	return "default-mcp-server"
}
