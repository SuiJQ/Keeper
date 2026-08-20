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

// NetworkStrategy 网络策略接口
type NetworkStrategy interface {
	// Name 返回策略名称
	Name() string
	
	// Configure 配置容器网络
	Configure(spec ContainerSpec) ([]string, error)
}

// DefaultNetworkStrategy 默认网络策略
type DefaultNetworkStrategy struct {
	logger log.Logger
}

// NewDefaultNetworkStrategy 创建默认网络策略
func NewDefaultNetworkStrategy(logger log.Logger) *DefaultNetworkStrategy {
	if logger == nil {
		logger = log.Global()
	}
	return &DefaultNetworkStrategy{logger: logger}
}

// Name 返回策略名称
func (s *DefaultNetworkStrategy) Name() string {
	return "default"
}

// Configure 配置容器网络
func (s *DefaultNetworkStrategy) Configure(spec ContainerSpec) ([]string, error) {
	args := []string{}
	
	// 配置 DNS
	if len(spec.Envvars) > 0 {
		// 设置 DNS 配置
		args = append(args, "--setenv=NAMESERVER=8.8.8.8")
		args = append(args, "--setenv=NAMESERVER=8.8.4.4")
	}
	
	return args, nil
}

// NewNetworkStrategy 创建网络策略
func NewNetworkStrategy(name string, logger log.Logger) (NetworkStrategy, error) {
	if logger == nil {
		logger = log.Global()
	}
	
	switch name {
	case "", "default":
		return NewDefaultNetworkStrategy(logger), nil
	default:
		return nil, fmt.Errorf("unknown network strategy: %s", name)
	}
}

// ResourceStrategy 资源限制策略接口
type ResourceStrategy interface {
	// Name 返回策略名称
	Name() string
	
	// Configure 配置容器资源限制
	Configure(spec ContainerSpec) ([]string, error)
}

// DefaultResourceStrategy 默认资源限制策略
type DefaultResourceStrategy struct {
	logger log.Logger
}

// NewDefaultResourceStrategy 创建默认资源限制策略
func NewDefaultResourceStrategy(logger log.Logger) *DefaultResourceStrategy {
	if logger == nil {
		logger = log.Global()
	}
	return &DefaultResourceStrategy{logger: logger}
}

// Name 返回策略名称
func (s *DefaultResourceStrategy) Name() string {
	return "default"
}

// Configure 配置容器资源限制
func (s *DefaultResourceStrategy) Configure(spec ContainerSpec) ([]string, error) {
	args := []string{}
	
	// 配置共享内存大小
	if spec.ShmSize > 0 {
		args = append(args, fmt.Sprintf("--shm-size=%dm", spec.ShmSize))
	}
	
	return args, nil
}

// NewResourceStrategy 创建资源限制策略
func NewResourceStrategy(name string, logger log.Logger) (ResourceStrategy, error) {
	if logger == nil {
		logger = log.Global()
	}
	
	switch name {
	case "", "default":
		return NewDefaultResourceStrategy(logger), nil
	default:
		return nil, fmt.Errorf("unknown resource strategy: %s", name)
	}
}

// LogStrategy 日志策略接口
type LogStrategy interface {
	// Name 返回策略名称
	Name() string
	
	// Configure 配置容器日志
	Configure(spec ContainerSpec) ([]string, error)
}

// DefaultLogStrategy 默认日志策略
type DefaultLogStrategy struct {
	logger log.Logger
}

// NewDefaultLogStrategy 创建默认日志策略
func NewDefaultLogStrategy(logger log.Logger) *DefaultLogStrategy {
	if logger == nil {
		logger = log.Global()
	}
	return &DefaultLogStrategy{logger: logger}
}

// Name 返回策略名称
func (s *DefaultLogStrategy) Name() string {
	return "default"
}

// Configure 配置容器日志
func (s *DefaultLogStrategy) Configure(spec ContainerSpec) ([]string, error) {
	args := []string{}
	
	// 配置日志输出
	args = append(args, "--log-level=info")
	args = append(args, "--log-file=/dev/null")
	
	return args, nil
}

// NewLogStrategy 创建日志策略
func NewLogStrategy(name string, logger log.Logger) (LogStrategy, error) {
	if logger == nil {
		logger = log.Global()
	}
	
	switch name {
	case "", "default":
		return NewDefaultLogStrategy(logger), nil
	default:
		return nil, fmt.Errorf("unknown log strategy: %s", name)
	}
}
