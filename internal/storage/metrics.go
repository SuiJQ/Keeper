// Package storage 提供存储模块的指标监控
package storage

import "keeper/internal/metrics"

// 存储操作指标
var (
	SnapshotCreateCounter    = metrics.RegisterCounter("keeper_storage_snapshot_create_total", "Total number of snapshot creations", []string{"result"})
	SnapshotRollbackCounter  = metrics.RegisterCounter("keeper_storage_snapshot_rollback_total", "Total number of snapshot rollbacks", []string{"result"})
	SnapshotPruneCounter     = metrics.RegisterCounter("keeper_storage_snapshot_prune_total", "Total number of snapshot prune operations", []string{"result"})
	ForkCounter              = metrics.RegisterCounter("keeper_storage_fork_total", "Total number of agent forks", []string{"result"})
	CachePruneCounter        = metrics.RegisterCounter("keeper_storage_cache_prune_total", "Total number of cache prune operations", []string{"result"})
	SnapshotCreateDuration   = metrics.RegisterHistogram("keeper_storage_snapshot_create_duration_seconds", "Snapshot creation duration in seconds", nil, nil)
	SnapshotRollbackDuration = metrics.RegisterHistogram("keeper_storage_snapshot_rollback_duration_seconds", "Snapshot rollback duration in seconds", nil, nil)
	SnapshotPruneDuration    = metrics.RegisterHistogram("keeper_storage_snapshot_prune_duration_seconds", "Snapshot prune duration in seconds", nil, nil)
	ForkDuration             = metrics.RegisterHistogram("keeper_storage_fork_duration_seconds", "Agent fork duration in seconds", nil, nil)
	CachePruneDuration       = metrics.RegisterHistogram("keeper_storage_cache_prune_duration_seconds", "Cache prune duration in seconds", nil, nil)
)

// RecordSnapshotCreate 记录快照创建
func RecordSnapshotCreate(result string) {
	SnapshotCreateCounter.Inc(result)
}

// RecordSnapshotRollback 记录快照回滚
func RecordSnapshotRollback(result string) {
	SnapshotRollbackCounter.Inc(result)
}

// RecordSnapshotPrune 记录快照清理
func RecordSnapshotPrune(result string) {
	SnapshotPruneCounter.Inc(result)
}

// RecordFork 记录 agent fork
func RecordFork(result string) {
	ForkCounter.Inc(result)
}

// RecordCachePrune 记录缓存清理
func RecordCachePrune(result string) {
	CachePruneCounter.Inc(result)
}

// RecordSnapshotCreateDuration 记录快照创建耗时
func RecordSnapshotCreateDuration(duration float64) {
	SnapshotCreateDuration.Observe(duration)
}

// RecordSnapshotRollbackDuration 记录快照回滚耗时
func RecordSnapshotRollbackDuration(duration float64) {
	SnapshotRollbackDuration.Observe(duration)
}

// RecordSnapshotPruneDuration 记录快照清理耗时
func RecordSnapshotPruneDuration(duration float64) {
	SnapshotPruneDuration.Observe(duration)
}

// RecordForkDuration 记录 fork 耗时
func RecordForkDuration(duration float64) {
	ForkDuration.Observe(duration)
}

// RecordCachePruneDuration 记录缓存清理耗时
func RecordCachePruneDuration(duration float64) {
	CachePruneDuration.Observe(duration)
}
