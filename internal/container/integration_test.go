package container

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"keeper/internal/network"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBwrapFactoryLifecycle 测试 bwrap 容器生命周期（mock 模式）
// 注意：在当前环境（内核 5.10）中无法运行真实 bwrap，使用 mock 后端验证逻辑
func TestBwrapFactoryLifecycle(t *testing.T) {
	// 创建临时目录作为 KEEPER_HOME
	tmpDir, err := os.MkdirTemp("", "keeper-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 创建必要的目录结构
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "agents", "test-agent"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "cache", "rootfs"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "agents", "test-agent", "upper"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "agents", "test-agent", "work"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "agents", "test-agent", "workspace"), 0755))

	// 创建 mock 工厂
	factory := NewMockFactory(nil)
	require.NotNil(t, factory)

	// 1. 创建容器
	c, err := factory.Create("test-agent")
	require.NoError(t, err)
	require.NotNil(t, c)

	// 2. 构建容器规格
	spec := Spec{
		Name:      "test-agent",
		Rootfs:    filepath.Join(tmpDir, "cache", "rootfs"),
		UpperDir:  filepath.Join(tmpDir, "agents", "test-agent", "upper"),
		WorkDir:   filepath.Join(tmpDir, "agents", "test-agent", "work"),
		Workspace: filepath.Join(tmpDir, "agents", "test-agent", "workspace"),
		ShmSize:   64,
		Envvars:   []string{"AGENT_NAME=test-agent", "TEST=value"},
		Ports:     []PortMapping{{Host: 8080, Container: 80}},
	}

	// 3. 启动容器
	pid, err := c.Start(context.Background(), spec)
	require.NoError(t, err)
	assert.Equal(t, 12345, pid)
	assert.True(t, func() bool { c, _ := c.(*MockContainer); return c.IsStarted() }())

	// 4. 查询状态
	status, err := c.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "running", status.State)
	assert.Equal(t, 12345, status.PID)

	// 5. 执行命令
	req := ExecRequest{
		Command: "echo hello",
		Env:     []string{"FOO=bar"},
	}
	resp, err := c.Exec(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, 0, resp.ExitCode)
	assert.Equal(t, []byte("mock output\n"), resp.Stdout)

	mock, _ := c.(*MockContainer)
	assert.Equal(t, 1, mock.ExecCount())

	// 6. 停止容器
	err = c.Stop(context.Background(), 5*time.Second)
	require.NoError(t, err)
	assert.True(t, func() bool { c, _ := c.(*MockContainer); return c.IsStopped() }())

	// 7. 验证状态已更新
	status, err = c.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "stopped", status.State)
	assert.Equal(t, 0, status.PID)

	// 8. 清理
	err = c.Close()
	require.NoError(t, err)
}

// TestMockContainerConcurrent 测试并发安全
func TestMockContainerConcurrent(t *testing.T) {
	factory := NewMockFactory(nil)
	c, err := factory.Create("concurrent-test")
	require.NoError(t, err)

	_, err = c.Start(context.Background(), Spec{})
	require.NoError(t, err)

	// 并发执行命令
	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func(_ int) {
			_, err := c.Exec(context.Background(), ExecRequest{Command: "cmd"})
			assert.NoError(t, err)
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	mock, _ := c.(*MockContainer)
	assert.Equal(t, 10, mock.ExecCount())
}

// TestPortForwardRoundTrip 测试端口转发 round trip
func TestPortForwardRoundTrip(t *testing.T) {
	// 启动一个简单的 TCP 服务器作为"容器"
	containerListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer containerListener.Close()

	containerAddr := containerListener.Addr().String()
	serverReady := make(chan struct{})
	go func() {
		conn, _ := containerListener.Accept()
		if conn != nil {
			defer conn.Close()
			buf := make([]byte, 1024)
			n, _ := conn.Read(buf)
			if n > 0 {
				_, _ = conn.Write(buf[:n]) // echo back
			}
		}
		close(serverReady)
	}()

	// 创建端口转发器
	forwarder := network.NewForwarder(nil)

	hostPort := 12345
	_, portStr, _ := net.SplitHostPort(containerAddr)
	containerPort, _ := strconv.Atoi(portStr)
	pf := &network.PortForward{
		Host:      hostPort,
		Container: containerPort,
		Protocol:  "tcp",
	}

	err = forwarder.AddForward(pf)
	require.NoError(t, err)

	err = forwarder.Start()
	require.NoError(t, err)
	defer forwarder.Stop()

	// 客户端连接到主机端口
	clientConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", hostPort))
	require.NoError(t, err)
	defer clientConn.Close()

	// 发送数据
	message := []byte("hello world")
	_, err = clientConn.Write(message)
	require.NoError(t, err)

	// 读取 echo 响应
	buf := make([]byte, 1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := clientConn.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, message, buf[:n])

	<-serverReady
}
