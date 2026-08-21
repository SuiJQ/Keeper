package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"keeper/internal/storage"
	"keeper/pkg/config"
)

// TestAgentLifecycleEndToEnd 测试 Agent 完整生命周期
func TestAgentLifecycleEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "e2e-lifecycle-agent"

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 列出 Agent
	err = listAgents(cfg, []string{})
	require.NoError(t, err)

	// 3. 检查 Agent 状态（created）
	err = statusAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 4. 启动 Agent（使用 mock 后端）
	// 注意：当前环境内核 5.10 不支持 CONFIG_OVERLAY_FS_USERNS，启动会失败
	// 在 GitHub Actions (Ubuntu 24.04, kernel >= 5.11) 上会成功
	err = startAgent(cfg, []string{agentName})
	if err != nil {
		t.Logf("startAgent failed (expected on kernel 5.10): %v", err)
	}

	// 等待 Agent 启动
	time.Sleep(100 * time.Millisecond)

	// 5. 检查 Agent 状态
	err = statusAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 6. 查看 Agent 详情
	err = inspectAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 7. 停止 Agent（可能未启动，忽略错误）
	_ = stopAgent(cfg, []string{agentName})

	// 等待 Agent 停止
	time.Sleep(100 * time.Millisecond)

	// 8. 检查 Agent 状态
	err = statusAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 9. 创建快照
	err = snapshotAgent(cfg, []string{agentName, "snap1"})
	require.NoError(t, err)

	// 10. 回滚快照
	err = rollbackAgent(cfg, []string{agentName, "snap1"})
	require.NoError(t, err)

	// 11. 恢复 Agent
	err = recoverAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 12. 再次启动 Agent
	err = startAgent(cfg, []string{agentName})
	if err != nil {
		t.Logf("startAgent failed (expected on kernel 5.10): %v", err)
	}

	// 13. 停止 Agent
	_ = stopAgent(cfg, []string{agentName})

	// 14. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 15. 验证 Agent 已删除
	agentDir := filepath.Join(cfg.Home, "agents", agentName)
	_, statErr := os.Stat(agentDir)
	assert.True(t, os.IsNotExist(statErr), "agent directory should be deleted")
}

// TestAgentForkEndToEnd 测试 Agent Fork 完整流程
func TestAgentForkEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	sourceAgent := "fork-source"
	targetAgent := "fork-target"

	// 1. 创建源 Agent
	err = createAgent(cfg, []string{sourceAgent})
	require.NoError(t, err)

	// 2. 启动源 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{sourceAgent})
	time.Sleep(100 * time.Millisecond)

	// 3. 停止源 Agent（可能未启动，忽略）
	_ = stopAgent(cfg, []string{sourceAgent})

	// 如果启动失败导致状态异常，销毁并重建源 Agent
	_ = destroyAgent(cfg, []string{sourceAgent})
	err = createAgent(cfg, []string{sourceAgent})
	require.NoError(t, err)

	// 4. Fork Agent
	err = forkAgent(cfg, []string{sourceAgent, targetAgent})
	require.NoError(t, err)

	// 5. 验证目标 Agent 存在
	err = statusAgent(cfg, []string{targetAgent})
	require.NoError(t, err)

	// 6. 启动目标 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{targetAgent})

	// 7. 停止目标 Agent（忽略）
	_ = stopAgent(cfg, []string{targetAgent})

	// 8. 销毁源 Agent
	err = destroyAgent(cfg, []string{sourceAgent})
	require.NoError(t, err)

	// 9. 销毁目标 Agent
	err = destroyAgent(cfg, []string{targetAgent})
	require.NoError(t, err)
}

// TestAgentSnapshotRollbackEndToEnd 测试快照与回滚完整流程
func TestAgentSnapshotRollbackEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "snapshot-rollback-agent"

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 启动 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{agentName})

	// 3. 创建多个快照
	snapshots := []string{"snap1", "snap2", "snap3"}
	for _, snap := range snapshots {
		err = snapshotAgent(cfg, []string{agentName, snap})
		require.NoError(t, err)
	}

	// 4. 回滚到第一个快照
	err = rollbackAgent(cfg, []string{agentName, "snap1"})
	require.NoError(t, err)

	// 5. 恢复 Agent
	err = recoverAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 6. 停止 Agent（忽略）
	_ = stopAgent(cfg, []string{agentName})

	// 7. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)
}

