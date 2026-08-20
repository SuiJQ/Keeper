package mcp

import (
	"testing"
)

// TestMCPMetrics 测试 MCP 指标函数不会 panic
func TestMCPMetrics(t *testing.T) {
	RecordMCPConnection("success")
	RecordMCPToolCall("test_tool", "success")
	RecordMCPToolCallDuration("test_tool", 0.5)
	SetActiveMCPConnections(1)
	SetActiveMCPConnections(0)
}

// TestMCPConnectionCounter 验证计数器可以多次调用
func TestMCPConnectionCounter(t *testing.T) {
	// 这里只是验证不会 panic，实际值通过 metrics 包内部验证
	for i := 0; i < 10; i++ {
		RecordMCPConnection("test")
	}
}

// TestMCPToolCallCounter 验证不同工具和结果的计数
func TestMCPToolCallCounter(t *testing.T) {
	tools := []string{"list", "inspect", "start"}
	results := []string{"success", "error"}
	
	for _, tool := range tools {
		for _, result := range results {
			RecordMCPToolCall(tool, result)
		}
	}
}

// TestMCPToolCallDuration 验证持续时间记录
func TestMCPToolCallDuration(t *testing.T) {
	durations := []float64{0.1, 0.5, 1.0, 2.5}
	for _, d := range durations {
		RecordMCPToolCallDuration("test_tool", d)
	}
}

// TestSetActiveMCPConnections 验证设置活跃连接数
func TestSetActiveMCPConnections(t *testing.T) {
	testValues := []float64{0, 1, 5, 10, 100}
	for _, v := range testValues {
		SetActiveMCPConnections(v)
	}
}

// TestMCPMetricsConcurrent 并发测试指标函数
func TestMCPMetricsConcurrent(t *testing.T) {
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			RecordMCPConnection("concurrent")
			RecordMCPToolCall("concurrent_tool", "success")
			RecordMCPToolCallDuration("concurrent_tool", 0.1)
			SetActiveMCPConnections(1)
			done <- true
		}()
	}
	
	for i := 0; i < 10; i++ {
		<-done
	}
}
