package container

import (
	"context"
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
	pid, err := container.Start(context.Background(), ContainerSpec{})
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

	_, err = container.Start(context.Background(), ContainerSpec{})
	require.NoError(t, err)

	// 再次启动应该失败
	_, err = container.Start(context.Background(), ContainerSpec{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")
}

func TestMockContainerExec(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	_, err = container.Start(context.Background(), ContainerSpec{})
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

func TestMockContainerMultipleExecs(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	_, err = container.Start(context.Background(), ContainerSpec{})
	require.NoError(t, err)

	// 执行多个命令
	_, err = container.Exec(context.Background(), ExecRequest{Command: "cmd1"})
	require.NoError(t, err)
	_, err = container.Exec(context.Background(), ExecRequest{Command: "cmd2"})
	require.NoError(t, err)

	mock, _ := container.(*MockContainer)
	assert.Equal(t, 2, mock.ExecCount())
	lastExec, _ := mock.LastExec()
	assert.Equal(t, "cmd2", lastExec.Command)
}

func TestMockContainerStopNotStarted(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	// 停止未启动的容器应该返回 nil
	err = container.Stop(context.Background(), 5*time.Second)
	assert.NoError(t, err)
}

func TestMockContainerExecNotStarted(t *testing.T) {
	factory := NewMockFactory(nil)
	container, err := factory.Create("test")
	require.NoError(t, err)

	// 在未启动的容器上执行命令应该成功（mock 不限制）
	_, err = container.Exec(context.Background(), ExecRequest{Command: "echo hello"})
	assert.NoError(t, err)
}
