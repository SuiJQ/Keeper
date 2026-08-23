// Package errors defines Keeper's error system.
package errors

import (
	"errors"
	"fmt"
)

// ErrorCode is the enumeration of Keeper error codes.
type ErrorCode int

const (
	// ErrCodeSystem system-level errors (1000-1999)
	ErrCodeSystem ErrorCode = iota + 1000
	// ErrCodeKernel kernel-level errors
	ErrCodeKernel
	// ErrCodeIO IO errors
	ErrCodeIO
	// ErrCodeProcess process errors
	ErrCodeProcess
	// ErrCodePermission permission errors
	ErrCodePermission

	// ErrCodeContainer container-level errors (2000-2999)
	ErrCodeContainer
	// ErrCodeOverlay overlay filesystem errors
	ErrCodeOverlay
	// ErrCodeSeccomp seccomp errors
	ErrCodeSeccomp
	// ErrCodeNamespace namespace errors
	ErrCodeNamespace
	// ErrCodeMount mount errors
	ErrCodeMount

	// ErrCodeAgent agent-level errors (3000-3999)
	ErrCodeAgent
	// ErrCodeNetwork network errors
	ErrCodeNetwork
	// ErrCodeStorage storage errors
	ErrCodeStorage
	// ErrCodeConfig configuration errors
	ErrCodeConfig
	// ErrCodePort port errors
	ErrCodePort

	// ErrCodeFatalKernel fatal kernel errors (5000-5999)
	ErrCodeFatalKernel
	// ErrCodeFatalDState fatal D-state errors
	ErrCodeFatalDState
	// ErrCodeFatalBwrap fatal bwrap errors
	ErrCodeFatalBwrap
	// ErrCodeFatalNoSpace fatal no space errors
	ErrCodeFatalNoSpace
)

// errorCodeNames 错误码到名称的映射
var errorCodeNames = map[ErrorCode]string{
	ErrCodeSystem:       "SYSTEM",
	ErrCodeKernel:       "KERNEL",
	ErrCodeIO:           "IO",
	ErrCodeProcess:      "PROCESS",
	ErrCodePermission:   "PERMISSION",
	ErrCodeContainer:    "CONTAINER",
	ErrCodeOverlay:      "OVERLAY",
	ErrCodeSeccomp:      "SECCOMP",
	ErrCodeNamespace:    "NAMESPACE",
	ErrCodeMount:        "MOUNT",
	ErrCodeAgent:        "AGENT",
	ErrCodeNetwork:      "NETWORK",
	ErrCodeStorage:      "STORAGE",
	ErrCodeConfig:       "CONFIG",
	ErrCodePort:         "PORT",
	ErrCodeFatalKernel:  "FATAL_KERNEL",
	ErrCodeFatalDState:  "FATAL_D_STATE",
	ErrCodeFatalBwrap:   "FATAL_BWRAP",
	ErrCodeFatalNoSpace: "FATAL_NO_SPACE",
}

// String 返回错误码的字符串表示
func (c ErrorCode) String() string {
	if name, ok := errorCodeNames[c]; ok {
		return name
	}
	return "UNKNOWN"
}

// KeeperError 结构化错误
type KeeperError struct {
	Code        ErrorCode
	Message     string
	Cause       error
	State       string
	Recoverable bool
	Metadata    map[string]string
}

// Error 实现 error 接口
func (e *KeeperError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 支持 errors.Unwrap
func (e *KeeperError) Unwrap() error {
	return e.Cause
}

// NewKeeperError creates a new KeeperError with the given code, message, and cause.
func NewKeeperError(code ErrorCode, message string, cause error) *KeeperError {
	return &KeeperError{
		Code:        code,
		Message:     message,
		Cause:       cause,
		Recoverable: false,
		Metadata:    make(map[string]string),
	}
}

// WithState 设置状态
func (e *KeeperError) WithState(state string) *KeeperError {
	e.State = state
	return e
}

// WithRecoverable 设置是否可恢复
func (e *KeeperError) WithRecoverable(recoverable bool) *KeeperError {
	e.Recoverable = recoverable
	return e
}

// WithMetadata 添加元数据
func (e *KeeperError) WithMetadata(key, value string) *KeeperError {
	e.Metadata[key] = value
	return e
}

// IsKeeperError checks whether the error is a KeeperError.
func IsKeeperError(err error) (*KeeperError, bool) {
	var keeperErr *KeeperError
	if errors.As(err, &keeperErr) {
		return keeperErr, true
	}
	return nil, false
}

// IsRecoverable checks whether the error is recoverable.
func IsRecoverable(err error) bool {
	if keeperErr, ok := IsKeeperError(err); ok {
		return keeperErr.Recoverable
	}
	return false
}

// 预定义错误
var (
	ErrOverlayFSNotSupported = NewKeeperError(
		ErrCodeFatalKernel,
		"检测到 OverlayFS+UserNS 不可用，请确保宿主机内核 ≥5.11 且开启 CONFIG_OVERLAY_FS_USERNS",
		nil,
	).WithRecoverable(true)

	ErrBwrapExecFailed = NewKeeperError(
		ErrCodeFatalBwrap,
		"无法执行内嵌 bwrap，请检查 MIRAGE_HOME 挂载属性或手动赋予执行权限",
		nil,
	).WithRecoverable(true)

	ErrNoSpace = NewKeeperError(
		ErrCodeFatalNoSpace,
		"存储设备空间不足，请扩容后执行 mirage recover --force 重置",
		nil,
	).WithRecoverable(true)

	ErrDStateDetected = NewKeeperError(
		ErrCodeFatalDState,
		"检测到不可中断 I/O 阻塞，请重启宿主机释放 D 态进程后，执行 mirage recover --force",
		nil,
	).WithRecoverable(true)
)
