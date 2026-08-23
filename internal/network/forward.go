package network

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"keeper/internal/log"
)

// bufferPool 复用缓冲区，减少 GC 压力
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

// Forwarder 端口转发器
type Forwarder struct {
	mu           sync.Mutex
	running      bool
	portForwards []*PortForward
	listeners    []net.Listener
	logger       log.Logger
	// activeConnections 记录每个端口的活跃连接数
	activeConnections map[int]int
	// connections 记录每个端口的活跃连接，用于优雅关闭时主动关闭
	connections map[int][]net.Conn
	// shutdown 用于通知 acceptLoop 退出
	shutdown context.Context
	cancel   context.CancelFunc
}

// NewForwarder 创建端口转发器
func NewForwarder(logger log.Logger) *Forwarder {
	if logger == nil {
		logger = log.Global()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Forwarder{
		portForwards:      make([]*PortForward, 0),
		listeners:         make([]net.Listener, 0),
		logger:            logger,
		activeConnections: make(map[int]int),
		connections:       make(map[int][]net.Conn),
		shutdown:          ctx,
		cancel:            cancel,
	}
}

// AddForward 添加端口转发
func (f *Forwarder) AddForward(pf *PortForward) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 检查端口是否已被占用（系统级）
	if IsPortInUse(pf.Host) {
		return fmt.Errorf("host port %d already in use", pf.Host)
	}

	// 检查是否已在转发列表中
	for _, existing := range f.portForwards {
		if existing.Host == pf.Host {
			return fmt.Errorf("host port %d already forwarded", pf.Host)
		}
	}

	f.portForwards = append(f.portForwards, pf)
	return nil
}

// Start 启动所有端口转发
func (f *Forwarder) Start() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.running {
		return fmt.Errorf("port forwarder already running")
	}
	startTime := time.Now()
	for _, pf := range f.portForwards {
		if err := f.startForward(pf); err != nil {
			f.stopInternal()
			RecordPortForward("error")
			RecordPortForwardDuration(time.Since(startTime).Seconds())
			return fmt.Errorf("start forward %s: %w", pf.String(), err)
		}
	}
	f.running = true

	RecordPortForward("success")
	RecordPortForwardDuration(time.Since(startTime).Seconds())
	return nil
}

// startForward 启动单个端口转发
func (f *Forwarder) startForward(pf *PortForward) error {
	addr := fmt.Sprintf(":%d", pf.Host)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	f.logger.Info("port forward started",
		log.Field{Key: "host", Value: pf.Host},
		log.Field{Key: "container", Value: pf.Container})

	f.listeners = append(f.listeners, listener)

	// 启动转发循环
	go f.acceptLoop(listener, pf)

	return nil
}

// acceptLoop 接受连接循环
func (f *Forwarder) acceptLoop(listener net.Listener, pf *PortForward) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			f.mu.Lock()
			running := f.running
			f.mu.Unlock()
			if running {
				f.logger.Debug("accept error", log.Field{Key: "error", Value: err.Error()})
			}
			return
		}

		// 检查连接数限制
		if pf.MaxConnections > 0 {
			f.mu.Lock()
			current := f.activeConnections[pf.Host]
			if current >= pf.MaxConnections {
				f.mu.Unlock()
				_ = conn.Close()
				f.logger.Warn("max connections reached, rejecting new connection",
					log.Field{Key: "host", Value: pf.Host},
					log.Field{Key: "max", Value: pf.MaxConnections},
					log.Field{Key: "current", Value: current})
				RecordProxyConnection("rejected")
				SetForwarderActiveConnections(fmt.Sprintf("%d", pf.Host), f.activeConnections[pf.Host])
				continue
			}
			f.activeConnections[pf.Host]++
			SetForwarderActiveConnections(fmt.Sprintf("%d", pf.Host), f.activeConnections[pf.Host])
			f.mu.Unlock()
		}

		go f.handleConnection(conn, pf)
	}
}

// handleConnection 处理单个连接
func (f *Forwarder) handleConnection(clientConn net.Conn, pf *PortForward) {
	startTime := time.Now()
	RecordProxyConnection("attempt")

	f.trackConnection(clientConn, pf)
	defer f.cleanupConnection(clientConn, pf)

	containerConn, err := f.dialContainer(pf)
	if err != nil {
		RecordProxyConnection("error")
		return
	}
	defer func() { _ = containerConn.Close() }()

	f.forwardData(clientConn, containerConn)
	RecordProxyConnection("success")
	RecordProxyConnectionDuration(time.Since(startTime).Seconds())
}

// trackConnection 注册连接以便优雅关闭时追踪
func (f *Forwarder) trackConnection(clientConn net.Conn, pf *PortForward) {
	f.mu.Lock()
	f.connections[pf.Host] = append(f.connections[pf.Host], clientConn)
	f.mu.Unlock()
}

