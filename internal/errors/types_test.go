package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorCodeString(t *testing.T) {
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrCodeSystem, "SYSTEM"},
		{ErrCodeKernel, "KERNEL"},
		{ErrCodeIO, "IO"},
		{ErrCodeProcess, "PROCESS"},
		{ErrCodePermission, "PERMISSION"},
		{ErrCodeContainer, "CONTAINER"},
		{ErrCodeOverlay, "OVERLAY"},
		{ErrCodeSeccomp, "SECCOMP"},
		{ErrCodeNamespace, "NAMESPACE"},
		{ErrCodeMount, "MOUNT"},
		{ErrCodeAgent, "AGENT"},
		{ErrCodeNetwork, "NETWORK"},
		{ErrCodeStorage, "STORAGE"},
		{ErrCodeConfig, "CONFIG"},
		{ErrCodePort, "PORT"},
		{ErrCodeFatalKernel, "FATAL_KERNEL"},
		{ErrCodeFatalDState, "FATAL_D_STATE"},
		{ErrCodeFatalBwrap, "FATAL_BWRAP"},
		{ErrCodeFatalNoSpace, "FATAL_NO_SPACE"},
		{ErrorCode(9999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.code.String()
			if result != tt.expected {
				t.Errorf("ErrorCode.String() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestKeeperErrorError(t *testing.T) {
	tests := []struct {
		name     string
		err      *KeeperError
		expected string
	}{
		{
			name:     "without cause",
			err:      NewKeeperError(ErrCodeKernel, "kernel error", nil),
			expected: "[KERNEL] kernel error",
		},
		{
			name:     "with cause",
			err:      NewKeeperError(ErrCodeIO, "io error", errors.New("underlying error")),
			expected: "[IO] io error: underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("KeeperError.Error() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestKeeperErrorUnwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := NewKeeperError(ErrCodeIO, "io error", cause)

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("KeeperError.Unwrap() = %v, want %v", unwrapped, cause)
	}
}

func TestKeeperErrorBuilderMethods(t *testing.T) {
	err := NewKeeperError(ErrCodeAgent, "agent error", nil).
		WithState("running").
		WithRecoverable(true).
		WithMetadata("agent_name", "test-agent")

	if err.State != "running" {
		t.Errorf("KeeperError.State = %s, want running", err.State)
	}
	if !err.Recoverable {
		t.Errorf("KeeperError.Recoverable = false, want true")
	}
	if err.Metadata["agent_name"] != "test-agent" {
		t.Errorf("KeeperError.Metadata[agent_name] = %s, want test-agent", err.Metadata["agent_name"])
	}
}

func TestIsKeeperError(t *testing.T) {
	keeperErr := NewKeeperError(ErrCodeKernel, "kernel error", nil)
	regularErr := errors.New("regular error")

	// Test with KeeperError
	result, ok := IsKeeperError(keeperErr)
	if !ok {
		t.Errorf("IsKeeperError(keeperErr) returned false, want true")
	}
	if result != keeperErr {
		t.Errorf("IsKeeperError(keeperErr) returned different error")
	}

	// Test with wrapped KeeperError
	wrapped := fmt.Errorf("wrapped: %w", keeperErr)
	result, ok = IsKeeperError(wrapped)
	if !ok {
		t.Errorf("IsKeeperError(wrapped) returned false, want true")
	}
	if result != keeperErr {
		t.Errorf("IsKeeperError(wrapped) returned different error")
	}

	// Test with regular error
	_, ok = IsKeeperError(regularErr)
	if ok {
		t.Errorf("IsKeeperError(regularErr) returned true, want false")
	}
}

func TestIsRecoverable(t *testing.T) {
	recoverableErr := NewKeeperError(ErrCodeFatalKernel, "kernel error", nil).WithRecoverable(true)
	nonRecoverableErr := NewKeeperError(ErrCodeFatalKernel, "kernel error", nil).WithRecoverable(false)
	regularErr := errors.New("regular error")

	if !IsRecoverable(recoverableErr) {
		t.Errorf("IsRecoverable(recoverableErr) = false, want true")
	}
	if IsRecoverable(nonRecoverableErr) {
		t.Errorf("IsRecoverable(nonRecoverableErr) = true, want false")
	}
	if IsRecoverable(regularErr) {
		t.Errorf("IsRecoverable(regularErr) = true, want false")
	}
}

func TestPredefinedErrors(t *testing.T) {
	// Test that predefined errors are not nil
	if ErrOverlayFSNotSupported == nil {
		t.Error("ErrOverlayFSNotSupported is nil")
	}
	if ErrBwrapExecFailed == nil {
		t.Error("ErrBwrapExecFailed is nil")
	}
	if ErrNoSpace == nil {
		t.Error("ErrNoSpace is nil")
	}
	if ErrDStateDetected == nil {
		t.Error("ErrDStateDetected is nil")
	}

	// Test that they have correct codes
	if ErrOverlayFSNotSupported.Code != ErrCodeFatalKernel {
		t.Errorf("ErrOverlayFSNotSupported.Code = %d, want %d", ErrOverlayFSNotSupported.Code, ErrCodeFatalKernel)
	}
	if ErrBwrapExecFailed.Code != ErrCodeFatalBwrap {
		t.Errorf("ErrBwrapExecFailed.Code = %d, want %d", ErrBwrapExecFailed.Code, ErrCodeFatalBwrap)
	}
	if ErrNoSpace.Code != ErrCodeFatalNoSpace {
		t.Errorf("ErrNoSpace.Code = %d, want %d", ErrNoSpace.Code, ErrCodeFatalNoSpace)
	}
	if ErrDStateDetected.Code != ErrCodeFatalDState {
		t.Errorf("ErrDStateDetected.Code = %d, want %d", ErrDStateDetected.Code, ErrCodeFatalDState)
	}

	// Test that they are recoverable
	if !ErrOverlayFSNotSupported.Recoverable {
		t.Error("ErrOverlayFSNotSupported should be recoverable")
	}
	if !ErrBwrapExecFailed.Recoverable {
		t.Error("ErrBwrapExecFailed should be recoverable")
	}
	if !ErrNoSpace.Recoverable {
		t.Error("ErrNoSpace should be recoverable")
	}
	if !ErrDStateDetected.Recoverable {
		t.Error("ErrDStateDetected should be recoverable")
	}
}
