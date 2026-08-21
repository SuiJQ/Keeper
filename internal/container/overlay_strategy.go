package container

import (
	"fmt"

	"keeper/internal/log"
)

// OverlayStrategy OverlayFS 挂载策略接口
type OverlayStrategy interface {
	// BuildArgs 构建 OverlayFS 挂载参数
	BuildArgs(lower, upper, work, mountPoint string) []string
	// Name 返回策略名称
	Name() string
}

// OverlayBuilder 默认 Overlay 构建器
type OverlayBuilder struct {
	logger log.Logger
}

// NewOverlayBuilder 创建默认 Overlay 构建器
func NewOverlayBuilder(logger log.Logger) *OverlayBuilder {
	if logger == nil {
		logger = log.Global()
	}
	return &OverlayBuilder{logger: logger}
}

// BuildArgs 构建 OverlayFS 挂载参数
func (b *OverlayBuilder) BuildArgs(lower, upper, work, mountPoint string) []string {
	return []string{
		"--overlay", mountPoint,
		"--ro-bind", lower, mountPoint,
		"--bind", upper, mountPoint,
		"--bind", work, work,
	}
}

// Name 返回策略名称
func (b *OverlayBuilder) Name() string {
	return "default"
}

// OverlayFSStrategy OverlayFS 挂载策略
type OverlayFSStrategy struct {
	logger log.Logger
}

// BuildArgs 构建 OverlayFS 挂载参数
func (o *OverlayFSStrategy) BuildArgs(lower, upper, work, mountPoint string) []string {
	return []string{
		"--overlay", mountPoint,
		"--ro-bind", lower, mountPoint,
		"--bind", upper, mountPoint,
		"--bind", work, work,
		"--setenv", "XDG_RUNTIME_DIR", "/tmp",
	}
}

// Name 返回策略名称
func (o *OverlayFSStrategy) Name() string {
	return "overlayfs"
}

// NewOverlayStrategy 根据配置创建 Overlay 策略
func NewOverlayStrategy(name string, logger log.Logger) (OverlayStrategy, error) {
	switch name {
	case "default", "":
		return NewOverlayBuilder(logger), nil
	case "overlayfs":
		return &OverlayFSStrategy{logger: logger}, nil
	default:
		return nil, fmt.Errorf("unknown overlay strategy: %s", name)
	}
}
