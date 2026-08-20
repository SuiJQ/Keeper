package watchdog

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"keeper/internal/log"
	"keeper/internal/errors"
)

// Watchdog 看门狗监控器
type Watchdog struct {
	mu        sync.Mutex
	running   bool
	cancel    context.CancelFunc
	logger    log.Logger
	timeout   time.Duration
	checkInterval time.Duration
}

// WatchdogConfig 看门狗配置
type WatchdogConfig struct {
	Timeout       time.Duration
	CheckInterval time.Duration
}

// NewWatchdog 创建看门狗实例
func NewWatchdog(cfg WatchdogConfig, logger log.Logger) *Watchdog {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 5 * time.Second
	}
	if logger == nil {
		logger = log.Global()
	}

	return &Watchdog{
		logger:        logger,
		timeout:       cfg.Timeout,
		checkInterval: cfg.CheckInterval,
	}
}

// Start 启动看门狗
func (w *Watchdog) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return fmt.Errorf("watchdog already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.running = true

	w.logger.Info("watchdog started", 
		log.Field{Key: "timeout", Value: w.timeout},
		log.Field{Key: "check_interval", Value: w.checkInterval})

	go w.monitorLoop(ctx)

	return nil
}

// Stop 停止看门狗
func (w *Watchdog) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	if w.cancel != nil {
		w.cancel()
	}
	w.running = false
	w.logger.Info("watchdog stopped")
}

// monitorLoop 监控循环
func (w *Watchdog) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 检查 context 是否已取消
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.checkAgents()
			}
		}
	}
}

// checkAgents 检查所有 agent 状态
func (w *Watchdog) checkAgents() {
	// TODO: 从 storage 获取所有 agent 列表
	// 检查每个 agent 的启动时间和状态
	// 如果运行时间超过 timeout，触发终止逻辑
	w.logger.Debug("watchdog checking agents")
}

// TriggerStop 触发 agent 停止
func (w *Watchdog) TriggerStop(agentName string) error {
	w.logger.Info("watchdog triggering stop", log.Field{Key: "agent", Value: agentName})

	// 使用 systemd-run 或直接 kill 进程组
	// 这里简化实现：使用 pkill
	cmd := exec.Command("pkill", "-f", agentName)
	if output, err := cmd.CombinedOutput(); err != nil {
		w.logger.Error("failed to stop agent",
			log.Field{Key: "agent", Value: agentName},
			log.Field{Key: "error", Value: string(output)})
		return fmt.Errorf("stop agent %s: %w", agentName, err)
	}

	w.logger.Info("agent stopped by watchdog", log.Field{Key: "agent", Value: agentName})
	return nil
}

// ForceStop 强制停止 agent（处理 D 态进程）
func (w *Watchdog) ForceStop(agentName string) error {
	w.logger.Warn("watchdog force stopping agent", log.Field{Key: "agent", Value: agentName})

	// 强制终止：SIGKILL
	cmd := exec.Command("pkill", "-9", "-f", agentName)
	if output, err := cmd.CombinedOutput(); err != nil {
		w.logger.Error("failed to force stop agent",
			log.Field{Key: "agent", Value: agentName},
			log.Field{Key: "error", Value: string(output)})
		return fmt.Errorf("force stop agent %s: %w", agentName, err)
	}

	w.logger.Info("agent force stopped by watchdog", log.Field{Key: "agent", Value: agentName})
	return nil
}

// DetectDState 检测不可中断 I/O 阻塞
func (w *Watchdog) DetectDState(pid int) bool {
	// 读取 /proc/<pid>/status 中的 State 字段
	path := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "State:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1] == "D"
			}
		}
	}
	return false
}

// RecoverDState 恢复 D 态进程（需要重启宿主机）
func (w *Watchdog) RecoverDState(agentName string) error {
	w.logger.Error("detected D state, manual intervention required",
		log.Field{Key: "agent", Value: agentName})

	// 返回致命错误，触发看门狗报告
	return errors.NewKeeperError(
		errors.ErrCodeFatalDState,
		fmt.Sprintf("agent %s is in uninterruptible I/O state (D state), restart host required", agentName),
		nil,
	)
}

// 以下为辅助变量
var osReadFile = os.ReadFile
var bufioNewScanner = bufio.NewScanner
var stringsNewReader = strings.NewReader
var stringsFields = strings.Fields
