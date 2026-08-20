package container

import (
	"testing"
)

// TestContainerMetricsRecording 测试容器指标记录
func TestContainerMetricsRecording(t *testing.T) {
	// 记录各种容器操作
	RecordContainerStart("bwrap", "success")
	RecordContainerStart("bwrap", "error")
	RecordContainerStop("bwrap", "success")
	RecordContainerStop("bwrap", "error")
	RecordContainerExec("bwrap", "success")
	RecordContainerExec("bwrap", "error")
	
	// 记录持续时间
	RecordContainerStartDuration("bwrap", 0.5)
	RecordContainerStopDuration("bwrap", 0.3)
	RecordContainerExecDuration("bwrap", 1.2)
	
	// 设置活跃容器数
	SetContainerActive("bwrap", 1)
	SetContainerActive("bwrap", 0)
}

// TestContainerMetricsConcurrent 并发测试容器指标
func TestContainerMetricsConcurrent(t *testing.T) {
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			runtime := "bwrap"
			RecordContainerStart(runtime, "success")
			RecordContainerStop(runtime, "success")
			RecordContainerExec(runtime, "success")
			RecordContainerStartDuration(runtime, 0.1)
			SetContainerActive(runtime, float64(id))
			done <- true
		}(i)
	}
	
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestContainerMetricsLabels 测试指标标签
func TestContainerMetricsLabels(t *testing.T) {
	runtimes := []string{"bwrap", "kubevirt", "mock"}
	results := []string{"success", "error"}
	
	for _, rt := range runtimes {
		for _, result := range results {
			RecordContainerStart(rt, result)
			RecordContainerStop(rt, result)
			RecordContainerExec(rt, result)
			RecordContainerStartDuration(rt, 0.5)
			RecordContainerStopDuration(rt, 0.3)
			RecordContainerExecDuration(rt, 1.0)
			SetContainerActive(rt, 1)
		}
	}
}

// TestContainerMetricsEdgeCases 测试边界情况
func TestContainerMetricsEdgeCases(t *testing.T) {
	// 测试零值
	RecordContainerStartDuration("bwrap", 0)
	RecordContainerStopDuration("bwrap", 0)
	RecordContainerExecDuration("bwrap", 0)
	
	// 测试大值
	RecordContainerStartDuration("bwrap", 999999.0)
	
	// 测试空字符串 runtime
	RecordContainerStart("", "success")
	RecordContainerStop("", "success")
	RecordContainerExec("", "success")
}

// TestContainerErrorMetrics 测试容器错误指标
func TestContainerErrorMetrics(t *testing.T) {
	RecordContainerError("bwrap", "start", "permission_denied")
	RecordContainerError("bwrap", "start", "kernel_not_supported")
	RecordContainerError("bwrap", "stop", "timeout")
	RecordContainerError("mock", "exec", "command_not_found")
}

// TestContainerErrorMetricsConcurrent 并发测试容器错误指标
func TestContainerErrorMetricsConcurrent(t *testing.T) {
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			errorType := []string{"timeout", "permission", "kernel", "unknown"}[id%4]
			RecordContainerError("bwrap", "start", errorType)
			done <- true
		}(i)
	}
	
	for i := 0; i < 10; i++ {
		<-done
	}
}
