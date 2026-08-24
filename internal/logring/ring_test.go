package logring

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewBuffer(t *testing.T) {
	b := NewBuffer(0)
	assert.NotNil(t, b)
	assert.Equal(t, 1024, b.capacity)
}

func TestAddAndSnapshot(t *testing.T) {
	b := NewBuffer(3)

	b.Add(Entry{Time: time.Now(), Level: "info", Message: "msg1"})
	b.Add(Entry{Time: time.Now(), Level: "info", Message: "msg2"})
	b.Add(Entry{Time: time.Now(), Level: "info", Message: "msg3"})

	snapshot, truncated := b.Snapshot(0)
	assert.False(t, truncated)
	assert.Len(t, snapshot, 3)
	assert.Equal(t, "msg1", snapshot[0].Message)
	assert.Equal(t, "msg2", snapshot[1].Message)
	assert.Equal(t, "msg3", snapshot[2].Message)
}

func TestOverwriteOldest(t *testing.T) {
	b := NewBuffer(2)

	b.Add(Entry{Time: time.Now(), Level: "info", Message: "msg1"})
	b.Add(Entry{Time: time.Now(), Level: "info", Message: "msg2"})
	b.Add(Entry{Time: time.Now(), Level: "info", Message: "msg3"})

	snapshot, _ := b.Snapshot(0)
	assert.Len(t, snapshot, 2)
	assert.Equal(t, "msg2", snapshot[0].Message)
	assert.Equal(t, "msg3", snapshot[1].Message)
	assert.Equal(t, int64(1), b.Dropped())
}

func TestSnapshotLimit(t *testing.T) {
	b := NewBuffer(10)

	for i := 0; i < 5; i++ {
		b.Add(Entry{Time: time.Now(), Level: "info", Message: string(rune('0' + i))})
	}

	snapshot, _ := b.Snapshot(3)
	assert.Len(t, snapshot, 3)
	assert.Equal(t, "2", snapshot[0].Message)
	assert.Equal(t, "3", snapshot[1].Message)
	assert.Equal(t, "4", snapshot[2].Message)
}
