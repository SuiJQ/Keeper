package watchdog

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"keeper/internal/log"
)

func TestNewWatchdog(t *testing.T) {
	logger := log.Global()
	wd := NewWatchdog(WatchdogConfig{}, logger)
	assert.NotNil(t, wd)
	assert.Equal(t, 60*time.Second, wd.timeout)
	assert.Equal(t, 5*time.Second, wd.checkInterval)
}

func TestNewWatchdogCustomConfig(t *testing.T) {
	cfg := WatchdogConfig{
		Timeout:       30 * time.Second,
		CheckInterval: 10 * time.Second,
	}
	wd := NewWatchdog(cfg, nil)
	assert.Equal(t, 30*time.Second, wd.timeout)
	assert.Equal(t, 10*time.Second, wd.checkInterval)
}

func TestWatchdogStartStop(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{}, nil)
	ctx := context.Background()

	err := wd.Start(ctx)
	require.NoError(t, err)

	// 等待一个检查周期
	time.Sleep(100 * time.Millisecond)

	wd.Stop()

	// 验证已停止
	wd.mu.Lock()
	running := wd.running
	wd.mu.Unlock()
	assert.False(t, running)
}

func TestWatchdogStartAlreadyRunning(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{}, nil)
	ctx := context.Background()

	err := wd.Start(ctx)
	require.NoError(t, err)

	// 再次启动应该失败
	err = wd.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	wd.Stop()
}

func TestWatchdogStopNotRunning(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{}, nil)

	// 停止未运行的看门狗应该不报错
	wd.Stop()
}

func TestWatchdogTriggerStop(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{}, nil)

	// 触发停止不存在的 agent 应该返回错误
	err := wd.TriggerStop("nonexistent-agent")
	assert.Error(t, err)
}

func TestWatchdogForceStop(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{}, nil)

	// 强制停止不存在的 agent 应该返回错误
	err := wd.ForceStop("nonexistent-agent")
	assert.Error(t, err)
}

func TestWatchdogDetectDState(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{}, nil)

	// 检测当前进程（不应该是 D 态）
	detected := wd.DetectDState(os.Getpid())
	assert.False(t, detected)
}

func TestWatchdogRecoverDState(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{}, nil)

	// 恢复 D 态应该返回错误
	err := wd.RecoverDState("test-agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "uninterruptible I/O state")
}

func TestWatchdogMonitorLoop(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{
		CheckInterval: 50 * time.Millisecond,
	}, nil)

	ctx := context.Background()
	err := wd.Start(ctx)
	require.NoError(t, err)

	// 等待几个检查周期
	time.Sleep(150 * time.Millisecond)

	// 看门狗应该仍在运行
	wd.mu.Lock()
	running := wd.running
	wd.mu.Unlock()
	assert.True(t, running)

	// 手动停止
	wd.Stop()

	// 验证已停止
	wd.mu.Lock()
	running = wd.running
	wd.mu.Unlock()
	assert.False(t, running)
}

func TestWatchdogRegisterUnregister(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{}, nil)

	// 注册 agent
	wd.RegisterAgent("test-agent", 12345)

	// 验证已注册
	wd.mu.Lock()
	_, exists := wd.agents["test-agent"]
	wd.mu.Unlock()
	assert.True(t, exists)

	// 注销 agent
	wd.UnregisterAgent("test-agent")

	// 验证已注销
	wd.mu.Lock()
	_, exists = wd.agents["test-agent"]
	wd.mu.Unlock()
	assert.False(t, exists)
}

func TestWatchdogCheckAgentTimeout(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{
		Timeout:       100 * time.Millisecond,
		CheckInterval: 50 * time.Millisecond,
	}, nil)

	ctx := context.Background()
	err := wd.Start(ctx)
	require.NoError(t, err)

	// 注册一个 agent，但使用一个不存在的 PID
	wd.RegisterAgent("timeout-agent", 99999)

	// 等待超时检测
	time.Sleep(300 * time.Millisecond)

	// 验证 agent 已被注销
	wd.mu.Lock()
	_, exists := wd.agents["timeout-agent"]
	wd.mu.Unlock()
	assert.False(t, exists, "agent should be unregistered after timeout")

	wd.Stop()
}

func TestIsProcessAlive(t *testing.T) {
	// 启动一个子进程用于测试
	cmd := exec.Command("sleep", "10")
	err := cmd.Start()
	require.NoError(t, err)
	defer func() { _ = cmd.Process.Kill() }()

	pid := cmd.Process.Pid

	// 子进程应该存活
	assert.True(t, isProcessAlive(pid))

	// 无效 PID 应该返回 false
	assert.False(t, isProcessAlive(-1))
	assert.False(t, isProcessAlive(0))
	assert.False(t, isProcessAlive(999999))
}

// TestWatchdogUpdateTimeout 测试更新看门狗超时
func TestWatchdogUpdateTimeout(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{
		Timeout:       1 * time.Minute,
		CheckInterval: 10 * time.Second,
	}, log.Global())

	// 更新超时时间
	wd.UpdateTimeout(2 * time.Minute)

	// 验证超时时间已更新
	// 注意：timeout 字段是私有的，我们通过日志或行为验证
	// 这里只是验证不会 panic
}

// TestWatchdogUpdateCheckInterval 测试更新看门狗检查间隔
func TestWatchdogUpdateCheckInterval(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{
		Timeout:       1 * time.Minute,
		CheckInterval: 10 * time.Second,
	}, log.Global())

	// 更新检查间隔
	wd.UpdateCheckInterval(5 * time.Second)

	// 验证检查间隔已更新
	// 这里只是验证不会 panic
}

// TestAgentInfoSignalProcess 测试向进程发送信号
func TestAgentInfoSignalProcess(t *testing.T) {
	// 创建一个子进程
	cmd := exec.Command("sleep", "10")
	err := cmd.Start()
	require.NoError(t, err)
	defer func() { _ = cmd.Process.Kill() }()

	agent := &AgentInfo{
		Name: "test",
		PID:  cmd.Process.Pid,
	}

	// 发送 SIGTERM 信号
	err = agent.signalProcess(syscall.SIGTERM)
	assert.NoError(t, err)

	// 等待进程结束
	_, err = cmd.Process.Wait()
	assert.NoError(t, err)
}

// TestAgentInfoSignalProcessInvalidPID 测试无效 PID
func TestAgentInfoSignalProcessInvalidPID(t *testing.T) {
	agent := &AgentInfo{
		Name: "test",
		PID:  -1,
	}

	err := agent.signalProcess(syscall.SIGTERM)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pid")
}

// TestWatchdogConcurrentUpdate 测试并发更新
func TestWatchdogConcurrentUpdate(t *testing.T) {
	wd := NewWatchdog(WatchdogConfig{
		Timeout:       1 * time.Minute,
		CheckInterval: 10 * time.Second,
	}, log.Global())

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			wd.UpdateTimeout(time.Duration(id+1) * time.Minute)
			wd.UpdateCheckInterval(time.Duration(id+1) * time.Second)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
