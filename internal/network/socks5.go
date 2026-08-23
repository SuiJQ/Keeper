package network

import (
	"fmt"
	"io"
	"net"
	"time"

	"keeper/internal/log"
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

// SOCKS5Request represents a parsed SOCKS5 CONNECT request.
type SOCKS5Request struct {
	Command  byte
	AddrType byte
	Host     string
	Port     uint16
}

// SOCKS5Response represents a parsed SOCKS5 response.
type SOCKS5Response struct {
	Version  byte
	Reply    byte
	AddrType byte
	Host     string
	Port     uint16
}

// formatIPv6 formats 16 bytes from data[offset:] as an IPv6 address string.
func formatIPv6(data []byte, offset int) string {
	return fmt.Sprintf("[%x:%x:%x:%x:%x:%x:%x:%x]",
		(uint16(data[offset])<<8)|uint16(data[offset+1]),
		(uint16(data[offset+2])<<8)|uint16(data[offset+3]),
		(uint16(data[offset+4])<<8)|uint16(data[offset+5]),
		(uint16(data[offset+6])<<8)|uint16(data[offset+7]),
		(uint16(data[offset+8])<<8)|uint16(data[offset+9]),
		(uint16(data[offset+10])<<8)|uint16(data[offset+11]),
		(uint16(data[offset+12])<<8)|uint16(data[offset+13]),
		(uint16(data[offset+14])<<8)|uint16(data[offset+15]),
	)
}

// ParseSOCKS5Request parses a SOCKS5 CONNECT request.
//
// Expected minimal layout:
//
//	VER(1) + CMD(1) + RSV(1) + ATYP(1) + DST.ADDR + DST.PORT(2)
func ParseSOCKS5Request(data []byte) (*SOCKS5Request, error) {
	if len(data) < 7 {
		return nil, fmt.Errorf("socks5 request too short: %d bytes", len(data))
	}

	if data[0] != 0x05 {
		return nil, fmt.Errorf("unsupported socks version: %d", data[0])
	}

	if data[1] != 0x01 {
		return nil, fmt.Errorf("unsupported socks command: %d", data[1])
	}

	req := &SOCKS5Request{
		Command:  data[1],
		AddrType: data[3],
	}

	switch data[3] {
	case 0x01:
		if len(data) < 10 {
			return nil, fmt.Errorf("socks5 ipv4 request too short: %d bytes", len(data))
		}
		req.Host = net.IP(data[4:8]).To4().String()
		req.Port = uint16(data[8])<<8 | uint16(data[9])
	case 0x03:
		if len(data) < 7 {
			return nil, fmt.Errorf("socks5 domain request too short: %d bytes", len(data))
		}
		domainLen := int(data[4])
		if len(data) < 7+domainLen {
			return nil, fmt.Errorf("socks5 domain request truncated: %d bytes", len(data))
		}
		req.Host = string(data[5 : 5+domainLen])
		req.Port = uint16(data[5+domainLen])<<8 | uint16(data[5+domainLen+1])
	case 0x04:
		if len(data) < 22 {
			return nil, fmt.Errorf("socks5 ipv6 request too short: %d bytes", len(data))
		}
		req.Host = formatIPv6(data, 4)
		req.Port = uint16(data[20])<<8 | uint16(data[21])
	default:
		return nil, fmt.Errorf("unsupported socks address type: %d", data[3])
	}

	return req, nil
}

// BuildSOCKS5ConnectRequest builds a SOCKS5 CONNECT request.
func BuildSOCKS5ConnectRequest(addr *net.TCPAddr) ([]byte, error) {
	if addr == nil {
		return nil, fmt.Errorf("socks5 target address is nil")
	}

	if ip := addr.IP.To4(); ip != nil {
		data := make([]byte, 10)
		data[0] = 0x05
		data[1] = 0x01
		data[2] = 0x00
		data[3] = 0x01
		copy(data[4:8], ip)
		data[8] = byte(addr.Port >> 8)
		data[9] = byte(addr.Port)
		return data, nil
	}

	if addr.IP != nil {
		data := make([]byte, 22)
		data[0] = 0x05
		data[1] = 0x01
		data[2] = 0x00
		data[3] = 0x04
		for i := 0; i < 16; i++ {
			data[4+i] = addr.IP[i]
		}
		data[20] = byte(addr.Port >> 8)
		data[21] = byte(addr.Port)
		return data, nil
	}

	return nil, fmt.Errorf("socks5 target address has no IP")
}

// BuildSOCKS5ConnectRequestHost builds a SOCKS5 CONNECT request for a host string.
func BuildSOCKS5ConnectRequestHost(host string, port uint16) ([]byte, error) {
	if host == "" {
		return nil, fmt.Errorf("socks5 target host is empty")
	}

	if ip := net.ParseIP(host); ip != nil {
		return BuildSOCKS5ConnectRequest(&net.TCPAddr{IP: ip, Port: int(port)})
	}

	hostBytes := []byte(host)
	if len(hostBytes) > 255 {
		return nil, fmt.Errorf("socks5 target host too long: %d bytes", len(hostBytes))
	}

	data := make([]byte, 7+len(hostBytes))
	data[0] = 0x05
	data[1] = 0x01
	data[2] = 0x00
	data[3] = 0x03
	data[4] = byte(len(hostBytes))
	copy(data[5:5+len(hostBytes)], hostBytes)
	data[5+len(hostBytes)] = byte(port >> 8)
	data[5+len(hostBytes)+1] = byte(port)
	return data, nil
}

// ParseSOCKS5Response parses a SOCKS5 response.
func ParseSOCKS5Response(data []byte) (*SOCKS5Response, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("socks5 response too short: %d bytes", len(data))
	}

	if data[0] != 0x05 {
		return nil, fmt.Errorf("unsupported socks version: %d", data[0])
	}

	resp := &SOCKS5Response{
		Version:  data[0],
		Reply:    data[1],
		AddrType: data[3],
	}

	switch data[3] {
	case 0x01:
		resp.Host = net.IP(data[4:8]).To4().String()
		resp.Port = uint16(data[8])<<8 | uint16(data[9])
	case 0x03:
		domainLen := int(data[4])
		resp.Host = string(data[5 : 5+domainLen])
		resp.Port = uint16(data[5+domainLen])<<8 | uint16(data[5+domainLen+1])
	case 0x04:
		resp.Host = formatIPv6(data, 4)
		resp.Port = uint16(data[20])<<8 | uint16(data[21])
	default:
		return nil, fmt.Errorf("unsupported socks address type: %d", data[3])
	}

	return resp, nil
}

