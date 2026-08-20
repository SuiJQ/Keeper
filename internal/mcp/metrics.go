// Package mcp 提供 MCP Server 指标监控
package mcp

import "keeper/internal/metrics"

// MCP 指标
var (
	MCPConnectionCounter = metrics.RegisterCounter("keeper_mcp_connection_total", "Total number of MCP connections", []string{"result"})
	MCPToolCallCounter   = metrics.RegisterCounter("keeper_mcp_tool_call_total", "Total number of MCP tool calls", []string{"tool", "result"})
	MCPToolCallDuration  = metrics.RegisterHistogram("keeper_mcp_tool_call_duration_seconds", "MCP tool call duration in seconds", nil, []string{"tool"})
	MCPActiveConnections = metrics.RegisterGauge("keeper_mcp_active_connections", "Number of currently active MCP connections", nil)
)

// RecordMCPConnection 记录 MCP 连接
func RecordMCPConnection(result string) {
	MCPConnectionCounter.Inc(result)
}

// RecordMCPToolCall 记录 MCP 工具调用
func RecordMCPToolCall(tool, result string) {
	MCPToolCallCounter.Inc(tool, result)
}

// RecordMCPToolCallDuration 记录 MCP 工具调用耗时
func RecordMCPToolCallDuration(tool string, duration float64) {
	MCPToolCallDuration.Observe(duration, tool)
}

// SetActiveMCPConnections 设置活跃 MCP 连接数
func SetActiveMCPConnections(count float64) {
	MCPActiveConnections.Set(count)
}
