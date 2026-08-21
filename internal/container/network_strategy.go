package container

import (
	"fmt"

	"keeper/internal/log"
)

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
	switch name {
	case "", "default":
		return NewDefaultNetworkStrategy(logger), nil
	default:
		return nil, fmt.Errorf("unknown network strategy: %s", name)
	}
}
