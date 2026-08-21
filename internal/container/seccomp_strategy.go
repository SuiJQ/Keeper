package container

import (
	"fmt"

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

// WhitelistStrategy 白名单 Seccomp 策略
type WhitelistStrategy struct {
	logger log.Logger
}

// GenerateBPF 生成白名单 BPF
func (w *WhitelistStrategy) GenerateBPF() ([]byte, error) {
	filter := seccomp.NewWhitelistFilter()
	return seccomp.GenerateBPF(filter)
}

// Name 返回策略名称
func (w *WhitelistStrategy) Name() string {
	return "whitelist"
}

// BlacklistStrategy 黑名单 Seccomp 策略
type BlacklistStrategy struct {
	logger log.Logger
}

// GenerateBPF 生成黑名单 BPF
func (b *BlacklistStrategy) GenerateBPF() ([]byte, error) {
	filter := seccomp.NewBlacklistFilter()
	return seccomp.GenerateBPF(filter)
}

// Name 返回策略名称
func (b *BlacklistStrategy) Name() string {
	return "blacklist"
}

// AllowAllStrategy 允许所有 Seccomp 策略
type AllowAllStrategy struct {
	logger log.Logger
}

// GenerateBPF 生成允许所有 BPF
func (a *AllowAllStrategy) GenerateBPF() ([]byte, error) {
	// 返回空 BPF，表示允许所有系统调用
	return []byte{}, nil
}

// Name 返回策略名称
func (a *AllowAllStrategy) Name() string {
	return "allow_all"
}

// NewSeccompStrategy 根据配置创建 Seccomp 策略
func NewSeccompStrategy(name string, logger log.Logger) (SeccompStrategy, error) {
	switch name {
	case "default", "":
		return NewBPFGenerator(logger), nil
	case "whitelist":
		return &WhitelistStrategy{logger: logger}, nil
	case "blacklist":
		return &BlacklistStrategy{logger: logger}, nil
	case "allow_all":
		return &AllowAllStrategy{logger: logger}, nil
	default:
		return nil, fmt.Errorf("unknown seccomp strategy: %s", name)
	}
}
