package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	meta, err := store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)
	assert.Equal(t, "test-agent", meta.Name)
	assert.Equal(t, "created", meta.State)
	assert.NotEmpty(t, meta.CreatedAt)
	assert.DirExists(t, meta.RootfsDir)
	assert.DirExists(t, meta.UpperDir)
	assert.DirExists(t, meta.WorkDir)
	assert.DirExists(t, meta.Workspace)
}

func TestCreateAgentDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 重复创建应失败
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestGetAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	created, err := store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	got, err := store.GetAgent(ctx, "test-agent")
	require.NoError(t, err)
	assert.Equal(t, created.Name, got.Name)
	assert.Equal(t, created.State, got.State)
}

func TestGetAgentNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.GetAgent(ctx, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListAgents(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "agent-a", 64, 1024*1024*1024)
	require.NoError(t, err)
	_, err = store.CreateAgent(ctx, "agent-b", 64, 1024*1024*1024)
	require.NoError(t, err)

	agents, err := store.ListAgents(ctx)
	require.NoError(t, err)
	assert.Len(t, agents, 2)
}

func TestDeleteAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	err = store.DeleteAgent(ctx, "test-agent")
	require.NoError(t, err)

	// 确认已删除
	_, err = store.GetAgent(ctx, "test-agent")
	require.Error(t, err)
}

func TestForkAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "source", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入一些文件到 upper 目录
	upperFile := filepath.Join(tmpDir, "agents", "source", "upper", "test.txt")
	err = os.WriteFile(upperFile, []byte("hello"), 0644)
	require.NoError(t, err)

	// Fork
	forked, err := store.ForkAgent(ctx, "source", "target")
	require.NoError(t, err)
	assert.Equal(t, "target", forked.Name)
	assert.Equal(t, "created", forked.State)
	assert.NotEmpty(t, forked.CreatedAt)

	// 确认文件已复制
	targetFile := filepath.Join(tmpDir, "agents", "target", "upper", "test.txt")
	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)

	// 确认 work 目录存在但是空的（启动时自动重建，不复制源内容）
	workDir := filepath.Join(tmpDir, "agents", "target", "work")
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "work directory should be empty")

	// 确认运行时文件已清理
	runtimeFiles := []string{"pgid", ".watchdog", ".api_sock", ".forward_sock"}
	for _, f := range runtimeFiles {
		_, err = os.Stat(filepath.Join(tmpDir, "agents", "target", f))
		assert.True(t, os.IsNotExist(err), "runtime file %s should be removed", f)
	}

	// 确认 ports.json 已重置
	portsPath := filepath.Join(tmpDir, "agents", "target", "ports.json")
	content, err = os.ReadFile(portsPath)
	require.NoError(t, err)
	assert.Equal(t, "[]\n", string(content))
}

func TestForkAgentInvalidState(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	meta, err := store.CreateAgent(ctx, "source", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 模拟 running 状态
	meta.State = "running"
	err = store.UpdateAgent(ctx, meta)
	require.NoError(t, err)

	// Fork 应该失败
	_, err = store.ForkAgent(ctx, "source", "target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot fork agent in state 'running'")
}

func TestForkAgentTargetExists(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "source", 64, 1024*1024*1024)
	require.NoError(t, err)
	_, err = store.CreateAgent(ctx, "target", 64, 1024*1024*1024)
	require.NoError(t, err)

	_, err = store.ForkAgent(ctx, "source", "target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入一些文件
	upperFile := filepath.Join(tmpDir, "agents", "test-agent", "upper", "data.txt")
	err = os.WriteFile(upperFile, []byte("snapshot data"), 0644)
	require.NoError(t, err)

	// 创建快照
	err = store.CreateSnapshot(ctx, "test-agent", "snap1")
	require.NoError(t, err)

	// 验证快照目录存在
	snapshotDir := filepath.Join(tmpDir, "agents", "test-agent", "backups", "snap1")
	assert.DirExists(t, snapshotDir)
}

func TestRollbackSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入初始文件
	upperFile := filepath.Join(tmpDir, "agents", "test-agent", "upper", "data.txt")
	err = os.WriteFile(upperFile, []byte("original"), 0644)
	require.NoError(t, err)

	// 创建快照
	err = store.CreateSnapshot(ctx, "test-agent", "snap1")
	require.NoError(t, err)

	// 修改文件
	err = os.WriteFile(upperFile, []byte("modified"), 0644)
	require.NoError(t, err)

	// 回滚
	err = store.RollbackSnapshot(ctx, "test-agent", "snap1")
	require.NoError(t, err)

	// 验证文件恢复
	content, err := os.ReadFile(upperFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("original"), content)
}

func TestPruneCache(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "agent-a", 64, 1024*1024*1024)
	require.NoError(t, err)
	_, err = store.CreateAgent(ctx, "agent-b", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 创建缓存目录
	cacheDir := filepath.Join(tmpDir, "cache", "rootfs")
	os.MkdirAll(filepath.Join(cacheDir, "used-cache"), 0700)
	os.MkdirAll(filepath.Join(cacheDir, "unused-cache"), 0700)

	// 设置 agent-a 使用 used-cache
	meta, _ := store.GetAgent(ctx, "agent-a")
	meta.CacheKey = "used-cache"
	store.UpdateAgent(ctx, meta)

	// Prune
	deleted, err := store.PruneCache(ctx, false)
	require.NoError(t, err)
	assert.Contains(t, deleted, "unused-cache")
	assert.NotContains(t, deleted, "used-cache")
}

func TestAgentMetaPaths(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	meta, err := store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 验证路径是否正确填充
	assert.Equal(t, filepath.Join(tmpDir, "agents", "test-agent", "rootfs"), meta.RootfsDir)
	assert.Equal(t, filepath.Join(tmpDir, "agents", "test-agent", "upper"), meta.UpperDir)
	assert.Equal(t, filepath.Join(tmpDir, "agents", "test-agent", "work"), meta.WorkDir)
	assert.Equal(t, filepath.Join(tmpDir, "agents", "test-agent", "workspace"), meta.Workspace)
}