// BuildSOCKS5Response builds a SOCKS5 response with the given reply code.
func BuildSOCKS5Response(addr *net.TCPAddr, reply byte) ([]byte, error) {
	if addr == nil {
		return nil, fmt.Errorf("socks5 bind address is nil")
	}

	data := make([]byte, 10)
	data[0] = 0x05
	data[1] = reply
	data[2] = 0x00

	if ip := addr.IP.To4(); ip != nil {
		data[3] = 0x01
		copy(data[4:8], ip)
	} else if len(addr.IP) >= 16 {
		data[3] = 0x04
		for i := 0; i < 16; i++ {
			data[4+i] = addr.IP[i]
		}
	} else {
		// Fallback to 0.0.0.0 for invalid/empty addresses
		data[3] = 0x01
		data[4] = 0
		data[5] = 0
		data[6] = 0
		data[7] = 0
	}

	data[8] = byte(addr.Port >> 8)
	data[9] = byte(addr.Port)

	return data, nil
}

// SOCKS5Server SOCKS5 代理服务器
type SOCKS5Server struct {
	addr    string
	auth    *ProxyAuth
	logger  log.Logger
	ln      net.Listener
	done    chan struct{}
	started bool
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
		done:   make(chan struct{}),
	}
}

// Start 启动 SOCKS5 服务器
func (s *SOCKS5Server) Start() error {
	if s.started {
		return fmt.Errorf("server already started")
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.ln = ln
	s.started = true
	go s.acceptLoop()

	s.logger.Info("SOCKS5 server started", log.Field{Key: "addr", Value: ln.Addr().String()})
	return nil
}

// Stop 停止 SOCKS5 服务器
func (s *SOCKS5Server) Stop() error {
	if !s.started {
		return nil
	}

	close(s.done)
	if s.ln != nil {
		_ = s.ln.Close()
	}
	s.started = false
	s.logger.Info("SOCKS5 server stopped")
	return nil
}

// Addr 返回服务器监听地址
func (s *SOCKS5Server) Addr() string {
	if s.ln != nil {
		return s.ln.Addr().String()
	}
	return s.addr
}

func (s *SOCKS5Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				s.logger.Warn("accept error", log.Field{Key: "error", Value: err.Error()})
				continue
			}
		}
		go s.handleConn(conn)
	}
}

func (s *SOCKS5Server) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// Version negotiation
	header := make([]byte, 3)
	if _, err := io.ReadFull(conn, header); err != nil {
		s.logger.Warn("read version error", log.Field{Key: "error", Value: err.Error()})
		return
	}

	if header[0] != 0x05 {
		s.logger.Warn("unsupported SOCKS version", log.Field{Key: "version", Value: header[0]})
		return
	}

	method := byte(0x00)
	if s.auth != nil {
		method = 0x02
	}
	if _, err := conn.Write([]byte{0x05, method}); err != nil {
		s.logger.Warn("write version response error", log.Field{Key: "error", Value: err.Error()})
		return
	}

	// Authentication
	if s.auth != nil {
		if !s.handleAuth(conn) {
			return
		}
	}

	// CONNECT request
	if !s.handleConnect(conn) {
		return
	}
}

