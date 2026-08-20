package watchdog

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"keeper/internal/errors"
	"keeper/internal/log"
)

// AgentInfo 被监控的 Agent 信息
type AgentInfo struct {
	Name      string
	PID       int
	StartedAt time.Time
}

// Watchdog 看门狗监控器
type Watchdog struct {
	mu            sync.Mutex
	running       bool
	cancel        context.CancelFunc
	logger        log.Logger
	timeout       time.Duration
	checkInterval time.Duration
	agents        map[string]*AgentInfo
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
		agents:        make(map[string]*AgentInfo),
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

// RegisterAgent 注册 agent 到看门狗
func (w *Watchdog) RegisterAgent(name string, pid int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.agents[name] = &AgentInfo{
		Name:      name,
		PID:       pid,
		StartedAt: time.Now(),
	}

	w.logger.Info("agent registered with watchdog",
		log.Field{Key: "agent", Value: name},
		log.Field{Key: "pid", Value: pid})
}

// UnregisterAgent 从看门狗注销 agent
func (w *Watchdog) UnregisterAgent(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.agents, name)
	w.logger.Info("agent unregistered from watchdog", log.Field{Key: "agent", Value: name})
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
	w.mu.Lock()
	agents := make([]*AgentInfo, 0, len(w.agents))
	for _, info := range w.agents {
		agents = append(agents, info)
	}
	w.mu.Unlock()

	for _, info := range agents {
		w.checkAgent(info)
	}
}

// checkAgent 检查单个 agent 状态
func (w *Watchdog) checkAgent(info *AgentInfo) {
	// 检查进程是否还存在
	if !isProcessAlive(info.PID) {
		w.logger.Warn("agent process not found",
			log.Field{Key: "agent", Value: info.Name},
			log.Field{Key: "pid", Value: info.PID})
		w.UnregisterAgent(info.Name)
		return
	}

	// 检查运行时间是否超时
	elapsed := time.Since(info.StartedAt)
	if elapsed > w.timeout {
		w.logger.Warn("agent timeout exceeded",
			log.Field{Key: "agent", Value: info.Name},
			log.Field{Key: "pid", Value: info.PID},
			log.Field{Key: "elapsed", Value: elapsed},
			log.Field{Key: "timeout", Value: w.timeout})

		// 触发停止
		if err := w.TriggerStop(info.Name); err != nil {
			w.logger.Error("failed to stop timed out agent",
				log.Field{Key: "agent", Value: info.Name},
				log.Field{Key: "error", Value: err})
		}

		w.UnregisterAgent(info.Name)
		return
	}

	// 检查 D 态
	if w.DetectDState(info.PID) {
		w.logger.Error("agent in D state",
			log.Field{Key: "agent", Value: info.Name},
			log.Field{Key: "pid", Value: info.PID})

		if err := w.RecoverDState(info.Name); err != nil {
			w.logger.Error("D state recovery failed",
				log.Field{Key: "agent", Value: info.Name},
				log.Field{Key: "error", Value: err})
		}

		w.UnregisterAgent(info.Name)
	}
}

// isProcessAlive 检查进程是否存活
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	// 读取 /proc/<pid>/stat 检查进程是否存在
	_, err := os.Stat(fmt.Sprintf("/proc/%d/stat", pid))
	return err == nil
}

// TriggerStop 触发 agent 停止
func (w *Watchdog) TriggerStop(agentName string) error {
	w.logger.Info("watchdog triggering stop", log.Field{Key: "agent", Value: agentName})

	// 查找 agent PID
	w.mu.Lock()
	info, exists := w.agents[agentName]
	w.mu.Unlock()

	if !exists {
		return fmt.Errorf("agent %s not registered", agentName)
	}

	// 优雅关闭：先尝试 SIGTERM
	if err := info.signalProcess(syscall.SIGTERM); err != nil {
		w.logger.Warn("failed to send SIGTERM", log.Field{Key: "error", Value: err.Error()})
	}

	// 等待优雅关闭或超时
	done := make(chan error, 1)
	go func() {
		_, err := syscall.Wait4(info.PID, nil, 0, nil)
		done <- err
	}()

	select {
	case <-time.After(5 * time.Second):
		// 超时，强制终止
		w.logger.Warn("grace period exceeded, force killing")
		if err := info.signalProcess(syscall.SIGKILL); err != nil {
			return fmt.Errorf("force kill: %w", err)
		}
		<-done
	case err := <-done:
		if err != nil {
			w.logger.Error("error waiting for process", log.Field{Key: "error", Value: err})
		}
	}

	w.logger.Info("agent stopped by watchdog", log.Field{Key: "agent", Value: agentName})
	return nil
}

// ForceStop 强制停止 agent（处理 D 态进程）
func (w *Watchdog) ForceStop(agentName string) error {
	w.logger.Warn("watchdog force stopping agent", log.Field{Key: "agent", Value: agentName})

	w.mu.Lock()
	info, exists := w.agents[agentName]
	w.mu.Unlock()

	if !exists {
		return fmt.Errorf("agent %s not registered", agentName)
	}

	// 强制终止：SIGKILL
	if err := info.signalProcess(syscall.SIGKILL); err != nil {
		return fmt.Errorf("force kill: %w", err)
	}

	w.logger.Info("agent force stopped by watchdog", log.Field{Key: "agent", Value: agentName})
	return nil
}

// DetectDState 检测不可中断 I/O 阻塞
func (w *Watchdog) DetectDState(pid int) bool {
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

	return errors.NewKeeperError(
		errors.ErrCodeFatalDState,
		fmt.Sprintf("agent %s is in uninterruptible I/O state (D state), restart host required", agentName),
		nil,
	)
}

// signalProcess 向进程发送信号
func (a *AgentInfo) signalProcess(sig os.Signal) error {
	if a.PID <= 0 {
		return fmt.Errorf("invalid pid: %d", a.PID)
	}

	proc, err := os.FindProcess(a.PID)
	if err != nil {
		return err
	}

	return proc.Signal(sig)
}

// 以下为辅助变量（便于测试）
var osReadFile = os.ReadFile
var bufioNewScanner = bufio.NewScanner
var stringsNewReader = strings.NewReader
var stringsFields = strings.Fields
