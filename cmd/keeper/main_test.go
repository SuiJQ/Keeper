package main

import (
	"bytes"
	"context"
	"os"
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

