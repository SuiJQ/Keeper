package container

import (
	"context"
	"fmt"
	"sync"
	"time"

	"keeper/internal/log"
)

// MockContainer 模拟容器运行时（用于测试）
type MockContainer struct {
	mu       sync.Mutex
	name     string
	status   ContainerStatus
	logger   log.Logger
	started  bool
	stopped  bool
	execs    []ExecRequest
}

// MockFactory 模拟容器运行时工厂
type MockFactory struct {
	mu      sync.Mutex
	created []string
	logger  log.Logger
}

// NewMockFactory 创建模拟工厂
func NewMockFactory(logger log.Logger) *MockFactory {
	if logger == nil {
		logger = log.Global()
	}
	return &MockFactory{logger: logger}
}

// Create 创建模拟容器
func (f *MockFactory) Create(name string) (Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, name)

	logger := f.logger.WithFields(log.Field{Key: "container", Value: name})
	logger.Info("creating mock container")

	return &MockContainer{
		name:   name,
		logger: logger,
		status: ContainerStatus{
			State: "created",
		},
	}, nil
}

// Type 返回运行时类型
func (f *MockFactory) Type() string {
	return "mock"
}

// CreatedNames 返回已创建的容器名称列表
func (f *MockFactory) CreatedNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]string, len(f.created))
	copy(result, f.created)
	return result
}

// Start 启动模拟容器
func (c *MockContainer) Start(ctx context.Context, spec ContainerSpec) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return 0, fmt.Errorf("container %s already started", c.name)
	}

	c.logger.Info("starting mock container")
	c.status = ContainerStatus{
		State:   "running",
		PID:     12345,
		PGID:    12345,
		Uptime:  0,
		Ports:   spec.Ports,
	}
	c.started = true

	return 12345, nil
}

// Stop 停止模拟容器
func (c *MockContainer) Stop(ctx context.Context, grace time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.stopped {
		return nil
	}

	c.logger.Info("stopping mock container")
	c.status.State = "stopped"
	c.status.PID = 0
	c.status.PGID = 0
	c.stopped = true

	return nil
}

// Exec 在模拟容器内执行命令
func (c *MockContainer) Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("executing command in mock container",
		log.Field{Key: "command", Value: req.Command})

	c.execs = append(c.execs, req)

	return &ExecResponse{
		ExitCode: 0,
		Stdout:   []byte("mock output\n"),
		Stderr:   []byte(""),
	}, nil
}

// Status 查询模拟容器状态
func (c *MockContainer) Status(ctx context.Context) (*ContainerStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return &ContainerStatus{
		State:   c.status.State,
		PID:     c.status.PID,
		PGID:    c.status.PGID,
		Uptime:  c.status.Uptime,
		Ports:   c.status.Ports,
	}, nil
}

// Close 清理模拟容器
func (c *MockContainer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status.State = "destroyed"
	return nil
}

// IsStarted 返回容器是否已启动
func (c *MockContainer) IsStarted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.started
}

// IsStopped 返回容器是否已停止
func (c *MockContainer) IsStopped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
}

// ExecCount 返回执行的命令数量
func (c *MockContainer) ExecCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.execs)
}

// LastExec 返回最后一次执行的命令
func (c *MockContainer) LastExec() (ExecRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.execs) == 0 {
		return ExecRequest{}, false
	}
	return c.execs[len(c.execs)-1], true
}
