package agent

import (
	"strings"
	"testing"
)

func TestStateIsTerminal(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateCreated, false},
		{StateRunning, false},
		{StateStopped, true},
		{StateFatalKernel, true},
		{StateFatalDState, true},
		{StateFatalBwrap, true},
		{StateFatalNoSpace, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := tt.state.IsTerminal()
			if result != tt.expected {
				t.Errorf("State.IsTerminal() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestStateIsFatal(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateCreated, false},
		{StateRunning, false},
		{StateStopped, false},
		{StateFatalKernel, true},
		{StateFatalDState, true},
		{StateFatalBwrap, true},
		{StateFatalNoSpace, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			result := tt.state.IsFatal()
			if result != tt.expected {
				t.Errorf("State.IsFatal() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	state := StateRunning
	if state.String() != "running" {
		t.Errorf("State.String() = %s, want running", state.String())
	}
}

func TestStateMachineInitialState(t *testing.T) {
	sm := NewStateMachine(StateCreated)
	if sm.State() != StateCreated {
		t.Errorf("StateMachine.State() = %s, want created", sm.State())
	}
}

func TestStateMachineValidTransitions(t *testing.T) {
	tests := []struct {
		from     State
		to       State
		expected string // expected error substring, empty if no error
	}{
		{StateCreated, StateRunning, ""},
		{StateStopped, StateRunning, ""},
		{StateRunning, StateStopped, ""},
		{StateRunning, StateFatalDState, ""},
		{StateFatalKernel, StateStopped, ""},
		{StateFatalDState, StateStopped, ""},
		{StateFatalBwrap, StateStopped, ""},
		{StateFatalNoSpace, StateStopped, ""},
		{StateCreated, StateStopped, "invalid state transition"},
		{StateRunning, StateCreated, "invalid state transition"},
		{StateStopped, StateCreated, "invalid state transition"},
		{StateRunning, StateFatalKernel, "invalid state transition"},
		{StateFatalDState, StateRunning, "invalid state transition"},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"_to_"+string(tt.to), func(t *testing.T) {
			sm := NewStateMachine(tt.from)
			err := sm.CanTransition(tt.to)
			if tt.expected == "" {
				if err != nil {
					t.Errorf("CanTransition(%s -> %s) returned %v, want nil", tt.from, tt.to, err)
				}
			} else {
				if err == nil {
					t.Errorf("CanTransition(%s -> %s) returned nil, want error containing %q", tt.from, tt.to, tt.expected)
				} else if !strings.Contains(err.Error(), tt.expected) {
					t.Errorf("CanTransition(%s -> %s) returned %q, want error containing %q", tt.from, tt.to, err.Error(), tt.expected)
				}
			}
		})
	}
}

func TestStateMachineSetState(t *testing.T) {
	sm := NewStateMachine(StateCreated)

	// Valid transition
	err := sm.SetState(StateRunning)
	if err != nil {
		t.Errorf("SetState(created -> running) returned %v, want nil", err)
	}
	if sm.State() != StateRunning {
		t.Errorf("State after transition = %s, want running", sm.State())
	}

	// Invalid transition
	err = sm.SetState(StateCreated)
	if err == nil {
		t.Errorf("SetState(running -> created) returned nil, want error")
	}
}

func TestAgentNewAgent(t *testing.T) {
	agent := NewAgent("test-agent")
	if agent.Name != "test-agent" {
		t.Errorf("Agent.Name = %s, want test-agent", agent.Name)
	}
	if agent.State != StateCreated {
		t.Errorf("Agent.State = %s, want created", agent.State)
	}
	if agent.PID != 0 {
		t.Errorf("Agent.PID = %d, want 0", agent.PID)
	}
	if agent.PGID != 0 {
		t.Errorf("Agent.PGID = %d, want 0", agent.PGID)
	}
}

func TestAgentUpdateState(t *testing.T) {
	agent := NewAgent("test-agent")

	// Valid transition
	err := agent.UpdateState(StateRunning)
	if err != nil {
		t.Errorf("UpdateState(created -> running) returned %v, want nil", err)
	}
	if agent.State != StateRunning {
		t.Errorf("Agent.State = %s, want running", agent.State)
	}

	// Invalid transition
	err = agent.UpdateState(StateCreated)
	if err == nil {
		t.Errorf("UpdateState(running -> created) returned nil, want error")
	}
}

func TestAgentStateString(t *testing.T) {
	agent := NewAgent("test-agent")
	if agent.StateString() != "created" {
		t.Errorf("Agent.StateString() = %s, want created", agent.StateString())
	}
}

func TestAgentTransitionCount(t *testing.T) {
	agent := NewAgent("test-agent")
	if agent.TransitionCount() != 0 {
		t.Errorf("Agent.TransitionCount() = %d, want 0", agent.TransitionCount())
	}

	_ = agent.UpdateState(StateRunning)
	if agent.TransitionCount() != 1 {
		t.Errorf("Agent.TransitionCount() = %d, want 1", agent.TransitionCount())
	}

	_ = agent.UpdateState(StateStopped)
	if agent.TransitionCount() != 2 {
		t.Errorf("Agent.TransitionCount() = %d, want 2", agent.TransitionCount())
	}

	// invalid transition should not increase count
	err := agent.UpdateState(StateCreated)
	if err == nil {
		t.Fatalf("UpdateState(stopped -> created) returned nil, want error")
	}
	if agent.TransitionCount() != 2 {
		t.Errorf("Agent.TransitionCount() = %d, want 2 after failed transition", agent.TransitionCount())
	}
}
