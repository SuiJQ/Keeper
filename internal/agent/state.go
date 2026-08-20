// Package agent 定义 Agent 状态机和核心接口
package agent

import (
	"fmt"
	"time"
)

// State Agent 状态枚举
type State string

const (
	// StateCreated 已创建，未启动
	StateCreated State = "created"
	// StateStopped 已停止
	StateStopped State = "stopped"
	// StateRunning 运行中
	StateRunning State = "running"
	// StateFatalKernel 内核不支持 OverlayFS+UserNS
	StateFatalKernel State = "fatal_unsupported_kernel"
	// StateFatalDState 检测到 D 状态进程
	StateFatalDState State = "fatal_d_state"
	// StateFatalBwrap 无法执行 bwrap
	StateFatalBwrap State = "fatal_bwrap_exec"
	// StateFatalNoSpace 存储空间不足
	StateFatalNoSpace State = "fatal_no_space"
)

// IsTerminal 检查是否为终态
func (s State) IsTerminal() bool {
	switch s {
	case StateStopped, StateFatalKernel, StateFatalDState, StateFatalBwrap, StateFatalNoSpace:
		return true
	}
	return false
}

// IsFatal 检查是否为 fatal 状态
func (s State) IsFatal() bool {
	switch s {
	case StateFatalKernel, StateFatalDState, StateFatalBwrap, StateFatalNoSpace:
		return true
	}
	return false
}

// String 返回状态的字符串表示
func (s State) String() string {
	return string(s)
}

// StateMachine 状态机
type StateMachine struct {
	current State
	mu      chan struct{} // 互斥锁（使用 channel 实现）
}

// NewStateMachine 创建状态机
func NewStateMachine(initial State) *StateMachine {
	return &StateMachine{
		current: initial,
		mu:      make(chan struct{}, 1),
	}
}

// State 获取当前状态
func (sm *StateMachine) State() State {
	sm.mu <- struct{}{}
	defer func() { <-sm.mu }()
	return sm.current
}

// CanTransition 检查状态转换是否允许
func (sm *StateMachine) CanTransition(to State) error {
	sm.mu <- struct{}{}
	defer func() { <-sm.mu }()

	from := sm.current

	// 允许的状态转换
	allowed := map[State][]State{
		StateCreated:      {StateRunning},
		StateStopped:      {StateRunning},
		StateRunning:      {StateStopped, StateFatalDState},
		StateFatalKernel:  {StateStopped},
		StateFatalDState:  {StateStopped},
		StateFatalBwrap:   {StateStopped},
		StateFatalNoSpace: {StateStopped},
	}

	permitted, ok := allowed[from]
	if !ok {
		return fmt.Errorf("invalid state: %s", from)
	}

	for _, p := range permitted {
		if p == to {
			return nil
		}
	}

	return fmt.Errorf("invalid state transition: %s -> %s", from, to)
}

// SetState 设置新状态（线程安全）
func (sm *StateMachine) SetState(to State) error {
	sm.mu <- struct{}{}
	defer func() { <-sm.mu }()

	if err := sm.CanTransition(to); err != nil {
		return fmt.Errorf("state transition not allowed: %w", err)
	}

	sm.current = to
	return nil
}

// Agent Agent 核心结构
type Agent struct {
	Name      string
	State     State
	CreatedAt time.Time
	StartedAt time.Time
	StoppedAt time.Time
	PID       int
	PGID      int
	Error     string
	state     *StateMachine
}

// NewAgent 创建新 Agent
func NewAgent(name string) *Agent {
	return &Agent{
		Name:      name,
		State:     StateCreated,
		CreatedAt: time.Now().UTC(),
		state:     NewStateMachine(StateCreated),
	}
}

// UpdateState 更新状态
func (a *Agent) UpdateState(to State) error {
	if err := a.state.SetState(to); err != nil {
		return err
	}
	a.State = to
	return nil
}

// StateString 返回当前状态字符串
func (a *Agent) StateString() string {
	return a.State.String()
}
