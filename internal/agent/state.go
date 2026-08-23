// Package agent defines Agent state machine and core interfaces.
package agent

import (
	"fmt"
	"sync"
	"time"
)

// State is the Agent state enumeration.
type State string

const (
	// StateCreated is created, not started.
	StateCreated State = "created"
	// StateStopped is stopped.
	StateStopped State = "stopped"
	// StateRunning is running.
	StateRunning State = "running"
	// StateFatalKernel is kernel does not support OverlayFS+UserNS.
	StateFatalKernel State = "fatal_unsupported_kernel"
	// StateFatalDState is D state process detected.
	StateFatalDState State = "fatal_d_state"
	// StateFatalBwrap is cannot execute bwrap.
	StateFatalBwrap State = "fatal_bwrap_exec"
	// StateFatalNoSpace is storage space insufficient.
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

// allowedTransitions 允许的状态转换表（集中定义，避免重复）
var allowedTransitions = map[State][]State{
	StateCreated:      {StateRunning},
	StateStopped:      {StateRunning},
	StateRunning:      {StateStopped, StateFatalDState},
	StateFatalKernel:  {StateStopped},
	StateFatalDState:  {StateStopped},
	StateFatalBwrap:   {StateStopped},
	StateFatalNoSpace: {StateStopped},
}

// StateMachine 状态机
type StateMachine struct {
	current State
	mu      sync.Mutex
	// 可观测：记录转换次数
	transitions int64
}

// NewStateMachine 创建状态机
func NewStateMachine(initial State) *StateMachine {
	return &StateMachine{
		current: initial,
	}
}

// State 获取当前状态
func (sm *StateMachine) State() State {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.current
}

// Transitions 返回状态转换次数
func (sm *StateMachine) Transitions() int64 {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.transitions
}

// CanTransition 检查状态转换是否允许
func (sm *StateMachine) CanTransition(to State) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	from := sm.current
	permitted, ok := allowedTransitions[from]
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	from := sm.current
	permitted, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("invalid state: %s", from)
	}
	for _, p := range permitted {
		if p == to {
			sm.current = to
			sm.transitions++
			return nil
		}
	}
	return fmt.Errorf("state transition not allowed: %s -> %s", from, to)
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

// TransitionCount 返回状态机转换次数
func (a *Agent) TransitionCount() int64 {
	return a.state.Transitions()
}
