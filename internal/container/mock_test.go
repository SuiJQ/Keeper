package container

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockFactory(t *testing.T) {
	factory := NewMockFactory(nil)
	assert.NotNil(t, factory)
	assert.Equal(t, "mock", factory.Type())
	assert.Empty(t, factory.CreatedNames())
}

func TestMockFactoryCreate(t *testing.T) {
	factory := NewMockFactory(nil)

	container, err := factory.Create("test-container")
	require.NoError(t, err)
	assert.NotNil(t, container)
	status, err := container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "created", status.State)

	// 验证创建记录
	assert.Equal(t, []string{"test-container"}, factory.CreatedNames())
}

func TestMockContainerStartStop(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	// 启动
	pid, err := container.Start(context.Background(), Spec{})
	require.NoError(t, err)
	assert.Equal(t, 12345, pid)
	assert.True(t, func() bool { c, _ := container.(*MockContainer); return c.IsStarted() }())

	// 停止
	err = container.Stop(context.Background(), 5*time.Second)
	require.NoError(t, err)
	assert.True(t, func() bool { c, _ := container.(*MockContainer); return c.IsStopped() }())
}

func TestMockContainerStartTwice(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	_, err = container.Start(context.Background(), Spec{})
	require.NoError(t, err)

	// 再次启动应该失败
	_, err = container.Start(context.Background(), Spec{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestMockContainerExec(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	_, err = container.Start(context.Background(), Spec{})
	require.NoError(t, err)

	// 执行命令
	req := ExecRequest{Command: "echo hello"}
	resp, err := container.Exec(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.ExitCode)
	assert.Equal(t, []byte("mock output\n"), resp.Stdout)

	// 验证执行记录
	mock, _ := container.(*MockContainer)
	assert.Equal(t, 1, mock.ExecCount())
	lastExec, _ := mock.LastExec()
	assert.Equal(t, "echo hello", lastExec.Command)
}

func TestMockContainerClose(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	err = container.Close()
	require.NoError(t, err)
	status, err := container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "destroyed", status.State)
}

func TestMockContainerExecNotStarted(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	// 在未启动的容器上执行命令应该成功（mock 不限制）
	_, err = container.Exec(context.Background(), ExecRequest{Command: "echo hello"})
	assert.NoError(t, err)
}

// TestMockContainerConcurrentOperations 测试并发操作
func TestMockContainerConcurrentOperations(t *testing.T) {
	factory := NewMockFactory(nil)

	// 并发创建多个容器
	containers := make([]*MockContainer, 10)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			c, err := factory.Create(fmt.Sprintf("agent-%d", idx))
			if err != nil {
				errors <- err
				return
			}
			containers[idx] = c.(*MockContainer)
			errors <- nil
		}(i)
	}

	for i := 0; i < 10; i++ {
		err := <-errors
		require.NoError(t, err)
	}

	// 并发启动所有容器
	spec := Spec{Name: "test"}
	for i := 0; i < 10; i++ {
		go func(idx int) {
			_, err := containers[idx].Start(context.Background(), spec)
			errors <- err
		}(i)
	}

	for i := 0; i < 10; i++ {
		err := <-errors
		require.NoError(t, err)
	}

	// 验证所有容器都在运行
	for i := 0; i < 10; i++ {
		assert.True(t, containers[i].IsStarted())
		assert.False(t, containers[i].IsStopped())
	}

	// 并发停止所有容器
	for i := 0; i < 10; i++ {
		go func(idx int) {
			err := containers[idx].Stop(context.Background(), 0)
			errors <- err
		}(i)
	}

	for i := 0; i < 10; i++ {
		err := <-errors
		require.NoError(t, err)
	}

	// 验证所有容器都已停止（注意：mock容器停止后IsStarted可能仍返回true）
	for i := 0; i < 10; i++ {
		assert.True(t, containers[i].IsStopped())
	}
}

// TestMockStatusTransitions 测试状态转换
func TestMockStatusTransitions(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)
	mockContainer := container.(*MockContainer)

	// 初始状态：已创建
	assert.False(t, mockContainer.IsStarted())
	assert.False(t, mockContainer.IsStopped())
	status, err := container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "created", status.State)

	// 启动
	spec := Spec{Name: "test"}
	_, err = container.Start(context.Background(), spec)
	require.NoError(t, err)
	assert.True(t, mockContainer.IsStarted())
	assert.False(t, mockContainer.IsStopped())
	status, err = container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "running", status.State)

	// 停止
	err = container.Stop(context.Background(), 0)
	require.NoError(t, err)
	// 注意：mock容器停止后IsStarted可能仍返回true（模拟行为）
	assert.True(t, mockContainer.IsStopped())
	status, err = container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "stopped", status.State)
}

// TestMockContainerMultipleExecs 测试多次执行
func TestMockContainerMultipleExecs(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	// 启动容器
	spec := Spec{Name: "test"}
	_, err = container.Start(context.Background(), spec)
	require.NoError(t, err)

	// 多次执行命令
	for i := 0; i < 5; i++ {
		result, err := container.Exec(context.Background(), ExecRequest{
			Command: fmt.Sprintf("echo %d", i),
		})
		require.NoError(t, err)
		assert.Equal(t, 0, result.ExitCode)
		assert.Equal(t, "mock output\n", string(result.Stdout))
	}

	// 验证执行计数
	mockContainer := container.(*MockContainer)
	assert.Equal(t, 5, mockContainer.ExecCount())
}
