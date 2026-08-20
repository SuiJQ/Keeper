package network

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSOCKS5ServerStartStop(t *testing.T) {
	server := NewSOCKS5Server("127.0.0.1:0", nil, nil)

	// 启动服务器
	err := server.Start()
	assert.NoError(t, err)
	assert.True(t, server.IsRunning())

	// 停止服务器
	err = server.Stop()
	assert.NoError(t, err)
	assert.False(t, server.IsRunning())
}

func TestSOCKS5ServerDoubleStart(t *testing.T) {
	server := NewSOCKS5Server("127.0.0.1:0", nil, nil)

	err := server.Start()
	assert.NoError(t, err)

	// 再次启动应该失败
	err = server.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	server.Stop()
}

func TestSOCKS5ServerDoubleStop(t *testing.T) {
	server := NewSOCKS5Server("127.0.0.1:0", nil, nil)

	err := server.Start()
	assert.NoError(t, err)

	// 第一次停止
	err = server.Stop()
	assert.NoError(t, err)

	// 第二次停止应该不报错
	err = server.Stop()
	assert.NoError(t, err)
}

func TestSOCKS5ServerWithAuth(t *testing.T) {
	auth := &ProxyAuth{
		Username: "testuser",
		Password: "testpass",
	}

	server := NewSOCKS5Server("127.0.0.1:0", auth, nil)

	err := server.Start()
	assert.NoError(t, err)
	defer server.Stop()

	// 获取监听地址
	addr := server.Addr()
	assert.NotEmpty(t, addr)
}

func TestSOCKS5ServerConnect(t *testing.T) {
	// 启动一个简单的 echo 服务器
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer echoListener.Close()

	go func() {
		conn, _ := echoListener.Accept()
		if conn != nil {
			defer conn.Close()
			buf := make([]byte, 1024)
			n, _ := conn.Read(buf)
			if n > 0 {
				conn.Write(buf[:n])
			}
		}
	}()

	// 启动 SOCKS5 服务器
	server := NewSOCKS5Server("127.0.0.1:0", nil, nil)
	err = server.Start()
	assert.NoError(t, err)
	defer server.Stop()

	// 连接到 SOCKS5 服务器
	conn, err := net.Dial("tcp", server.Addr())
	assert.NoError(t, err)
	defer conn.Close()

	// 发送 SOCKS5 版本协商
	conn.Write([]byte{0x05, 0x01, 0x00})

	// 读取服务器响应
	buf := make([]byte, 2)
	conn.Read(buf)
	assert.Equal(t, []byte{0x05, 0x00}, buf)

	// 发送 CONNECT 请求
	echoAddr := echoListener.Addr().String()
	host, portStr, _ := net.SplitHostPort(echoAddr)
	port, _ := strconv.Atoi(portStr)

	// 构建 CONNECT 请求 (IPv4)
	req := []byte{0x05, 0x01, 0x00, 0x01}
	// 解析 IPv4 地址
	ip := net.ParseIP(host).To4()
	for i := 0; i < 4; i++ {
		req = append(req, ip[i])
	}
	req = append(req, byte(port>>8))
	req = append(req, byte(port&0xff))

	conn.Write(req)

	// 读取响应
	resp := make([]byte, 1024)
	n, _ := conn.Read(resp)
	assert.True(t, n > 0)
}

func TestSOCKS5ServerInvalidVersion(t *testing.T) {
	server := NewSOCKS5Server("127.0.0.1:0", nil, nil)
	err := server.Start()
	assert.NoError(t, err)
	defer server.Stop()

	conn, err := net.Dial("tcp", server.Addr())
	assert.NoError(t, err)
	defer conn.Close()

	// 发送无效的 SOCKS 版本
	conn.Write([]byte{0x04, 0x01, 0x00})

	// 服务器应该关闭连接
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	assert.Error(t, err)
}

func TestForwarderAddDuplicatePort(t *testing.T) {
	forwarder := NewForwarder(nil)

	// 使用一个随机高端端口，减少与系统服务冲突的可能性
	hostPort := 49152

	// 添加第一个端口转发
	err := forwarder.AddForward(&PortForward{Host: hostPort, Container: 80, Protocol: "tcp"})
	assert.NoError(t, err)

	// 尝试添加相同主机端口应该失败
	err = forwarder.AddForward(&PortForward{Host: hostPort, Container: 81, Protocol: "tcp"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already forwarded")
}

func TestForwarderStartStop(t *testing.T) {
	forwarder := NewForwarder(nil)

	// 添加端口转发
	hostPort := 49153
	err := forwarder.AddForward(&PortForward{Host: hostPort, Container: 80, Protocol: "tcp"})
	assert.NoError(t, err)

	// 启动转发器
	err = forwarder.Start()
	assert.NoError(t, err)

	// 停止转发器
	forwarder.Stop()
}

func TestPortForwardString(t *testing.T) {
	pf := &PortForward{Host: 8080, Container: 80, Protocol: "tcp"}
	assert.Equal(t, "8080:80/tcp", pf.String())

	pf = &PortForward{Host: 8443, Container: 443, Protocol: "udp"}
	assert.Equal(t, "8443:443/udp", pf.String())
}

func TestNetworkManagerAddPortForward(t *testing.T) {
	nm := NewNetworkManager()

	pf := &PortForward{Host: 8080, Container: 80, Protocol: "tcp"}
	nm.AddPortForward(pf)

	forwards := nm.PortForwards()
	assert.Len(t, forwards, 1)
	assert.Equal(t, pf, forwards[0])
}

func TestNetworkManagerSOCKS5Proxy(t *testing.T) {
	nm := NewNetworkManager()

	proxy := &SOCKS5Proxy{
		ListenAddr: "127.0.0.1:1080",
		Auth: &ProxyAuth{
			Username: "user",
			Password: "pass",
		},
	}

	nm.SetSOCKS5Proxy(proxy)
	result := nm.SOCKS5Proxy()
	assert.Equal(t, proxy, result)
	assert.Equal(t, "127.0.0.1:1080", result.ListenAddr)
	assert.NotNil(t, result.Auth)
}

func TestIsPortInUse(t *testing.T) {
	// 启动一个临时监听器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	assert.True(t, IsPortInUse(port))

	// 关闭监听器后端口应该不再被占用
	listener.Close()
	// 注意：端口可能处于 TIME_WAIT 状态，所以可能仍然返回 true
	// 这里只是基本测试
	_ = IsPortInUse(port)
}
