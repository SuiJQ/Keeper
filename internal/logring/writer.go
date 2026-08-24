// Package logring provides a thread-safe fixed-size ring buffer for log entries
// and helpers to keep the in-memory buffer in sync with the structured logger.
package logring

import (
	"sync"

	"keeper/internal/log"
	"keeper/internal/metrics"
)

// Hook 将日志条目写入环形缓冲区的钩子
type Hook struct {
	mu     sync.Mutex
	buffer *Buffer
}

// NewHook 创建日志钩子
func NewHook(buffer *Buffer) *Hook {
	if buffer == nil {
		buffer = DefaultBuffer
	}
	return &Hook{buffer: buffer}
}

// OnLog 将结构化日志条目写入环形缓冲区
func (h *Hook) OnLog(entry log.Entry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.buffer.Add(Entry{
		Time:    entry.Time,
		Level:   string(entry.Level),
		Message: entry.Message,
		Fields:  entry.Fields,
	})
	metrics.RingBufferSize.Set(float64(h.buffer.Size()))
}

// DefaultBuffer 返回全局默认环形缓冲区（容量 4096）
var DefaultBuffer = NewBuffer(4096)

// DefaultHook 返回默认全局钩子
func DefaultHook() *Hook {
	return NewHook(DefaultBuffer)
}

// DefaultSnapshot 读取默认缓冲区的快照
func DefaultSnapshot(limit int) ([]Entry, bool) {
	return DefaultBuffer.Snapshot(limit)
}
