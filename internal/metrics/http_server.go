// Package metrics 提供指标导出能力
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
)

// HTTPServer 指标 HTTP 服务器
type HTTPServer struct {
	addr    string
	server  *http.Server
	mu      sync.RWMutex
	running bool

	// healthCheck 健康检查函数
	healthCheck func() error
	readyCheck  func() error
}

// NewHTTPServer 创建指标 HTTP 服务器
func NewHTTPServer(addr string) *HTTPServer {
	return &HTTPServer{
		addr: addr,
	}
}

// Addr 返回服务器实际监听地址
func (s *HTTPServer) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.server != nil {
		if s.server.Addr != "" {
			return s.server.Addr
		}
	}
	return s.addr
}

// Start 启动 HTTP 服务器
func (s *HTTPServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprint(w, PrometheusFormat())
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.healthCheck != nil {
			if err := s.healthCheck(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, `{"status":"unhealthy","error":"%s"}`, err.Error())
				return
			}
		}
		fmt.Fprint(w, `{"status":"healthy"}`)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if s.readyCheck != nil {
			if err := s.readyCheck(); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, `{"status":"not ready","error":"%s"}`, err.Error())
				return
			}
		}
		fmt.Fprint(w, `{"status":"ready"}`)
	})

	s.server = &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	server := s.server
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 记录错误但不panic
			fmt.Printf("metrics server error: %v\n", err)
		}
	}()

	s.running = true
	return nil
}

// Stop 停止 HTTP 服务器
func (s *HTTPServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running || s.server == nil {
		return nil
	}

	s.running = false
	return s.server.Shutdown(ctx)
}

// IsRunning 检查服务器是否运行中
func (s *HTTPServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// SetHealthCheck 设置健康检查函数
func (s *HTTPServer) SetHealthCheck(check func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthCheck = check
}

// SetReadyCheck 设置就绪检查函数
func (s *HTTPServer) SetReadyCheck(check func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readyCheck = check
}

// 全局 HTTP 服务器
var (
	defaultHTTPServer *HTTPServer
	httpServerOnce    sync.Once
)

// GetHTTPServer 获取全局 HTTP 服务器
func GetHTTPServer() *HTTPServer {
	httpServerOnce.Do(func() {
		defaultHTTPServer = NewHTTPServer(":9090")
	})
	return defaultHTTPServer
}

// StartMetricsServer 启动全局指标服务器
func StartMetricsServer() error {
	return GetHTTPServer().Start()
}

// StopMetricsServer 停止全局指标服务器
func StopMetricsServer(ctx context.Context) error {
	return GetHTTPServer().Stop(ctx)
}
