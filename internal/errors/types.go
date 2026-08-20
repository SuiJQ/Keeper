// Package errors 定义 Keeper 的错误体系
package errors

import (
	"errors"
	"fmt"
)

// ErrorCode 错误码枚举
type ErrorCode int

const (
	// 系统级错误 (1000-1999)
	ErrCodeSystem ErrorCode = iota + 1000
	ErrCodeKernel
	ErrCodeIO
	ErrCodeProcess
	ErrCodePermission

	// 容器级错误 (2000-2999)
	ErrCodeContainer
	ErrCodeOverlay
	ErrCodeSeccomp
	ErrCodeNamespace
	ErrCodeMount

	// 业务级错误 (3000-3999)
	ErrCodeAgent
	ErrCodeNetwork
	ErrCodeStorage
	ErrCodeConfig
	ErrCodePort

	// 致命错误 (5000-5999)
	ErrCodeFatalKernel
	ErrCodeFatalDState
	ErrCodeFatalBwrap
	ErrCodeFatalNoSpace
)

// String 返回错误码的字符串表示
func (c ErrorCode) String() string {
	switch c {
	case ErrCodeSystem:
		return "SYSTEM"
	case ErrCodeKernel:
		return "KERNEL"
	case ErrCodeIO:
		return "IO"
	case ErrCodeProcess:
		return "PROCESS"
	case ErrCodePermission:
		return "PERMISSION"
	case ErrCodeContainer:
		return "CONTAINER"
	case ErrCodeOverlay:
		return "OVERLAY"
	case ErrCodeSeccomp:
		return "SECCOMP"
	case ErrCodeNamespace:
		return "NAMESPACE"
	case ErrCodeMount:
		return "MOUNT"
	case ErrCodeAgent:
		return "AGENT"
	case ErrCodeNetwork:
		return "NETWORK"
	case ErrCodeStorage:
		return "STORAGE"
	case ErrCodeConfig:
		return "CONFIG"
	case ErrCodePort:
		return "PORT"
	case ErrCodeFatalKernel:
		return "FATAL_KERNEL"
	case ErrCodeFatalDState:
		return "FATAL_D_STATE"
	case ErrCodeFatalBwrap:
		return "FATAL_BWRAP"
	case ErrCodeFatalNoSpace:
		return "FATAL_NO_SPACE"
	default:
		return "UNKNOWN"
	}
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

// NewKeeperError 创建新的 KeeperError
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

// IsKeeperError 检查是否为 KeeperError
func IsKeeperError(err error) (*KeeperError, bool) {
	var keeperErr *KeeperError
	if errors.As(err, &keeperErr) {
		return keeperErr, true
	}
	return nil, false
}

// IsRecoverable 检查错误是否可恢复
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
