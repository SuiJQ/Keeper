package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-" + t.Name() + ".sock",
		AgentName:  "test",
	}
	server, err := NewServer(cfg, nil)
	requireNoError(t, err)
	return server
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateAllowedUIDs(t *testing.T) {
	server := newTestServer(t)
	defer func() { _ = server.Stop() }()

	// 初始状态应该是空的
	server.mu.Lock()
	initialUIDs := server.allowedUIDs
	server.mu.Unlock()
	assert.Empty(t, initialUIDs)

	// 更新 UID 白名单
	uids := []uint32{1000, 1001, 1002}
	server.UpdateAllowedUIDs(uids)

	// 验证更新结果
	server.mu.Lock()
	defer server.mu.Unlock()
	assert.Len(t, server.allowedUIDs, 3)
	assert.Contains(t, server.allowedUIDs, uint32(1000))
	assert.Contains(t, server.allowedUIDs, uint32(1001))
	assert.Contains(t, server.allowedUIDs, uint32(1002))
}

func TestUpdateAllowedGIDs(t *testing.T) {
	server := newTestServer(t)
	defer func() { _ = server.Stop() }()

	// 初始状态应该是空的
	server.mu.Lock()
	initialGIDs := server.allowedGIDs
	server.mu.Unlock()
	assert.Empty(t, initialGIDs)

	// 更新 GID 白名单
	gids := []uint32{1000, 1001}
	server.UpdateAllowedGIDs(gids)

	// 验证更新结果
	server.mu.Lock()
	defer server.mu.Unlock()
	assert.Len(t, server.allowedGIDs, 2)
	assert.Contains(t, server.allowedGIDs, uint32(1000))
	assert.Contains(t, server.allowedGIDs, uint32(1001))
}

func TestUpdateAllowedUIDsEmpty(t *testing.T) {
	server := newTestServer(t)
	defer func() { _ = server.Stop() }()

	// 先设置一些值
	server.UpdateAllowedUIDs([]uint32{1000})
	server.UpdateAllowedGIDs([]uint32{1000})

	// 更新为空列表
	server.UpdateAllowedUIDs([]uint32{})
	server.UpdateAllowedGIDs([]uint32{})

	// 验证清空结果
	server.mu.Lock()
	defer server.mu.Unlock()
	assert.Empty(t, server.allowedUIDs)
	assert.Empty(t, server.allowedGIDs)
}

func TestUpdateAllowedUIDsOverwrite(t *testing.T) {
	server := newTestServer(t)
	defer func() { _ = server.Stop() }()

	// 设置初始值
	server.UpdateAllowedUIDs([]uint32{1000, 1001})

	// 覆盖为新值
	server.UpdateAllowedUIDs([]uint32{2000, 2001, 2002})

	// 验证旧值被清除
	server.mu.Lock()
	defer server.mu.Unlock()
	assert.Len(t, server.allowedUIDs, 3)
	assert.NotContains(t, server.allowedUIDs, uint32(1000))
	assert.NotContains(t, server.allowedUIDs, uint32(1001))
	assert.Contains(t, server.allowedUIDs, uint32(2000))
	assert.Contains(t, server.allowedUIDs, uint32(2001))
	assert.Contains(t, server.allowedUIDs, uint32(2002))
}

func TestUpdateAllowedUIDsConcurrent(t *testing.T) {
	server := newTestServer(t)
	defer func() { _ = server.Stop() }()

	// 并发更新
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id uint32) {
			server.UpdateAllowedUIDs([]uint32{id, id + 1000})
			done <- true
		}(safeUint32(i))
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
