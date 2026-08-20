package watchdog

import (
	"context"
	"os"
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
