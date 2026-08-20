package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"keeper/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCpAgent 测试 cp 命令端到端流程
func TestCpAgent(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建源 agent
	require.NoError(t, createAgent(cfg, []string{"source-agent"}))

	// 写入一些文件到 workspace
	sourceWorkspace := filepath.Join(tmpDir, "agents", "source-agent", "workspace")
	require.NoError(t, os.MkdirAll(sourceWorkspace, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceWorkspace, "file.txt"), []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceWorkspace, "readme.md"), []byte("# README"), 0644))

	// 创建子目录和文件
	require.NoError(t, os.MkdirAll(filepath.Join(sourceWorkspace, "subdir"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceWorkspace, "subdir", "nested.txt"), []byte("nested"), 0644))

	// 1. 从 agent 复制单个文件到本地
	localFile := filepath.Join(tmpDir, "local-file.txt")
	require.NoError(t, copyFile(cfg, []string{fmt.Sprintf("source-agent:/file.txt"), localFile}))

	// 验证本地文件已复制
	content, err := os.ReadFile(localFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)

	// 2. 从本地复制文件到 agent
	targetWorkspace := filepath.Join(tmpDir, "agents", "source-agent", "workspace")
	require.NoError(t, copyFile(cfg, []string{localFile, fmt.Sprintf("source-agent:/uploaded.txt")}))

	// 验证文件已上传到 agent
	content, err = os.ReadFile(filepath.Join(targetWorkspace, "uploaded.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)

	// 3. 递归复制目录（从 agent 到本地）
	localDir := filepath.Join(tmpDir, "local-dir")
	require.NoError(t, copyFile(cfg, []string{"-r", "source-agent:/", localDir}))

	// 验证目录已复制
	assert.DirExists(t, localDir)
	assert.FileExists(t, filepath.Join(localDir, "file.txt"))
	assert.FileExists(t, filepath.Join(localDir, "readme.md"))
	assert.FileExists(t, filepath.Join(localDir, "subdir", "nested.txt"))
}

// TestCpAgentMissingArgs 测试 cp 命令缺少参数
func TestCpAgentMissingArgs(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 缺少参数
	err = copyFile(cfg, []string{"source-agent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage")

	// 缺少目标
	err = copyFile(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage")
}

// TestForkAndSnapshot 测试 fork 和 snapshot 组合流程
func TestForkAndSnapshot(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建源 agent
	require.NoError(t, createAgent(cfg, []string{"main-agent"}))

	// 写入一些文件到 workspace
	workspace := filepath.Join(tmpDir, "agents", "main-agent", "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "config.yaml"), []byte("key: value"), 0644))

	// fork agent
	require.NoError(t, forkAgent(cfg, []string{"main-agent", "forked-agent"}))

	// 验证 forked agent 存在
	forkedWorkspace := filepath.Join(tmpDir, "agents", "forked-agent", "workspace")
	assert.DirExists(t, forkedWorkspace)

	// 验证文件已复制
	content, err := os.ReadFile(filepath.Join(forkedWorkspace, "config.yaml"))
	require.NoError(t, err)
	assert.Equal(t, []byte("key: value"), content)

	// 在 forked agent 中创建快照
	require.NoError(t, snapshotAgent(cfg, []string{"forked-agent", "snap1"}))

	// 验证快照存在
	snapshotDir := filepath.Join(tmpDir, "agents", "forked-agent", "backups", "snap1")
	assert.DirExists(t, snapshotDir)
	assert.FileExists(t, filepath.Join(snapshotDir, "meta.json"))
}

// TestRollbackAndRecover 测试 rollback 和 recover 组合流程
func TestRollbackAndRecover(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建 agent
	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	// 写入初始文件
	workspace := filepath.Join(tmpDir, "agents", "test-agent", "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("original"), 0644))

	// 创建快照 v1
	require.NoError(t, snapshotAgent(cfg, []string{"test-agent", "v1"}))

	// 修改文件并创建快照 v2
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("modified"), 0644))
	require.NoError(t, snapshotAgent(cfg, []string{"test-agent", "v2"}))

	// 再次修改文件
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("modified again"), 0644))

	// 回滚到 v1
	require.NoError(t, rollbackAgent(cfg, []string{"test-agent", "v1"}))
	content, err := os.ReadFile(filepath.Join(workspace, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("original"), content)

	// 回滚到 v2
	require.NoError(t, rollbackAgent(cfg, []string{"test-agent", "v2"}))
	content, err = os.ReadFile(filepath.Join(workspace, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("modified"), content)

	// recover agent（重新创建）
	require.NoError(t, recoverAgent(cfg, []string{"test-agent"}))
}

// TestMultipleSnapshots 测试多个快照管理
func TestMultipleSnapshots(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建 agent
	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	// 写入初始文件
	workspace := filepath.Join(tmpDir, "agents", "test-agent", "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("v0"), 0644))

	// 创建多个快照
	snapshots := []string{"snap1", "snap2", "snap3", "snap4"}
	for i, snap := range snapshots {
		require.NoError(t, snapshotAgent(cfg, []string{"test-agent", snap}))

		// 修改文件
		require.NoError(t, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("v"+string(rune('0'+i+1))), 0644))
	}

	// 回滚到第一个快照
	require.NoError(t, rollbackAgent(cfg, []string{"test-agent", "snap1"}))
	content, err := os.ReadFile(filepath.Join(workspace, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("v0"), content)

	// 回滚到第三个快照
	require.NoError(t, rollbackAgent(cfg, []string{"test-agent", "snap3"}))
	content, err = os.ReadFile(filepath.Join(workspace, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("v2"), content)
}

// TestForkAfterSnapshot 测试快照后 fork
func TestForkAfterSnapshot(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建源 agent
	require.NoError(t, createAgent(cfg, []string{"source-agent"}))

	// 写入文件
	workspace := filepath.Join(tmpDir, "agents", "source-agent", "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "original.txt"), []byte("original"), 0644))

	// 创建快照
	require.NoError(t, snapshotAgent(cfg, []string{"source-agent", "before-fork"}))

	// 修改文件
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "original.txt"), []byte("modified"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "new.txt"), []byte("new file"), 0644))

	// fork agent
	require.NoError(t, forkAgent(cfg, []string{"source-agent", "forked-agent"}))

	// forked agent 应该包含修改后的文件
	forkedWorkspace := filepath.Join(tmpDir, "agents", "forked-agent", "workspace")
	content, err := os.ReadFile(filepath.Join(forkedWorkspace, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("modified"), content)

	content, err = os.ReadFile(filepath.Join(forkedWorkspace, "new.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("new file"), content)

	// 回滚快照到修改前的状态
	require.NoError(t, rollbackAgent(cfg, []string{"source-agent", "before-fork"}))
	content, err = os.ReadFile(filepath.Join(workspace, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("original"), content)

	// forked agent 不受影响
	content, err = os.ReadFile(filepath.Join(forkedWorkspace, "original.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("modified"), content)
}
