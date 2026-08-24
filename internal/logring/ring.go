// Package logring provides a thread-safe fixed-size ring buffer for log entries.
package logring

import (
	"sync"
	"time"

	"keeper/internal/metrics"
)

// Entry 表示一条内存日志条目
type Entry struct {
	Time    time.Time
	Level   string
	Message string
	Fields  map[string]string
}

// Buffer 固定容量环形缓冲区
type Buffer struct {
	mu       sync.Mutex
	entries  []Entry
	capacity int
	head     int
	size     int
	dropped  int64
}

// NewBuffer 创建环形缓冲区
func NewBuffer(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = 1024
	}
	return &Buffer{
		entries:  make([]Entry, capacity),
		capacity: capacity,
	}
}

// Add 追加一条日志条目；缓冲区满时覆盖最旧条目并累加 dropped 计数
func (b *Buffer) Add(entry Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.size < b.capacity {
		b.entries[b.head] = entry
		b.head = (b.head + 1) % b.capacity
		b.size++
	} else {
		b.entries[b.head] = entry
		b.head = (b.head + 1) % b.capacity
		b.dropped++
		metrics.RingBufferDropped.Inc()
	}
}

// Snapshot 返回最近 N 条日志的快照（按时间正序），以及是否发生截断
func (b *Buffer) Snapshot(limit int) ([]Entry, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if limit <= 0 || limit > b.size {
		limit = b.size
	}
	if limit > b.capacity {
		limit = b.capacity
	}

	result := make([]Entry, 0, limit)
	start := (b.head - limit + b.capacity) % b.capacity
	for i := 0; i < limit; i++ {
		idx := (start + i) % b.capacity
		result = append(result, b.entries[idx])
	}
	return result, b.dropped > 0
}

// Dropped 返回累计丢弃条目数
func (b *Buffer) Dropped() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

// Size 返回当前条目数
func (b *Buffer) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.size
}
