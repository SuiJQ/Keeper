package container

import (
	"keeper/internal/log"
	"keeper/internal/seccomp"
)

// SeccompStrategy Seccomp BPF 生成策略接口
type SeccompStrategy interface {
	// GenerateBPF 生成 Seccomp BPF 程序
	GenerateBPF() ([]byte, error)
	// Name 返回策略名称
	Name() string
}

// OverlayStrategy OverlayFS 挂载策略接口
type OverlayStrategy interface {
	// BuildArgs 构建 OverlayFS 挂载参数
	BuildArgs(lower, upper, work, mountPoint string) []string
	// Name 返回策略名称
	Name() string
}

// BPFGenerator 默认 BPF 生成器
type BPFGenerator struct {
	logger log.Logger
}

// NewBPFGenerator 创建默认 BPF 生成器
func NewBPFGenerator(logger log.Logger) *BPFGenerator {
	if logger == nil {
		logger = log.Global()
	}
	return &BPFGenerator{logger: logger}
}

// GenerateBPF 生成默认 Seccomp BPF 程序
func (g *BPFGenerator) GenerateBPF() ([]byte, error) {
	filter := seccomp.NewDefaultFilter()
	return seccomp.GenerateBPF(filter)
}

// Name 返回策略名称
func (g *BPFGenerator) Name() string {
	return "default"
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
