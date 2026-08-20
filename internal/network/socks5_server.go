package network

import (
	"fmt"
	"net"
	"sync"
	"time"

	"keeper/internal/log"
)

// SOCKS5Server SOCKS5 代理服务器
type SOCKS5Server struct {
	mu       sync.Mutex
	running  bool
	listener net.Listener
	addr     string
	auth     *ProxyAuth
	logger   log.Logger
}

// NewSOCKS5Server 创建 SOCKS5 服务器
func NewSOCKS5Server(addr string, auth *ProxyAuth, logger log.Logger) *SOCKS5Server {
	if logger == nil {
		logger = log.Global()
	}
	return &SOCKS5Server{
		addr:   addr,
		auth:   auth,
		logger: logger,
	}
}

// Start 启动 SOCKS5 服务器
func (s *SOCKS5Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("SOCKS5 server already running")
	}

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}

	s.listener = listener
	s.running = true

	s.logger.Info("SOCKS5 server started", log.Field{Key: "addr", Value: s.addr})

	go s.acceptLoop()

	return nil
}

// Stop 停止 SOCKS5 服务器
func (s *SOCKS5Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	if s.listener != nil {
		s.listener.Close()
	}

	s.logger.Info("SOCKS5 server stopped")
	return nil
}

// acceptLoop 接受连接循环
func (s *SOCKS5Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.running {
				s.logger.Error("accept error", log.Field{Key: "error", Value: err.Error()})
			}
			return
		}

		go s.handleConnection(conn)
	}
}

// handleConnection 处理单个连接
func (s *SOCKS5Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// 设置超时
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// 1. 版本协商
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	if n < 2 || buf[0] != 0x05 {
		// 不是 SOCKS5 请求
		return
	}

	// 2. 认证
	if s.auth != nil {
		// 返回需要用户名/密码认证
		conn.Write([]byte{0x05, 0x02})
		s.handleUsernameAuth(conn)
		return
	}

	// 无需认证
	conn.Write([]byte{0x05, 0x00})

	// 3. 处理 CONNECT 请求
	s.handleConnect(conn)
}

// handleUsernameAuth 处理用户名密码认证
func (s *SOCKS5Server) handleUsernameAuth(conn net.Conn) {
	// 读取用户名/密码
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	if n < 4 || buf[0] != 0x01 {
		return
	}

	usernameLen := int(buf[1])
	passwordLen := int(buf[2+usernameLen])
	username := string(buf[2 : 2+usernameLen])
	password := string(buf[2+usernameLen : 2+usernameLen+passwordLen])

	// 验证凭据
	if username == s.auth.Username && password == s.auth.Password {
		conn.Write([]byte{0x01, 0x00}) // 认证成功
		s.handleConnect(conn)
		return
	}

	// 认证失败
	conn.Write([]byte{0x01, 0x01}) // 认证失败
}

// handleConnect 处理 CONNECT 请求
func (s *SOCKS5Server) handleConnect(conn net.Conn) {
	// 读取请求
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	if n < 7 || buf[1] != 0x01 {
		// 仅支持 CONNECT 命令
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // 命令不支持
		return
	}

	// 解析目标地址
	var target string
	switch buf[3] {
	case 0x01: // IPv4
		target = fmt.Sprintf("%d.%d.%d.%d:%d", buf[4], buf[5], buf[6], buf[7], (int(buf[8])<<8)|int(buf[9]))
	case 0x03: // 域名
		domainLen := int(buf[4])
		domain := string(buf[5 : 5+domainLen])
		port := (int(buf[5+domainLen]) << 8) | int(buf[6+domainLen])
		target = fmt.Sprintf("%s:%d", domain, port)
	case 0x04: // IPv6
		target = fmt.Sprintf("[%x:%x:%x:%x:%x:%x:%x:%x]:%d",
			(int(buf[4])<<8)|int(buf[5]), (int(buf[6])<<8)|int(buf[7]),
			(int(buf[8])<<8)|int(buf[9]), (int(buf[10])<<8)|int(buf[11]),
			(int(buf[12])<<8)|int(buf[13]), (int(buf[14])<<8)|int(buf[15]),
			(int(buf[16])<<8)|int(buf[17]), (int(buf[18])<<8)|int(buf[19]),
			(int(buf[20])<<8)|int(buf[21]))
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // 地址类型不支持
		return
	}

	// 连接到目标
	targetConn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		s.logger.Warn("connect to target failed",
			log.Field{Key: "target", Value: target},
			log.Field{Key: "error", Value: err.Error()})
		conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // 连接被拒绝
		return
	}
	defer targetConn.Close()

	// 返回连接成功
	localAddr := conn.LocalAddr().(*net.TCPAddr)
	conn.Write([]byte{
		0x05, 0x00, 0x00,
		0x01,
		byte(localAddr.IP[0]), byte(localAddr.IP[1]), byte(localAddr.IP[2]), byte(localAddr.IP[3]),
		byte(localAddr.Port >> 8), byte(localAddr.Port & 0xff),
	})

	// 清除超时
	conn.SetDeadline(time.Time{})

	// 双向转发
	done := make(chan struct{}, 2)

	go func() {
		ioCopy(conn, targetConn)
		done <- struct{}{}
	}()

	go func() {
		ioCopy(targetConn, conn)
		done <- struct{}{}
	}()

	<-done
}

// IsRunning 检查服务器是否运行中
func (s *SOCKS5Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Addr 返回监听地址
func (s *SOCKS5Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.addr
}
