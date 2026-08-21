package network

import (
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
	portForwards []*PortForward
	listeners    []net.Listener
	logger       log.Logger
}

// NewForwarder 创建端口转发器
func NewForwarder(logger log.Logger) *Forwarder {
	if logger == nil {
		logger = log.Global()
	}
	return &Forwarder{
		portForwards: make([]*PortForward, 0),
		listeners:    make([]net.Listener, 0),
		logger:       logger,
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

	startTime := time.Now()
	for _, pf := range f.portForwards {
		if err := f.startForward(pf); err != nil {
			f.Stop()
			RecordPortForward("error")
			RecordPortForwardDuration(time.Since(startTime).Seconds())
			return fmt.Errorf("start forward %s: %w", pf.String(), err)
		}
	}

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
			f.logger.Debug("accept error", log.Field{Key: "error", Value: err.Error()})
			return
		}

		go f.handleConnection(conn, pf)
	}
}

// handleConnection 处理单个连接
func (f *Forwarder) handleConnection(clientConn net.Conn, pf *PortForward) {
	startTime := time.Now()
	RecordProxyConnection("attempt")

	defer clientConn.Close()

	// 连接到容器端口
	containerAddr := fmt.Sprintf("127.0.0.1:%d", pf.Container)
	containerConn, err := net.DialTimeout("tcp", containerAddr, 5*time.Second)
	if err != nil {
		f.logger.Warn("connect to container failed",
			log.Field{Key: "container", Value: containerAddr},
			log.Field{Key: "error", Value: err.Error()})
		RecordProxyConnection("error")
		return
	}
	defer containerConn.Close()

	// 双向数据转发
	done := make(chan struct{}, 2)

	go func() {
		n, _ := ioCopy(clientConn, containerConn)
		RecordDataTransfer("to_container", int64(n))
		done <- struct{}{}
	}()

	go func() {
		n, _ := ioCopy(containerConn, clientConn)
		RecordDataTransfer("to_client", int64(n))
		done <- struct{}{}
	}()

	// 等待两个方向都完成
	for i := 0; i < 2; i++ {
		<-done
	}
	RecordProxyConnection("success")
	RecordProxyConnectionDuration(time.Since(startTime).Seconds())
}

// ioCopy 双向数据复制（带超时，使用缓冲池）
func ioCopy(dst, src net.Conn) (int64, error) {
	bufPtr := bufferPool.Get().(*[]byte)
	buf := *bufPtr
	defer bufferPool.Put(bufPtr)

	var total int64
	for {
		src.SetReadDeadline(time.Now().Add(30 * time.Second))
		n, err := src.Read(buf)
		if err != nil {
			return total, err
		}
		total += int64(n)
		dst.SetWriteDeadline(time.Now().Add(30 * time.Second))
		if _, err := dst.Write(buf[:n]); err != nil {
			return total, err
		}
	}
}

// Stop 停止所有端口转发
func (f *Forwarder) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, listener := range f.listeners {
		listener.Close()
	}
	f.listeners = nil
	f.portForwards = nil
}
