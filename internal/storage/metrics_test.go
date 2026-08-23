package storage

import (
	"testing"
)

// TestStorageMetricsRecording 测试存储指标记录
func TestStorageMetricsRecording(t *testing.T) {
	RecordSnapshotCreate("success")
	RecordSnapshotCreate("error")
	RecordSnapshotRollback("success")
	RecordSnapshotRollback("error")
	RecordFork("success")
	RecordFork("error")
	RecordCachePrune("success")
	RecordCachePrune("error")

	RecordSnapshotCreateDuration(0.5)
	RecordSnapshotRollbackDuration(0.3)
	RecordForkDuration(0.8)
	RecordCachePruneDuration(0.2)
}

// TestStorageMetricsConcurrent 并发测试存储指标
func TestStorageMetricsConcurrent(t *testing.T) {
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(_ int) {
			RecordSnapshotCreate("success")
			RecordSnapshotRollback("success")
			RecordFork("success")
			RecordCachePrune("success")
			RecordSnapshotCreateDuration(0.1)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestStorageMetricsEdgeCases 测试边界情况
func TestStorageMetricsEdgeCases(t *testing.T) {
	// 测试零值
	RecordSnapshotCreateDuration(0)
	RecordSnapshotRollbackDuration(0)
	RecordForkDuration(0)
	RecordCachePruneDuration(0)

	// 测试大值
	RecordSnapshotCreateDuration(999999.0)
}
