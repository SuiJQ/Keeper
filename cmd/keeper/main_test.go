package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"keeper/internal/storage"
	"keeper/pkg/config"
)

// helper: 创建临时配置
func setupTestConfig(t *testing.T) (string, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Home = tmpDir
	require.NoError(t, cfg.Save())
	return tmpDir, func() {}
}

func TestCreateAgent(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	args := []string{"test-agent"}
	err = createAgent(cfg, args)
	require.NoError(t, err)

	// 验证目录创建
	agentDir := filepath.Join(tmpDir, "agents", "test-agent")
	assert.DirExists(t, agentDir)
	assert.FileExists(t, filepath.Join(agentDir, "meta.json"))
	assert.FileExists(t, filepath.Join(agentDir, "ports.json"))
}

func TestCreateAgentInvalidName(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	cases := []string{
		"-invalid",
		"name with spaces",
		"verylongnameverylongnameverylongnameverylongname",
		"",
	}

	for _, name := range cases {
		err := createAgent(cfg, []string{name})
		assert.Error(t, err, "name %q should fail", name)
	}
}

func TestDestroyAgent(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 先创建
	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	// 销毁
	err = destroyAgent(cfg, []string{"test-agent"})
	require.NoError(t, err)

	// 验证已删除
	agentDir := filepath.Join(tmpDir, "agents", "test-agent")
	assert.NoDirExists(t, agentDir)
}

func TestListAgents(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建两个 agent
	require.NoError(t, createAgent(cfg, []string{"agent-a"}))
	require.NoError(t, createAgent(cfg, []string{"agent-b"}))

	// 捕获输出
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = listAgents(cfg, nil)

	w.Close()
	os.Stdout = oldStdout

	require.NoError(t, err)
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "agent-a")
	assert.Contains(t, output, "agent-b")
}

func TestInspectAgent(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = inspectAgent(cfg, []string{"test-agent"})

	w.Close()
	os.Stdout = oldStdout

	require.NoError(t, err)
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "NAME:       test-agent")
	assert.Contains(t, output, "STATE:      created")
	assert.Contains(t, output, "ROOTFS:")
}

func TestForkAgent(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建源 agent
	require.NoError(t, createAgent(cfg, []string{"source"}))

	// Fork
	err = forkAgent(cfg, []string{"source", "target"})
	require.NoError(t, err)

	// 验证目标创建
	targetDir := filepath.Join(tmpDir, "agents", "target")
	assert.DirExists(t, targetDir)
	assert.FileExists(t, filepath.Join(targetDir, "meta.json"))

	// 验证 state 为 created
	metaPath := filepath.Join(targetDir, "meta.json")
	data, _ := os.ReadFile(metaPath)
	assert.Contains(t, string(data), `"state": "created"`)
}

