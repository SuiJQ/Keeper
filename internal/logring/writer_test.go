package logring

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"keeper/internal/log"
)

func TestNewHook(t *testing.T) {
	b := NewBuffer(10)
	hook := NewHook(b)
	assert.NotNil(t, hook)
	assert.Equal(t, b, hook.buffer)
}

func TestNewHookNilBuffer(t *testing.T) {
	hook := NewHook(nil)
	assert.NotNil(t, hook)
	assert.Equal(t, DefaultBuffer, hook.buffer)
}

func TestOnLog(t *testing.T) {
	b := NewBuffer(10)
	hook := NewHook(b)

	entry := log.Entry{
		Time:    time.Now(),
		Level:   log.LevelInfo,
		Message: "hello",
		Fields:  map[string]string{"key": "value"},
	}

	hook.OnLog(entry)

	snapshot, _ := b.Snapshot(0)
	assert.Len(t, snapshot, 1)
	assert.Equal(t, "hello", snapshot[0].Message)
	assert.Equal(t, "info", snapshot[0].Level)
	assert.Equal(t, "value", snapshot[0].Fields["key"])
}

func TestDefaultHook(t *testing.T) {
	hook := DefaultHook()
	assert.NotNil(t, hook)
	assert.Equal(t, DefaultBuffer, hook.buffer)
}

func TestDefaultSnapshot(t *testing.T) {
	DefaultBuffer.Add(Entry{Time: time.Now(), Level: "info", Message: "default"})
	snapshot, truncated := DefaultSnapshot(10)
	assert.True(t, len(snapshot) > 0)
	assert.False(t, truncated)
}

func TestBufferSize(t *testing.T) {
	b := NewBuffer(5)
	assert.Equal(t, 0, b.Size())

	b.Add(Entry{Time: time.Now(), Level: "info", Message: "1"})
	assert.Equal(t, 1, b.Size())

	b.Add(Entry{Time: time.Now(), Level: "info", Message: "2"})
	assert.Equal(t, 2, b.Size())
}
