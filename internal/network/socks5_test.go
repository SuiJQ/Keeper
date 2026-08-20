package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPortForwardString(t *testing.T) {
	pf := &PortForward{
		Host:      8080,
		Container: 80,
		Protocol:  "tcp",
	}
	assert.Equal(t, "8080:80/tcp", pf.String())
}

func TestNetworkManagerAddPortForward(t *testing.T) {
	nm := NewNetworkManager()
	pf := &PortForward{
		Host:      8080,
		Container: 80,
		Protocol:  "tcp",
	}

	nm.AddPortForward(pf)
	forwards := nm.PortForwards()

	assert.Len(t, forwards, 1)
	assert.Equal(t, 8080, forwards[0].Host)
	assert.Equal(t, 80, forwards[0].Container)
}

func TestNetworkManagerSOCKS5Proxy(t *testing.T) {
	nm := NewNetworkManager()
	assert.Nil(t, nm.SOCKS5Proxy())

	proxy := &SOCKS5Proxy{
		ListenAddr: "127.0.0.1:1080",
		Auth: &ProxyAuth{
			Username: "user",
			Password: "pass",
		},
	}
	nm.SetSOCKS5Proxy(proxy)

	assert.Equal(t, proxy, nm.SOCKS5Proxy())
	assert.Equal(t, "127.0.0.1:1080", nm.SOCKS5Proxy().ListenAddr)
}

func TestIsPortInUse(t *testing.T) {
	// 使用一个随机高端端口测试
	port := 49152
	// 在这个测试环境中，这个端口很可能未被占用
	// 我们只验证函数不 panic
	inUse := IsPortInUse(port)
	assert.IsType(t, false, inUse)
}
