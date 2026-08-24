// Package container 提供容器运行时的工厂方法
package container

import (
	"fmt"

	"keeper/internal/log"
)

// NewFactory 根据运行时类型创建对应的工厂
func NewFactory(runtime string) (Factory, error) {
	switch runtime {
	case "docker", "":
		return NewDockerFactory(), nil
	case "bwrap":
		return NewBwrapFactory(), nil
	case "mock":
		return NewMockFactory(nil), nil
	default:
		return nil, fmt.Errorf("unsupported container runtime: %s", runtime)
	}
}

// NewFactoryWithLogger 根据运行时类型创建对应的工厂（带日志）
func NewFactoryWithLogger(runtime string, logger log.Logger) (Factory, error) {
	factory, err := NewFactory(runtime)
	if err != nil {
		return nil, err
	}
	return factory, nil
}