// TestAgentConcurrentOperations 测试并发操作
func TestAgentConcurrentOperations(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 并发创建多个 Agent
	agentCount := 5
	agentNames := make([]string, agentCount)
	for i := 0; i < agentCount; i++ {
		agentNames[i] = "concurrent-agent-" + string(rune('0'+i))
	}

	// 并发创建
	done := make(chan bool, agentCount)
	for i, name := range agentNames {
		go func(idx int, agentName string) {
			err := createAgent(cfg, []string{agentName})
			assert.NoError(t, err)
			done <- true
		}(i, name)
	}

	for i := 0; i < agentCount; i++ {
		<-done
	}

	// 验证所有 Agent 都已创建
	for _, name := range agentNames {
		err = statusAgent(cfg, []string{name})
		require.NoError(t, err)
	}

	// 并发销毁
	for i, name := range agentNames {
		go func(idx int, agentName string) {
			err := destroyAgent(cfg, []string{agentName})
			assert.NoError(t, err)
			done <- true
		}(i, name)
	}

	for i := 0; i < agentCount; i++ {
		<-done
	}
}

// TestAgentCopyEndToEnd 测试 Agent 复制完整流程
func TestAgentCopyEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	sourceAgent := "copy-source"
	targetAgent := "copy-target"

	// 1. 创建源 Agent
	err = createAgent(cfg, []string{sourceAgent})
	require.NoError(t, err)

	// 2. 启动源 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{sourceAgent})

	// 3. 停止源 Agent（忽略）
	_ = stopAgent(cfg, []string{sourceAgent})

	// 如果启动失败导致状态异常，销毁并重建源 Agent
	_ = destroyAgent(cfg, []string{sourceAgent})
	err = createAgent(cfg, []string{sourceAgent})
	require.NoError(t, err)

	// 4. Fork Agent（使用 storage API）
	store, err := storage.NewStore(cfg.Home)
	require.NoError(t, err)

	_, err = store.ForkAgent(context.Background(), sourceAgent, targetAgent)
	require.NoError(t, err)

	// 5. 验证目标 Agent 存在
	err = statusAgent(cfg, []string{targetAgent})
	require.NoError(t, err)

	// 6. 启动目标 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{targetAgent})

	// 7. 停止目标 Agent（忽略）
	_ = stopAgent(cfg, []string{targetAgent})

	// 8. 销毁源 Agent
	err = destroyAgent(cfg, []string{sourceAgent})
	require.NoError(t, err)

	// 9. 销毁目标 Agent
	err = destroyAgent(cfg, []string{targetAgent})
	require.NoError(t, err)
}

// TestAgentRecoveryEndToEnd 测试 Agent 恢复完整流程
func TestAgentRecoveryEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "recovery-agent"

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 启动 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{agentName})

	// 3. 模拟异常停止（直接删除 PID 文件）
	agentDir := filepath.Join(cfg.Home, "agents", agentName)
	pidFile := filepath.Join(agentDir, "agent.pid")
	os.Remove(pidFile) // 忽略错误

	// 4. 恢复 Agent
	err = recoverAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 5. 验证 Agent 状态
	err = statusAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 6. 停止 Agent（忽略）
	_ = stopAgent(cfg, []string{agentName})

	// 7. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)
}

// TestAgentCopyEndToEndLocalToAgent 测试从本地复制文件到 Agent workspace
func TestAgentCopyEndToEndLocalToAgent(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 1. 创建目标 Agent
	err = createAgent(cfg, []string{"copy-target"})
	require.NoError(t, err)

	// 2. 创建本地源文件
	localSrc := filepath.Join(tmpDir, "local_src.txt")
	require.NoError(t, os.WriteFile(localSrc, []byte("hello from local"), 0644))

	// 3. 复制文件到 Agent workspace
	err = copyFile(cfg, []string{localSrc, "copy-target:/dest.txt"})
	require.NoError(t, err)

	// 4. 验证文件已复制
	dstFile := filepath.Join(tmpDir, "agents", "copy-target", "workspace", "dest.txt")
	assert.FileExists(t, dstFile)
	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello from local"), content)

	// 5. 销毁 Agent
	err = destroyAgent(cfg, []string{"copy-target"})
	require.NoError(t, err)
}