func TestCopyFile(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建 agent
	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	// 创建源文件
	srcFile := filepath.Join(tmpDir, "source.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("hello"), 0644))

	// 复制到 agent workspace
	err = copyFile(cfg, []string{srcFile, "test-agent:/workspace/dest.txt"})
	require.NoError(t, err)

	// 验证
	destPath := filepath.Join(tmpDir, "agents", "test-agent", "workspace", "dest.txt")
	assert.FileExists(t, destPath)
	content, _ := os.ReadFile(destPath)
	assert.Equal(t, "hello", string(content))
}

func TestPrintUsage(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	assert.Contains(t, output, "keeper - AI Agent 轻量级 Linux 运行时环境")
	assert.Contains(t, output, "keeper create")
	assert.Contains(t, output, "keeper fork")
	assert.Contains(t, output, "keeper cp")
}

func TestRecoverAgent(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建 agent
	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	// 启动一个子进程用于测试 killProcess
	cmd := exec.Command("sleep", "10")
	err = cmd.Start()
	require.NoError(t, err)
	defer cmd.Process.Kill()

	// 模拟 agent 处于错误状态
	store, _ := storage.NewStore(cfg.Home)
	meta, _ := store.GetAgent(context.Background(), "test-agent")
	meta.State = "fatal_bwrap_exec"
	meta.PID = cmd.Process.Pid
	meta.Error = "test error"
	store.UpdateAgent(context.Background(), meta)

	// 恢复 agent
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err = recoverAgent(cfg, []string{"test-agent"})

	w.Close()
	os.Stdout = oldStdout
	require.NoError(t, err)

	// 验证状态已重置
	meta, _ = store.GetAgent(context.Background(), "test-agent")
	assert.Equal(t, "stopped", meta.State)
	assert.Equal(t, 0, meta.PID)
	assert.Empty(t, meta.Error)
}

func TestStopAgent(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建 agent
	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	// 模拟 agent 处于 running 状态
	store, _ := storage.NewStore(cfg.Home)
	meta, _ := store.GetAgent(context.Background(), "test-agent")
	meta.State = "running"
	meta.PID = os.Getpid()
	store.UpdateAgent(context.Background(), meta)

	// 停止 agent
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err = stopAgent(cfg, []string{"test-agent"})

	w.Close()
	os.Stdout = oldStdout
	require.NoError(t, err)

	// 验证状态已更新
	meta, _ = store.GetAgent(context.Background(), "test-agent")
	assert.Equal(t, "stopped", meta.State)
}

func TestCopyDirRecursive(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	_, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建源目录结构
	srcDir := filepath.Join(tmpDir, "src")
	dstDir := filepath.Join(tmpDir, "dst")

	err = os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("hello"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(srcDir, "subdir", "nested.txt"), []byte("world"), 0644)
	require.NoError(t, err)

	// 递归复制
	err = copyDirRecursive(srcDir, dstDir)
	require.NoError(t, err)

	// 验证文件已复制
	assert.FileExists(t, filepath.Join(dstDir, "file.txt"))
	assert.FileExists(t, filepath.Join(dstDir, "subdir", "nested.txt"))

	content, _ := os.ReadFile(filepath.Join(dstDir, "file.txt"))
	assert.Equal(t, []byte("hello"), content)
}

func TestStartAgentMissingName(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	err = startAgent(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper start <name>")
}

func TestStopAgentMissingName(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	err = stopAgent(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper stop <name>")
}

func TestStatusAgentMissingName(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	err = statusAgent(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper status <name>")
}

func TestRecoverAgentMissingName(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	err = recoverAgent(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper recover <name>")
}

func TestSnapshotAgentMissingArgs(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	err = snapshotAgent(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper snapshot <name> <snapshot-id>")

	err = snapshotAgent(cfg, []string{"name"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper snapshot <name> <snapshot-id>")
}

func TestRollbackAgentMissingArgs(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	err = rollbackAgent(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper rollback <name> <snapshot-id>")

	err = rollbackAgent(cfg, []string{"name"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper rollback <name> <snapshot-id>")
}

func TestSnapshotAndRollback(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建 agent
	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	// 写入一些文件到 workspace
	workspace := filepath.Join(tmpDir, "agents", "test-agent", "workspace")
	err = os.MkdirAll(workspace, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("hello"), 0644)
	require.NoError(t, err)

	// 创建快照
	err = snapshotAgent(cfg, []string{"test-agent", "snap1"})
	require.NoError(t, err)

	// 验证快照已创建
	snapshotDir := filepath.Join(tmpDir, "agents", "test-agent", "backups", "snap1")
	assert.DirExists(t, snapshotDir)
	// 快照是目录复制，不是 tar 文件
	assert.DirExists(t, filepath.Join(snapshotDir, "upper"))
	assert.DirExists(t, filepath.Join(snapshotDir, "workspace"))
	assert.FileExists(t, filepath.Join(snapshotDir, "workspace", "file.txt"))

	// 修改文件
	err = os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("modified"), 0644)
	require.NoError(t, err)

	// 回滚快照
	err = rollbackAgent(cfg, []string{"test-agent", "snap1"})
	require.NoError(t, err)

	// 验证文件已恢复
	content, err := os.ReadFile(filepath.Join(workspace, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)
}

func TestStartStopStatusRecover(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建 agent
	require.NoError(t, createAgent(cfg, []string{"test-agent"}))

	// 尝试启动（当前环境可能不支持 bwrap，会进入 fatal 状态）
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = startAgent(cfg, []string{"test-agent"})

	w.Close()
	os.Stdout = oldStdout

	// 启动可能因内核不支持而失败，这是预期行为
	store, _ := storage.NewStore(cfg.Home)
	var meta *storage.AgentMeta
	if err != nil {
		// 验证状态已更新为 fatal
		meta, err = store.GetAgent(context.Background(), "test-agent")
		require.NoError(t, err)
		assert.Contains(t, meta.State, "fatal")
	} else {
		// 如果启动成功，验证状态
		meta, err = store.GetAgent(context.Background(), "test-agent")
		require.NoError(t, err)
		assert.Equal(t, "running", meta.State)
	}

	// 状态查询
	r, w, _ = os.Pipe()
	os.Stdout = w

	err = statusAgent(cfg, []string{"test-agent"})

	w.Close()
	os.Stdout = oldStdout
	require.NoError(t, err)

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	// 状态可能是 running 或 fatal，取决于环境
	assert.Contains(t, output, "Agent 'test-agent':")
	if meta.State == "running" {
		assert.Contains(t, output, "PID:")
	}

	// 尝试停止（如果 agent 在 running 状态）
	meta, _ = store.GetAgent(context.Background(), "test-agent")
	if meta.State == "running" {
		r, w, _ = os.Pipe()
		os.Stdout = w

		err = stopAgent(cfg, []string{"test-agent"})

		w.Close()
		os.Stdout = oldStdout
		require.NoError(t, err)

		// 恢复
		r, w, _ = os.Pipe()
		os.Stdout = w

		err = recoverAgent(cfg, []string{"test-agent"})

		w.Close()
		os.Stdout = oldStdout
		require.NoError(t, err)
	}
}
