package container

import (
	"fmt"

	"keeper/internal/log"
)

// LogStrategy 日志策略接口
type LogStrategy interface {
	// Name 返回策略名称
	Name() string

	// Configure 配置容器日志
	Configure(spec ContainerSpec) ([]string, error)
}

const defaultStrategyName = "default"

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
	return defaultStrategyName
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
	switch name {
	case "", defaultStrategyName:
		return NewDefaultLogStrategy(logger), nil
	default:
		return nil, fmt.Errorf("unknown log strategy: %s", name)
	}
}
