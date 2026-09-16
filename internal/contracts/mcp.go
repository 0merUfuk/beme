package contracts

// Agent-facing MCP tool surface (ADR-009, FR-040/041). The server registers
// exactly MCPTools; administrative verbs never appear here.
const (
	ToolResolveContext = "beme.resolve_context"
	ToolGetContextItem = "beme.get_context_item"
	ToolReportFeedback = "beme.report_feedback"
	ToolStatus         = "beme.status"
)

// MCPTools is the complete tool list, in registration order.
var MCPTools = []string{ToolResolveContext, ToolGetContextItem, ToolReportFeedback, ToolStatus}
