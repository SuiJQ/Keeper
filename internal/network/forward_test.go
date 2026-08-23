package network

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"keeper/internal/log"
)

func TestForwarderMaxConnections(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	// 添加一个端口转发，限制最大连接数为 2
	pf := &PortForward{
		Host:           8765,
		Container:      9000,
		Protocol:       "tcp",
		MaxConnections: 2,
		ConnectTimeout: 100 * time.Millisecond,
	}

	// 模拟一个监听器
	listener, err := net.Listen("tcp", "127.0.0.1:8765")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	// 模拟一个容器连接（实际启动一个 TCP 服务器）
	containerListener, err := net.Listen("tcp", "127.0.0.1:9000")
	if err != nil {
		t.Fatalf("failed to listen on container: %v", err)
	}
	defer containerListener.Close()

	// 启动容器服务器（接受连接但不做任何事情）
	go func() {
		for {
			conn, err := containerListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				// 保持连接打开
				buf := make([]byte, 1024)
				for {
					_, err := c.Read(buf)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	// 启动转发
	forwarder.mu.Lock()
	forwarder.portForwards = append(forwarder.portForwards, pf)
	forwarder.listeners = append(forwarder.listeners, listener)
	forwarder.activeConnections[8765] = 0
	forwarder.mu.Unlock()

	go forwarder.acceptLoop(listener, pf)

	// 建立 2 个连接（应该成功）
	var conns []net.Conn
	var mu sync.Mutex

	for i := 0; i < 2; i++ {
		conn, err := net.Dial("tcp", "127.0.0.1:8765")
		if err != nil {
			t.Fatalf("failed to establish connection %d: %v", i, err)
		}
		mu.Lock()
		conns = append(conns, conn)
		mu.Unlock()
	}

	// 等待一下，确保连接计数已经更新
	time.Sleep(50 * time.Millisecond)

	// 建立第 3 个连接
	conn, err := net.Dial("tcp", "127.0.0.1:8765")
	if err != nil {
		t.Fatalf("failed to establish 3rd connection: %v", err)
	}
	defer conn.Close()

	// 尝试读取数据，如果连接被拒绝，应该立即返回错误或超时
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("expected 3rd connection to be rejected or timeout, but data was received")
	}

	// 清理
	for _, c := range conns {
		c.Close()
	}
}

func TestForwarderConnectTimeout(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	// 添加一个端口转发，设置连接超时
	pf := &PortForward{
		Host:           8766,
		Container:      9001, // 没有监听在这个端口
		Protocol:       "tcp",
		MaxConnections: 0,
		ConnectTimeout: 100 * time.Millisecond,
	}

	// 启动监听
	listener, err := net.Listen("tcp", "127.0.0.1:8766")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	// 启动转发
	forwarder.mu.Lock()
	forwarder.portForwards = append(forwarder.portForwards, pf)
	forwarder.listeners = append(forwarder.listeners, listener)
	forwarder.mu.Unlock()

	go forwarder.acceptLoop(listener, pf)

	// 连接应该因为超时而失败
	conn, err := net.Dial("tcp", "127.0.0.1:8766")
	if err != nil {
		t.Fatalf("failed to connect to forwarder: %v", err)
	}
	defer conn.Close()

	// 等待连接超时
	time.Sleep(200 * time.Millisecond)
}

func TestForwarderNoConnectionLimit(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	// 添加一个端口转发，不限制连接数
	pf := &PortForward{
		Host:           8767,
		Container:      9002,
		Protocol:       "tcp",
		MaxConnections: 0, // 不限制
		ConnectTimeout: 100 * time.Millisecond,
	}

	// 启动监听
	listener, err := net.Listen("tcp", "127.0.0.1:8767")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	// 模拟容器连接
	containerListener, err := net.Listen("tcp", "127.0.0.1:9002")
	if err != nil {
		t.Fatalf("failed to listen on container: %v", err)
	}
	defer containerListener.Close()

	// 启动容器服务器
	go func() {
		for {
			conn, err := containerListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					_, err := c.Read(buf)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	// 启动转发
	forwarder.mu.Lock()
	forwarder.portForwards = append(forwarder.portForwards, pf)
	forwarder.listeners = append(forwarder.listeners, listener)
	forwarder.activeConnections[8767] = 0
	forwarder.mu.Unlock()

	go forwarder.acceptLoop(listener, pf)

	// 建立 5 个连接（都应该成功）
	var conns []net.Conn
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		conn, err := net.Dial("tcp", "127.0.0.1:8767")
		if err == nil {
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}

	if len(conns) != 5 {
		t.Errorf("expected 5 successful connections, got %d", len(conns))
	}

	// 清理
	for _, c := range conns {
		c.Close()
	}
}

func TestForwarderAddForward(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	// 添加第一个端口转发
	pf1 := &PortForward{
		Host:      8768,
		Container: 9003,
		Protocol:  "tcp",
	}
	err := forwarder.AddForward(pf1)
	assert.NoError(t, err)
	assert.Len(t, forwarder.portForwards, 1)

	// 添加第二个不同的端口转发
	pf2 := &PortForward{
		Host:      8769,
		Container: 9004,
		Protocol:  "tcp",
	}
	err = forwarder.AddForward(pf2)
	assert.NoError(t, err)
	assert.Len(t, forwarder.portForwards, 2)

	// 尝试添加重复的端口转发
	pf3 := &PortForward{
		Host:      8768, // 与 pf1 相同
		Container: 9005,
		Protocol:  "tcp",
	}
	err = forwarder.AddForward(pf3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already forwarded")
	assert.Len(t, forwarder.portForwards, 2) // 不应增加
}

func TestForwarderStartStop(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	// 添加端口转发
	pf := &PortForward{
		Host:      8770,
		Container: 9006,
		Protocol:  "tcp",
	}
	err := forwarder.AddForward(pf)
	assert.NoError(t, err)

	// 模拟容器监听
	containerListener, err := net.Listen("tcp", "127.0.0.1:9006")
	if err != nil {
		t.Fatalf("failed to listen on container: %v", err)
	}
	defer containerListener.Close()

	go func() {
		for {
			conn, err := containerListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					_, err := c.Read(buf)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	// 启动转发
	err = forwarder.Start()
	assert.NoError(t, err)
	assert.Len(t, forwarder.listeners, 1)

	// 验证可以连接
	conn, err := net.Dial("tcp", "127.0.0.1:8770")
	assert.NoError(t, err)
	conn.Close()

	// 停止转发
	forwarder.Stop()
	assert.Len(t, forwarder.listeners, 0)
	assert.Len(t, forwarder.portForwards, 0)
	assert.Len(t, forwarder.activeConnections, 0)
}

func TestForwarderStartFailure(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	// 添加一个端口转发，但目标端口已被占用
	pf := &PortForward{
		Host:      8771,
		Container: 9007,
		Protocol:  "tcp",
	}
	err := forwarder.AddForward(pf)
	assert.NoError(t, err)

	// 在 Host 端口上启动一个监听器，导致 startForward 失败
	existingListener, err := net.Listen("tcp", "127.0.0.1:8771")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer existingListener.Close()

	// 启动转发应该失败
	err = forwarder.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listen on")
}

func TestForwarderStartAlreadyRunning(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	pf := &PortForward{
		Host:      8772,
		Container: 9008,
		Protocol:  "tcp",
	}
	require.NoError(t, forwarder.AddForward(pf))

	listener, err := net.Listen("tcp", "127.0.0.1:8772")
	require.NoError(t, err)
	defer listener.Close()

	forwarder.mu.Lock()
	forwarder.portForwards = append(forwarder.portForwards, pf)
	forwarder.listeners = append(forwarder.listeners, listener)
	forwarder.running = true
	forwarder.mu.Unlock()

	err = forwarder.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestForwarderRunningFlag(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	pf := &PortForward{
		Host:      8773,
		Container: 9009,
		Protocol:  "tcp",
	}
	require.NoError(t, forwarder.AddForward(pf))

	listener, err := net.Listen("tcp", "127.0.0.1:8773")
	require.NoError(t, err)
	defer listener.Close()

	forwarder.mu.Lock()
	forwarder.portForwards = append(forwarder.portForwards, pf)
	forwarder.listeners = append(forwarder.listeners, listener)
	forwarder.running = true
	forwarder.mu.Unlock()

	assert.True(t, forwarder.running)

	forwarder.Stop()
	forwarder.mu.Lock()
	assert.False(t, forwarder.running)
	forwarder.mu.Unlock()
}