func (s *SOCKS5Server) handleAuth(conn net.Conn) bool {
	// Set a deadline to prevent infinite blocking on malformed requests
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	// Read version and username length
	header := make([]byte, 2)
	s.logger.Info("handleAuth: reading auth header")
	if _, err := io.ReadFull(conn, header); err != nil {
		s.logger.Warn("read auth header error", log.Field{Key: "error", Value: err.Error()})
		return false
	}
	s.logger.Info("handleAuth: read auth header", log.Field{Key: "header", Value: fmt.Sprintf("%x", header)})

	if header[0] != 0x01 {
		s.logger.Warn("invalid auth version", log.Field{Key: "version", Value: header[0]})
		return false
	}

	userLen := int(header[1])

	// Read username
	userBuf := make([]byte, userLen)
	if _, err := io.ReadFull(conn, userBuf); err != nil {
		s.logger.Warn("read username error", log.Field{Key: "error", Value: err.Error()})
		return false
	}

	// Read password length
	passLenBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, passLenBuf); err != nil {
		s.logger.Warn("read password length error", log.Field{Key: "error", Value: err.Error()})
		return false
	}
	passLen := int(passLenBuf[0])

	// Read password
	passBuf := make([]byte, passLen)
	if _, err := io.ReadFull(conn, passBuf); err != nil {
		s.logger.Warn("read password error", log.Field{Key: "error", Value: err.Error()})
		return false
	}

	user := string(userBuf)
	password := string(passBuf)

	if user == s.auth.Username && password == s.auth.Password && user != "" {
		_, _ = conn.Write([]byte{0x01, 0x00})
		return true
	}

	_, _ = conn.Write([]byte{0x01, 0x01})
	s.logger.Warn("auth failed", log.Field{Key: "user", Value: user})
	return false
}

func (s *SOCKS5Server) handleConnect(conn net.Conn) bool {
	header, err := s.readConnectHeader(conn)
	if err != nil {
		return false
	}

	addrBuf, err := s.readConnectAddress(conn, header[3])
	if err != nil {
		return false
	}

	fullReq := make([]byte, 4+len(addrBuf))
	copy(fullReq, header)
	copy(fullReq[4:], addrBuf)

	req, err := ParseSOCKS5Request(fullReq)
	if err != nil {
		s.logger.Warn("parse connect request error", log.Field{Key: "error", Value: err.Error()})
		resp, _ := BuildSOCKS5Response(&net.TCPAddr{}, 0x01)
		_, _ = conn.Write(resp)
		return false
	}

	backend, err := net.Dial("tcp", net.JoinHostPort(req.Host, fmt.Sprintf("%d", req.Port)))
	if err != nil {
		s.logger.Warn("dial backend error", log.Field{Key: "error", Value: err.Error()})
		resp, _ := BuildSOCKS5Response(&net.TCPAddr{}, 0x04)
		_, _ = conn.Write(resp)
		return false
	}
	defer func() { _ = backend.Close() }()

	resp, err := BuildSOCKS5Response(backend.LocalAddr().(*net.TCPAddr), 0x00)
	if err != nil {
		s.logger.Warn("build connect response error", log.Field{Key: "error", Value: err.Error()})
		return false
	}
	if _, err := conn.Write(resp); err != nil {
		s.logger.Warn("write connect response error", log.Field{Key: "error", Value: err.Error()})
		return false
	}

	s.forward(conn, backend)
	return true
}

func (s *SOCKS5Server) readConnectHeader(conn net.Conn) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		s.logger.Warn("read connect header error", log.Field{Key: "error", Value: err.Error()})
		return nil, err
	}

	if header[0] != 0x05 || header[1] != 0x01 {
		s.logger.Warn("unsupported SOCKS command", log.Field{Key: "version", Value: header[0]}, log.Field{Key: "command", Value: header[1]})
		resp, _ := BuildSOCKS5Response(&net.TCPAddr{}, 0x07)
		_, _ = conn.Write(resp)
		return nil, fmt.Errorf("unsupported command")
	}

	return header, nil
}

func (s *SOCKS5Server) readConnectAddress(conn net.Conn, addrType byte) ([]byte, error) {
	var addrBuf []byte
	switch addrType {
	case 0x01: // IPv4
		addrBuf = make([]byte, 6)
	case 0x03: // Domain
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			s.logger.Warn("read domain length error", log.Field{Key: "error", Value: err.Error()})
			return nil, err
		}
		domainLen := int(lenBuf[0])
		addrBuf = make([]byte, 1+domainLen+2)
		addrBuf[0] = lenBuf[0]
		if _, err := io.ReadFull(conn, addrBuf[1:]); err != nil {
			s.logger.Warn("read domain error", log.Field{Key: "error", Value: err.Error()})
			return nil, err
		}
	case 0x04: // IPv6
		addrBuf = make([]byte, 18)
	default:
		s.logger.Warn("unsupported address type", log.Field{Key: "atype", Value: addrType})
		resp, _ := BuildSOCKS5Response(&net.TCPAddr{}, 0x08)
		_, _ = conn.Write(resp)
		return nil, fmt.Errorf("unsupported address type")
	}

	if _, err := io.ReadFull(conn, addrBuf); err != nil {
		s.logger.Warn("read connect address error", log.Field{Key: "error", Value: err.Error()})
		return nil, err
	}

	return addrBuf, nil
}

func (s *SOCKS5Server) forward(client, backend net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(backend, client)
		_ = client.Close()
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, backend)
		_ = backend.Close()
		done <- struct{}{}
	}()
	<-done
}