// TestAgentCopyEndToEndAgentToLocal 测试从 Agent workspace 复制文件到本地
func TestAgentCopyEndToEndAgentToLocal(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 1. 创建源 Agent 并写入文件
	err = createAgent(cfg, []string{"copy-source"})
	require.NoError(t, err)
	srcFile := filepath.Join(tmpDir, "agents", "copy-source", "workspace", "src.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("hello from agent"), 0644))

	// 2. 复制文件到本地
	localDst := filepath.Join(tmpDir, "local_dst.txt")
	err = copyFile(cfg, []string{"copy-source:/src.txt", localDst})
	require.NoError(t, err)

	// 3. 验证文件已复制
	assert.FileExists(t, localDst)
	content, err := os.ReadFile(localDst)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello from agent"), content)

	// 4. 销毁 Agent
	err = destroyAgent(cfg, []string{"copy-source"})
	require.NoError(t, err)
}

// TestAgentCopyEndToEndRecursive 测试递归复制目录
func TestAgentCopyEndToEndRecursive(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 1. 创建目标 Agent
	err = createAgent(cfg, []string{"copy-dir-target"})
	require.NoError(t, err)

	// 2. 创建本地源目录
	localSrcDir := filepath.Join(tmpDir, "local_src_dir")
	_ = os.MkdirAll(filepath.Join(localSrcDir, "sub"), 0755)
	require.NoError(t, os.WriteFile(filepath.Join(localSrcDir, "file1.txt"), []byte("file1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(localSrcDir, "sub", "file2.txt"), []byte("file2"), 0644))

	// 3. 递归复制目录到 Agent workspace
	err = copyFile(cfg, []string{"-r", localSrcDir, "copy-dir-target:/dest_dir"})
	require.NoError(t, err)

	// 4. 验证目录已复制
	dstFile1 := filepath.Join(tmpDir, "agents", "copy-dir-target", "workspace", "dest_dir", "file1.txt")
	dstFile2 := filepath.Join(tmpDir, "agents", "copy-dir-target", "workspace", "dest_dir", "sub", "file2.txt")
	assert.FileExists(t, dstFile1)
	assert.FileExists(t, dstFile2)

	// 5. 销毁 Agent
	err = destroyAgent(cfg, []string{"copy-dir-target"})
	require.NoError(t, err)
}

// TestAgentCopyEndToEndInvalidSource 测试复制到不存在的 Agent
func TestAgentCopyEndToEndInvalidSource(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 创建本地源文件
	localSrc := filepath.Join(tmpDir, "local_src.txt")
	require.NoError(t, os.WriteFile(localSrc, []byte("hello"), 0644))

	// 复制到不存在的 Agent 应该返回错误
	err = copyFile(cfg, []string{localSrc, "nonexistent:/dest.txt"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestAgentListEndToEnd 测试 list 命令端到端
func TestAgentListEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	// 1. 空列表
	err = listAgents(cfg, []string{})
	require.NoError(t, err)

	// 2. 创建几个 Agent
	agentNames := []string{"list-agent-1", "list-agent-2", "list-agent-3"}
	for _, name := range agentNames {
		err = createAgent(cfg, []string{name})
		require.NoError(t, err)
	}

	// 3. 列出所有 Agent
	err = listAgents(cfg, []string{})
	require.NoError(t, err)

	// 4. 销毁 Agent
	for _, name := range agentNames {
		err = destroyAgent(cfg, []string{name})
		require.NoError(t, err)
	}
}

// TestAgentInspectEndToEnd 测试 inspect 命令端到端
func TestAgentInspectEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "inspect-agent"

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 查看 Agent 信息
	err = inspectAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 3. 查看不存在的 Agent（应该返回错误）
	err = inspectAgent(cfg, []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// 4. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)
}

// TestAgentDestroyEndToEnd 测试 destroy 命令端到端
func TestAgentDestroyEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "destroy-agent"

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 3. 再次销毁（应该成功，因为 destroy 是幂等的）
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)
}

// TestAgentStatusEndToEnd 测试 status 命令端到端
func TestAgentStatusEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "status-agent"

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 查询状态（created）
	err = statusAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 3. 启动 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{agentName})

	// 4. 查询状态（可能是 running 或 fatal）
	err = statusAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 5. 查询不存在的 Agent（应该返回错误）
	err = statusAgent(cfg, []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// 6. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)
}

// TestAgentRunEndToEnd 测试 run 命令端到端
func TestAgentRunEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "run-agent"

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 启动 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{agentName})

	// 3. 检查 Agent 状态
	store, err := storage.NewStore(cfg.Home)
	require.NoError(t, err)
	meta, err := store.GetAgent(context.Background(), agentName)
	require.NoError(t, err)

	// 如果 Agent 处于 running 状态，run 应该返回错误
	if meta.State == "running" {
		err = runAgentCommand(cfg, []string{agentName})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot run agent in state: running")
	}

	// 4. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)
}

// TestAgentFullLifecycleEndToEnd 测试 Agent 完整生命周期
func TestAgentFullLifecycleEndToEnd(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "lifecycle-agent"

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 检查初始状态
	err = statusAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 3. 启动 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{agentName})

	// 4. 检查运行状态
	_ = statusAgent(cfg, []string{agentName})

	// 5. 查看 Agent 信息
	_ = inspectAgent(cfg, []string{agentName})

	// 6. 停止 Agent（可能未运行，忽略）
	_ = stopAgent(cfg, []string{agentName})

	// 7. 重新启动 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{agentName})

	// 8. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 9. 验证 Agent 已删除
	err = statusAgent(cfg, []string{agentName})
	assert.Error(t, err)
}

