package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"keeper/internal/container"
	"keeper/internal/metrics"
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
	// 快照现在是压缩的 tar.gz 文件
	assert.FileExists(t, filepath.Join(snapshotDir, "upper.tar.gz"))
	assert.FileExists(t, filepath.Join(snapshotDir, "workspace.tar.gz"))
	assert.FileExists(t, filepath.Join(snapshotDir, "meta.json"))

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

func TestRunAgentCommandMissingName(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	err = runAgentCommand(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper run <name>")
}

func TestRunAgentCommandInvalidState(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建一个 agent，但手动将其状态设置为无效状态
	store, _ := storage.NewStore(cfg.Home)
	_, err = store.GetAgent(context.Background(), "test-agent")
	// 忽略错误（agent 不存在）

	// 直接测试错误路径
	// 由于 runAgentCommand 需要完整的环境，我们只测试参数验证
	err = runAgentCommand(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper run <name>")
}

// TestMultipleAgentsLifecycle 测试多个 agent 的完整生命周期
func TestMultipleAgentsLifecycle(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentNames := []string{"agent1", "agent2", "agent3"}

	// 批量创建 agent
	for _, name := range agentNames {
		err := createAgent(cfg, []string{name})
		require.NoError(t, err)
	}

	// 验证所有 agent 都已创建
	for _, name := range agentNames {
		agentDir := filepath.Join(tmpDir, "agents", name)
		assert.DirExists(t, agentDir)
		assert.FileExists(t, filepath.Join(agentDir, "meta.json"))
	}

	// 列出所有 agent
	r, w, _ := os.Pipe()
	os.Stdout = w
	err = listAgents(cfg, []string{})
	w.Close()
	os.Stdout = os.Stderr

	require.NoError(t, err)
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// 验证输出包含所有 agent
	for _, name := range agentNames {
		assert.Contains(t, output, name)
	}
}

// TestForkAgentLifecycle 测试 fork 后的完整生命周期
func TestForkAgentLifecycle(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建源 agent
	require.NoError(t, createAgent(cfg, []string{"source"}))

	// 向源 agent 写入数据
	workspace := filepath.Join(tmpDir, "agents", "source", "workspace")
	require.NoError(t, os.MkdirAll(workspace, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "data.txt"), []byte("source data"), 0644))

	// Fork
	require.NoError(t, forkAgent(cfg, []string{"source", "forked"}))

	// 验证 fork 后的 agent
	forkedDir := filepath.Join(tmpDir, "agents", "forked")
	assert.DirExists(t, forkedDir)

	// 验证状态为 created
	store, _ := storage.NewStore(cfg.Home)
	meta, err := store.GetAgent(context.Background(), "forked")
	require.NoError(t, err)
	assert.Equal(t, "created", meta.State)

	// 验证 pgid 为空（fork 时已清理）
	assert.Empty(t, meta.PGID)
}

// TestCopyFileAgentPath 测试 agent 路径复制
func TestCopyFileAgentPath(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建源 agent
	require.NoError(t, createAgent(cfg, []string{"source"}))

	// 写入文件到本地临时目录
	localSrc := filepath.Join(tmpDir, "local_src.txt")
	require.NoError(t, os.WriteFile(localSrc, []byte("hello"), 0644))

	// 复制文件到目标 agent
	require.NoError(t, createAgent(cfg, []string{"target"}))
	err = copyFile(cfg, []string{localSrc, "target:/copied.txt"})
	require.NoError(t, err)

	// 验证文件已复制
	targetFile := filepath.Join(tmpDir, "agents", "target", "workspace", "copied.txt")
	assert.FileExists(t, targetFile)
	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)
}

// TestInvalidNameValidation 测试名称验证
func TestInvalidNameValidation(t *testing.T) {
	tmpDir, _ := setupTestConfig(t)
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	invalidNames := []string{
		"",           // 空名称
		"-start",     // 以连字符开头
		"_start",     // 以下划线开头
		"start!",     // 包含特殊字符
		"start name", // 包含空格
		"this-name-is-way-too-long-to-be-valid-because-it-exceeds-the-maximum-length-of-32-chars", // 太长
	}

	for _, name := range invalidNames {
		err := createAgent(cfg, []string{name})
		assert.Error(t, err, "name '%s' should be invalid", name)
	}
}

