// Package container 定义容器运行时的抽象接口
package container

import (
	"context"
	"time"
)

// PortMapping 端口映射配置
type PortMapping struct {
	// Host 宿主机端口
	Host int
	// Container 容器端口
	Container int
}

// ContainerSpec 容器规格
type ContainerSpec struct {
	// Name 容器名称
	Name string
	// Rootfs 根文件系统路径
	Rootfs string
	// UpperDir overlayfs upper 目录
	UpperDir string
	// WorkDir 工作目录
	WorkDir string
	// Workspace 工作空间路径
	Workspace string
	// ShmSize 共享内存大小（MB）
	ShmSize int
	// SeccompBPF seccomp BPF 程序
	SeccompBPF []byte
	// Envvars 环境变量
	Envvars []string
	// Ports 端口映射列表
	Ports []PortMapping
}

// ContainerStatus 容器状态
type ContainerStatus struct {
	// State 容器状态：running/stopped/fatal_*
	State string
	// PID 主进程 PID
	PID int
	// PGID 进程组 ID
	PGID int
	// Error 错误信息
	Error string
	// Uptime 运行时间
	Uptime time.Duration
	// Ports 端口映射列表
	Ports []PortMapping
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
	// Command 要执行的命令
	Command string
	// Env 环境变量
	Env []string
	// Timeout 执行超时
	Timeout time.Duration
}

// ExecResponse 执行响应
type ExecResponse struct {
	// ExitCode 退出码
	ExitCode int
	// Stdout 标准输出
	Stdout []byte
	// Stderr 标准错误
	Stderr []byte
	// Error 错误信息
	Error string
}

// Factory 容器运行时工厂
type Factory interface {
	// Create 创建容器运行时实例
	Create(name string) (Container, error)

	// Type 返回运行时类型
	Type() string
}
