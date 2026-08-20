// Package container 定义容器运行时的抽象接口
package container

import (
	"context"
	"time"
)

// PortMapping 端口映射
type PortMapping struct {
	Host      int
	Container int
}

// ContainerSpec 容器规格
type ContainerSpec struct {
	Name       string
	Rootfs     string
	UpperDir   string
	WorkDir    string
	Workspace  string
	ShmSize    int
	SeccompBPF []byte
	Envvars    []string
	Ports      []PortMapping
}

// ContainerStatus 容器状态
type ContainerStatus struct {
	State  string
	PID    int
	PGID   int
	Error  string
	Uptime time.Duration
	Ports  []PortMapping
}

// Container 容器运行时接口
type Container interface {
	// Start 启动容器，返回监控进程 PID
	Start(ctx context.Context, spec ContainerSpec) (int, error)

	// Stop 停止容器，grace 为优雅关闭超时
	Stop(ctx context.Context, grace time.Duration) error

	// Exec 在容器内执行命令
	Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error)

	// Status 查询容器状态
	Status(ctx context.Context) (*ContainerStatus, error)

	// Close 清理所有资源
	Close() error
}

// ExecRequest 执行请求
type ExecRequest struct {
	Command string
	Env     []string
	Timeout time.Duration
}

// ExecResponse 执行响应
type ExecResponse struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	Error    string
}

// Factory 容器运行时工厂
type Factory interface {
	// Create 创建容器运行时实例
	Create(name string) (Container, error)

	// Type 返回运行时类型
	Type() string
}