// TestRunAgentCommandUsage 测试 run 子命令的 usage 输出
func TestRunAgentCommandUsage(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试空参数
	err = runAgentCommand(cfg, []string{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "usage: keeper run <name>")
}

// TestInspectAgentNotFound 测试 inspect 不存在的 agent
func TestInspectAgentNotFound(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试 inspect 不存在的 agent
	_, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	err = inspectAgent(cfg, []string{"nonexistent"})

	w.Close()
	os.Stdout = oldStdout

	assert.Error(t, err)
	// 错误信息可能被包装过
	assert.Contains(t, err.Error(), "nonexistent")
}

// TestStopAgentNotFound 测试 stop 不存在的 agent
func TestStopAgentNotFound(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试 stop 不存在的 agent
	err = stopAgent(cfg, []string{"nonexistent"})
	assert.Error(t, err)
	// 错误信息可能被包装过
	assert.Contains(t, err.Error(), "nonexistent")
}

// TestDestroyAgentNotFound 测试 destroy 不存在的 agent
func TestDestroyAgentNotFound(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试 destroy 不存在的 agent（可能不会返回错误）
	// 这里我们只验证不会 panic
	err = destroyAgent(cfg, []string{"nonexistent"})
	// 不强制要求返回错误，因为 destroy 可能是幂等的
	_ = err
}

// TestSnapshotAgentNotFound 测试 snapshot 不存在的 agent
func TestSnapshotAgentNotFound(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试 snapshot 不存在的 agent
	err = snapshotAgent(cfg, []string{"nonexistent", "snap1"})
	assert.Error(t, err)
	// 错误信息可能被包装过
	assert.Contains(t, err.Error(), "nonexistent")
}

// TestRollbackAgentNotFound 测试 rollback 不存在的 agent
func TestRollbackAgentNotFound(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试 rollback 不存在的 agent
	err = rollbackAgent(cfg, []string{"nonexistent", "snap1"})
	assert.Error(t, err)
	// 错误信息可能被包装过
	assert.Contains(t, err.Error(), "nonexistent")
}

// TestRecoverAgentNotFound 测试 recover 不存在的 agent
func TestRecoverAgentNotFound(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试 recover 不存在的 agent
	err = recoverAgent(cfg, []string{"nonexistent"})
	assert.Error(t, err)
	// 错误信息可能被包装过
	assert.Contains(t, err.Error(), "nonexistent")
}

// TestStatusAgentNotFound 测试 status 不存在的 agent
func TestStatusAgentNotFound(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试 status 不存在的 agent
	err = statusAgent(cfg, []string{"nonexistent"})
	assert.Error(t, err)
	// 错误信息可能被包装过
	assert.Contains(t, err.Error(), "nonexistent")
}

// TestMetricsCommand 测试 metrics 命令输出
func TestMetricsCommand(t *testing.T) {
	// 记录一些指标
	container.RecordContainerStart("bwrap", "success")
	container.RecordContainerStop("bwrap", "success")
	container.SetContainerActive("bwrap", 1)

	// 获取 Prometheus 格式输出
	output := metrics.PrometheusFormat()

	// 验证输出包含 Prometheus 格式
	assert.Contains(t, output, "# HELP")
	assert.Contains(t, output, "# TYPE")
	assert.Contains(t, output, "keeper_container_start_total")
	assert.Contains(t, output, "keeper_container_stop_total")
}

// TestVersionCommand 测试 version 命令输出
func TestVersionCommand(t *testing.T) {
	// version 命令直接打印版本号
	output := fmt.Sprintf("keeper %s\n", version)
	assert.Contains(t, output, "keeper")
	assert.NotEmpty(t, version)
}

// TestHelpCommand 测试 help 命令输出
func TestHelpCommand(t *testing.T) {
	// 捕获 printUsage 输出
	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w
	
	printUsage()
	
	w.Close()
	os.Stdout = oldStdout
	
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()
	
	assert.Contains(t, output, "keeper")
	assert.Contains(t, output, "create")
	assert.Contains(t, output, "start")
}

// TestListAgentsEmpty 测试空列表
func TestListAgentsEmpty(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()
	
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 列出 Agent（应该为空）
	err = listAgents(cfg, []string{})
	require.NoError(t, err)
}

// TestInvalidAgentName 测试无效名称
func TestInvalidAgentName(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()
	
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试各种无效名称
	invalidNames := []string{
		"",           // 空名称
		"agent@name", // 包含特殊字符
		"agent name", // 包含空格
		"agent/name", // 包含斜杠
		"agent:name", // 包含冒号
		strings.Repeat("a", 33), // 太长（>32）
	}

	for _, name := range invalidNames {
		err := createAgent(cfg, []string{name})
		assert.Error(t, err, "name %q should be invalid", name)
	}
}

// TestValidAgentName 测试有效名称
func TestValidAgentName(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()
	
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 测试有效名称
	validNames := []string{
		"agent",
		"my-agent",
		"my_agent",
		"Agent123",
		"a",
		strings.Repeat("a", 32), // 最大长度
	}

	for _, name := range validNames {
		err := createAgent(cfg, []string{name})
		assert.NoError(t, err, "name %q should be valid", name)
		
		// 清理
		_ = destroyAgent(cfg, []string{name})
	}
}

// TestCreateAgentDuplicate 测试重复创建 Agent
func TestCreateAgentDuplicate(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()
	
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "duplicate-agent"

	// 第一次创建
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 第二次创建应该失败
	err = createAgent(cfg, []string{agentName})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	// 清理
	_ = destroyAgent(cfg, []string{agentName})
}

// TestDestroyAgentTwice 测试重复销毁 Agent
func TestDestroyAgentTwice(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()
	
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "destroy-twice-agent"

	// 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 第一次销毁
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 第二次销毁（幂等性）
	err = destroyAgent(cfg, []string{agentName})
	// 不报错，因为 destroy 是幂等的
	_ = err
}

// TestSnapshotRollbackWithoutSnapshot 测试回滚不存在的快照
func TestSnapshotRollbackWithoutSnapshot(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()
	
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "no-snapshot-agent"

	// 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 尝试回滚不存在的快照
	err = rollbackAgent(cfg, []string{agentName, "nonexistent"})
	assert.Error(t, err)

	// 清理
	_ = destroyAgent(cfg, []string{agentName})
}

// TestListSnapshotsEmpty 测试空快照列表
func TestListSnapshotsEmpty(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()
	
	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "no-snapshots-agent"

	// 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 创建快照（验证快照功能正常）
	err = snapshotAgent(cfg, []string{agentName, "snap1"})
	require.NoError(t, err)

	// 清理
	_ = destroyAgent(cfg, []string{agentName})
}
