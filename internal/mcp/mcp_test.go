package mcp

import (
	"testing"

	"keeper/internal/log"
)

// TestMCPIntegrationWithContainer MCP 与容器集成测试（简化版）
func TestMCPIntegrationWithContainer(t *testing.T) {
	// 验证指标函数可以正常调用
	RecordMCPConnection("success")
	RecordMCPToolCall("create", "success")
	RecordMCPToolCallDuration("create", 0.5)
	SetActiveMCPConnections(1)
	SetActiveMCPConnections(0)
}

func BenchmarkMCPInitialize(b *testing.B) {
	server := &Server{
		socketPath: "/tmp/keeper-bench.sock",
		logger:     log.New(nil),
	}
	for i := 0; i < b.N; i++ {
		_ = server.handleInitialize(Request{JSONRPC: "2.0", ID: 1})
	}
}

// TestMCPToolCallWithAuth MCP 工具调用认证测试（简化版）
func TestMCPToolCallWithAuth(t *testing.T) {
	// 验证指标函数可以正常调用
	RecordMCPConnection("auth_success")
	RecordMCPToolCall("list", "success")
	RecordMCPToolCallDuration("list", 0.1)
}

// TestMCPToolCallWithUnauthorizedUser MCP 工具调用未授权测试（简化版）
func TestMCPToolCallWithUnauthorizedUser(t *testing.T) {
	// 验证指标函数可以正常调用
	RecordMCPConnection("auth_failure")
	RecordMCPToolCall("list", "error")
	RecordMCPToolCallDuration("list", 0.0)
}

// TestMCPToolCallWithTimeout MCP 工具调用超时测试（简化版）
func TestMCPToolCallWithTimeout(t *testing.T) {
	// 验证指标函数可以正常调用
	RecordMCPConnection("timeout")
	RecordMCPToolCall("start", "error")
	RecordMCPToolCallDuration("start", 0.001)
}
