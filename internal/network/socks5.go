package network

import (
	"fmt"
	"net"
	"time"
)

// PortForward 端口转发配置
type PortForward struct {
	// Host 宿主机端口
	Host int
	// Container 容器端口
	Container int
	// Protocol 协议，支持 tcp / udp
	Protocol string
	// MaxConnections 最大并发连接数，0 表示不限制
	MaxConnections int
	// ConnectTimeout 连接超时时间
	ConnectTimeout time.Duration
}

// String 返回端口转发的字符串表示
func (p *PortForward) String() string {
	return fmt.Sprintf("%d:%d/%s", p.Host, p.Container, p.Protocol)
}

// SOCKS5Proxy SOCKS5 代理配置
type SOCKS5Proxy struct {
	// ListenAddr 监听地址
	ListenAddr string
	// Auth 认证配置
	Auth *ProxyAuth
}

// ProxyAuth 代理认证配置
type ProxyAuth struct {
	// Username 用户名
	Username string
	// Password 密码
	Password string
}

// Manager 网络管理器
type Manager struct {
	portForwards []*PortForward
	proxy        *SOCKS5Proxy
}

// NewManager 创建网络管理器
func NewManager() *Manager {
	return &Manager{
		portForwards: make([]*PortForward, 0),
	}
}

// AddPortForward 添加端口转发
func (m *Manager) AddPortForward(pf *PortForward) {
	m.portForwards = append(m.portForwards, pf)
}

// PortForwards 返回端口转发列表
func (m *Manager) PortForwards() []*PortForward {
	return m.portForwards
}

// SetSOCKS5Proxy 设置 SOCKS5 代理
func (m *Manager) SetSOCKS5Proxy(proxy *SOCKS5Proxy) {
	m.proxy = proxy
}

// SOCKS5Proxy 返回 SOCKS5 代理配置
func (m *Manager) SOCKS5Proxy() *SOCKS5Proxy {
	return m.proxy
}

// IsPortInUse 检查端口是否被占用
func IsPortInUse(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return true
	}
	_ = listener.Close()
	return false
}