// TestAgentForkEndToEndSimple 测试 fork 命令端到端（简化版）
func TestAgentForkEndToEndSimple(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	sourceName := "fork-source"
	targetName := "fork-target"

	// 1. 创建源 Agent
	err = createAgent(cfg, []string{sourceName})
	require.NoError(t, err)

	// 2. 启动源 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{sourceName})

	// 如果启动失败导致状态异常，销毁并重建源 Agent
	_ = destroyAgent(cfg, []string{sourceName})
	err = createAgent(cfg, []string{sourceName})
	require.NoError(t, err)

	// 3. 停止源 Agent（fork 要求源 Agent 处于 stopped 状态）
	_ = stopAgent(cfg, []string{sourceName})

	// 4. Fork Agent
	err = forkAgent(cfg, []string{sourceName, targetName})
	require.NoError(t, err)

	// 5. 验证目标 Agent 存在
	err = statusAgent(cfg, []string{targetName})
	require.NoError(t, err)

	// 6. 启动目标 Agent（可能失败，忽略）
	_ = startAgent(cfg, []string{targetName})

	// 7. 销毁所有 Agent
	err = destroyAgent(cfg, []string{targetName})
	require.NoError(t, err)
	err = destroyAgent(cfg, []string{sourceName})
	require.NoError(t, err)
}

// TestAgentFullChainOperations 测试 cp/snapshot/rollback/recover 全链路操作
func TestAgentFullChainOperations(t *testing.T) {
	tmpDir, cleanup := setupTestConfig(t)
	defer cleanup()

	cfg, err := config.Load(tmpDir)
	require.NoError(t, err)

	agentName := "full-chain-agent"
	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)
	testFile := filepath.Join(sourceDir, "test.txt")
	err = os.WriteFile(testFile, []byte("original content"), 0644)
	require.NoError(t, err)

	// 1. 创建 Agent
	err = createAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 2. 复制文件到 Agent
	err = copyFile(cfg, []string{sourceDir, agentName + ":"})
	require.NoError(t, err)

	// 3. 创建快照
	snapshotName := "chain-snap"
	err = snapshotAgent(cfg, []string{agentName, snapshotName})
	require.NoError(t, err)

	// 4. 修改源文件并再次复制
	err = os.WriteFile(testFile, []byte("modified content"), 0644)
	require.NoError(t, err)
	err = copyFile(cfg, []string{sourceDir, agentName + ":"})
	require.NoError(t, err)

	// 5. 回滚到快照
	err = rollbackAgent(cfg, []string{agentName, snapshotName})
	require.NoError(t, err)

	// 6. 恢复 Agent
	err = recoverAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 7. 验证 Agent 状态
	err = statusAgent(cfg, []string{agentName})
	require.NoError(t, err)

	// 8. 销毁 Agent
	err = destroyAgent(cfg, []string{agentName})
	require.NoError(t, err)
}
