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

// TestNetworkManagerIntegration 测试 NetworkManager 集成功能
func TestNetworkManagerIntegration(t *testing.T) {
	m := NewManager()

	// 测试添加端口转发
	pf := &PortForward{
		Host:           9080,
		Container:      8080,
		Protocol:       "tcp",
		MaxConnections: 10,
		ConnectTimeout: 500 * time.Millisecond,
	}
	m.AddPortForward(pf)

	portForwards := m.PortForwards()
	require.Len(t, portForwards, 1)
	assert.Equal(t, 9080, portForwards[0].Host)
	assert.Equal(t, 8080, portForwards[0].Container)

	// 测试设置 SOCKS5 代理
	proxy := &SOCKS5Proxy{
		ListenAddr: "127.0.0.1:0",
		Auth: &ProxyAuth{
			Username: "test",
			Password: "secret",
		},
	}
	m.SetSOCKS5Proxy(proxy)

	retrievedProxy := m.SOCKS5Proxy()
	require.NotNil(t, retrievedProxy)
	assert.Equal(t, "127.0.0.1:0", retrievedProxy.ListenAddr)
	assert.Equal(t, "test", retrievedProxy.Auth.Username)
	assert.Equal(t, "secret", retrievedProxy.Auth.Password)
}

// TestIsPortInUse 测试端口占用检查
func TestIsPortInUse(t *testing.T) {
	// 监听一个随机端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// 端口应该被占用
	assert.True(t, IsPortInUse(port))

	// 关闭监听器
	listener.Close()

	// 等待端口释放
	time.Sleep(100 * time.Millisecond)

	// 端口应该不再被占用
	assert.False(t, IsPortInUse(port))
}

// TestSOCKS5ProxyWithForwarderIntegration 测试 SOCKS5 代理与转发器集成
func TestSOCKS5ProxyWithForwarderIntegration(t *testing.T) {
	logger := log.Global()

	// 启动后端服务器
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer backendListener.Close()

	backendPort := backendListener.Addr().(*net.TCPAddr).Port

	backendResponses := make(chan string, 1)
	go func() {
		for {
			conn, err := backendListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, _ := c.Read(buf)
				if n > 0 {
					backendResponses <- string(buf[:n])
				}
				_, _ = c.Write([]byte("backend-ok"))
			}(conn)
		}
	}()

	// 创建转发器
	forwarder := NewForwarder(logger)
	pf := &PortForward{
		Host:           0, // 随机端口
		Container:      backendPort,
		Protocol:       "tcp",
		MaxConnections: 5,
		ConnectTimeout: 1 * time.Second,
	}

	// 启动转发器监听
	hostListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer hostListener.Close()

	pf.Host = hostListener.Addr().(*net.TCPAddr).Port

	forwarder.mu.Lock()
	forwarder.portForwards = append(forwarder.portForwards, pf)
	forwarder.listeners = append(forwarder.listeners, hostListener)
	forwarder.mu.Unlock()

	go forwarder.acceptLoop(hostListener, pf)

	// 创建 SOCKS5 服务器
	server := NewSOCKS5Server("127.0.0.1:0", nil, log.Global())
	require.NotNil(t, server)

	err = server.Start()
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	// 通过 SOCKS5 代理连接到转发器
	conn, err := net.Dial("tcp", server.Addr())
	require.NoError(t, err)
	defer conn.Close()

	// 发送版本协商
	_, _ = conn.Write([]byte{0x05, 0x01, 0x00})
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	assert.Equal(t, []byte{0x05, 0x00}, buf[:n])

	// 构建 CONNECT 请求到转发器端口
	host, port, err := net.SplitHostPort(hostListener.Addr().String())
	require.NoError(t, err)

	portNum, err := net.LookupPort("tcp", port)
	require.NoError(t, err)

	var req []byte
	ip := net.ParseIP(host)
	if ip != nil && ip.To4() != nil {
		req = append(req, 0x05, 0x01, 0x00, 0x01)
		req = append(req, ip.To4()...)
	} else {
		req = append(req, 0x05, 0x01, 0x00, 0x03)
		req = append(req, byte(len(host)))
		req = append(req, host...)
	}
	req = append(req, byte(portNum>>8), byte(portNum&0xff))

	_, _ = conn.Write(req)

	// 读取 CONNECT 响应
	_, _ = conn.Read(buf)
	assert.Equal(t, byte(0x05), buf[0])
	assert.Equal(t, byte(0x00), buf[1])

	// 发送数据
	_, _ = conn.Write([]byte("hello-backend"))

	// 读取后端响应
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	response := make([]byte, 1024)
	n, err = conn.Read(response)
	require.NoError(t, err)
	assert.Equal(t, "backend-ok", string(response[:n]))
}

// TestForwarderConcurrentConnections 测试转发器并发连接
func TestForwarderConcurrentConnections(t *testing.T) {
	logger := log.Global()
	forwarder := NewForwarder(logger)

	// 启动后端服务器
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer backendListener.Close()

	go func() {
		for {
			conn, err := backendListener.Accept()
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

	// 创建转发器
	pf := &PortForward{
		Host:           0,
		Container:      backendListener.Addr().(*net.TCPAddr).Port,
		Protocol:       "tcp",
		MaxConnections: 0, // 不限制
		ConnectTimeout: 1 * time.Second,
	}

	hostListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer hostListener.Close()

	pf.Host = hostListener.Addr().(*net.TCPAddr).Port

	forwarder.mu.Lock()
	forwarder.portForwards = append(forwarder.portForwards, pf)
	forwarder.listeners = append(forwarder.listeners, hostListener)
	forwarder.mu.Unlock()

	go forwarder.acceptLoop(hostListener, pf)

	// 建立多个并发连接
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("tcp", hostListener.Addr().String())
			if err != nil {
				errors <- err
				return
			}
			defer conn.Close()

			_, err = conn.Write([]byte("ping"))
			if err != nil {
				errors <- err
				return
			}
		}()
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	var errCount int
	for err := range errors {
		t.Logf("connection error: %v", err)
		errCount++
	}

	// 允许少量连接失败（端口竞争）
	assert.Less(t, errCount, 3, "too many connection failures")
}

// TestPortForwardString 测试 PortForward String 方法
func TestPortForwardString(t *testing.T) {
	pf := &PortForward{
		Host:      8080,
		Container: 80,
		Protocol:  "tcp",
	}
	assert.Equal(t, "8080:80/tcp", pf.String())

	pfUDP := &PortForward{
		Host:      9090,
		Container: 53,
		Protocol:  "udp",
	}
	assert.Equal(t, "9090:53/udp", pfUDP.String())
}