// cleanupConnection 清理连接追踪与活跃计数
func (f *Forwarder) cleanupConnection(clientConn net.Conn, pf *PortForward) {
	_ = clientConn.Close()

	f.mu.Lock()
	if conns, ok := f.connections[pf.Host]; ok {
		for i, c := range conns {
			if c == clientConn {
				f.connections[pf.Host] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
	}
	f.mu.Unlock()

	if pf.MaxConnections > 0 {
		f.mu.Lock()
		f.activeConnections[pf.Host]--
		if f.activeConnections[pf.Host] < 0 {
			f.activeConnections[pf.Host] = 0
		}
		SetForwarderActiveConnections(fmt.Sprintf("%d", pf.Host), f.activeConnections[pf.Host])
		f.mu.Unlock()
	}
}

// dialContainer 连接到容器端口
func (f *Forwarder) dialContainer(pf *PortForward) (net.Conn, error) {
	containerAddr := fmt.Sprintf("127.0.0.1:%d", pf.Container)
	connectTimeout := pf.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}

	containerConn, err := net.DialTimeout("tcp", containerAddr, connectTimeout)
	if err != nil {
		f.logger.Warn("connect to container failed",
			log.Field{Key: "container", Value: containerAddr},
			log.Field{Key: "error", Value: err.Error()})
		return nil, err
	}
	return containerConn, nil
}

// forwardData 双向数据转发
func (f *Forwarder) forwardData(clientConn, containerConn net.Conn) {
	done := make(chan struct{}, 2)

	go func() {
		n, err := ioCopy(clientConn, containerConn)
		if err != nil {
			_ = containerConn.Close()
		}
		RecordDataTransfer("to_container", n)
		done <- struct{}{}
	}()

	go func() {
		n, err := ioCopy(containerConn, clientConn)
		if err != nil {
			_ = clientConn.Close()
		}
		RecordDataTransfer("to_client", n)
		done <- struct{}{}
	}()

	for i := 0; i < 2; i++ {
		<-done
	}
}

// ioCopy 双向数据复制（带超时，使用缓冲池）
func ioCopy(dst, src net.Conn) (int64, error) {
	bufPtr := bufferPool.Get().(*[]byte)
	buf := *bufPtr
	defer bufferPool.Put(bufPtr)

	var total int64
	for {
		_ = src.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := src.Read(buf)
		if err != nil {
			return total, err
		}
		total += int64(n)
		_ = dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if _, err := dst.Write(buf[:n]); err != nil {
			return total, err
		}
	}
}

// Shutdown 优雅关闭端口转发器。
// 它会停止接受新连接，等待现有连接完成（或 ctx 超时），然后关闭所有监听器和活跃连接。
func (f *Forwarder) Shutdown(ctx context.Context) error {
	f.mu.Lock()
	if !f.running {
		f.mu.Unlock()
		return nil
	}
	f.running = false
	f.mu.Unlock()

	// 取消 shutdown context，通知 acceptLoop 退出
	f.cancel()

	// 关闭所有监听器，停止接受新连接
	f.mu.Lock()
	listeners := f.listeners
	f.listeners = nil
	f.mu.Unlock()
	for _, l := range listeners {
		_ = l.Close()
	}

	// 等待上下文完成或超时
	if ctx != nil {
		<-ctx.Done()
	}

	// 关闭所有活跃连接（优雅关闭超时后的强制清理）
	f.mu.Lock()
	for host, conns := range f.connections {
		for _, conn := range conns {
			_ = conn.Close()
		}
		f.connections[host] = nil
	}
	f.mu.Unlock()

	// 等待一小段时间让 goroutine 清理
	time.Sleep(50 * time.Millisecond)

	f.mu.Lock()
	f.portForwards = nil
	f.activeConnections = make(map[int]int)
	f.connections = make(map[int][]net.Conn)
	f.mu.Unlock()

	return nil
}

// Stop 停止所有端口转发（硬停止）
func (f *Forwarder) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopInternal()
}

// stopInternal 内部停止方法（不持有锁，避免死锁）
func (f *Forwarder) stopInternal() {
	f.cancel()

	for _, listener := range f.listeners {
		_ = listener.Close()
	}
	for _, conns := range f.connections {
		for _, conn := range conns {
			_ = conn.Close()
		}
	}
	f.listeners = nil
	f.portForwards = nil
	f.activeConnections = make(map[int]int)
	f.connections = make(map[int][]net.Conn)
	f.running = false

	// Reset context for potential reuse
	ctx, cancel := context.WithCancel(context.Background())
	f.shutdown = ctx
	f.cancel = cancel
}
