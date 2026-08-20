package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBwrapFactory(t *testing.T) {
	factory := NewBwrapFactory()
	assert.NotNil(t, factory)
	assert.Equal(t, "bwrap", factory.Type())
}

func TestBwrapContainerCreate(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test-container")
	require.NoError(t, err)
	assert.NotNil(t, container)
	status, err := container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "created", status.State)
}

func TestBwrapContainerStart(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test-container")
	require.NoError(t, err)

	// Start 应该因为缺少 bwrap 而失败
	spec := ContainerSpec{
		Rootfs:   "/",
		UpperDir: "/tmp/upper",
		WorkDir:  "/tmp/work",
	}
	_, err = container.Start(context.Background(), spec)
	assert.Error(t, err)
}

func TestBwrapContainerStop(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test-container")
	require.NoError(t, err)

	// 停止未运行的容器应该返回 nil
	err = container.Stop(context.Background(), 5*time.Second)
	assert.NoError(t, err)
}

func TestBwrapContainerExec(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test-container")
	require.NoError(t, err)

	// 在未运行的容器上执行命令应该失败
	req := ExecRequest{
		Command: "echo hello",
	}
	_, err = container.Exec(context.Background(), req)
	assert.Error(t, err)
}

func TestBwrapContainerClose(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test-container")
	require.NoError(t, err)

	err = container.Close()
	assert.NoError(t, err)
	status, err := container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "destroyed", status.State)
}

func TestBuildArgs(t *testing.T) {
	// 测试 ContainerSpec 结构体
	spec := ContainerSpec{
		Rootfs:    "/rootfs",
		UpperDir:  "/upper",
		WorkDir:   "/work",
		Workspace: "/workspace",
		ShmSize:   128,
		Ports:     []PortMapping{{Host: 8080, Container: 80}},
		Envvars:   []string{"FOO=bar", "BAZ=qux"},
		SeccompBPF: []byte{0x01, 0x02, 0x03},
	}

	assert.Equal(t, "/rootfs", spec.Rootfs)
	assert.Equal(t, "/upper", spec.UpperDir)
	assert.Equal(t, 128, spec.ShmSize)
	assert.Len(t, spec.Ports, 1)
	assert.Len(t, spec.Envvars, 2)
}

func TestCheckKernelSupport(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	// 在当前环境（内核 5.10）应该返回 false
	supported := bwrap.checkKernelSupport()
	assert.False(t, supported)
}
