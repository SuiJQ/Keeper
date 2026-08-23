package container

import (
	"fmt"

	"keeper/internal/log"
)

// ResourceStrategy 资源限制策略接口
type ResourceStrategy interface {
	// Name 返回策略名称
	Name() string

	// Configure 配置容器资源限制
	Configure(spec Spec) ([]string, error)
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
	return defaultStrategyName
}

// Configure 配置容器资源限制
func (s *DefaultResourceStrategy) Configure(spec Spec) ([]string, error) {
	args := []string{}

	// 配置共享内存大小
	if spec.ShmSize > 0 {
		args = append(args, fmt.Sprintf("--shm-size=%dm", spec.ShmSize))
	}

	return args, nil
}

// NewResourceStrategy 创建资源限制策略
func NewResourceStrategy(name string, logger log.Logger) (ResourceStrategy, error) {
	switch name {
	case "", defaultStrategyName:
		return NewDefaultResourceStrategy(logger), nil
	default:
		return nil, fmt.Errorf("unknown resource strategy: %s", name)
	}
}
