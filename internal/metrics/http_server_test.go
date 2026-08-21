package metrics

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPServerStartStop 测试 HTTP 服务器启停
func TestHTTPServerStartStop(t *testing.T) {
	server := NewHTTPServer("127.0.0.1:0")
	require.NotNil(t, server)

	// 启动
	err := server.Start()
	require.NoError(t, err)
	assert.True(t, server.IsRunning())

	// 停止
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = server.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, server.IsRunning())
}

// TestHTTPServerStartTwice 测试重复启动
func TestHTTPServerStartTwice(t *testing.T) {
	server := NewHTTPServer("127.0.0.1:0")

	err := server.Start()
	require.NoError(t, err)

	// 再次启动应该静默成功
	err = server.Start()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = server.Stop(ctx)
	require.NoError(t, err)
}

// TestHTTPServerStopNotRunning 测试停止未运行的服务器
func TestHTTPServerStopNotRunning(t *testing.T) {
	server := NewHTTPServer("127.0.0.1:0")

	// 停止未运行的服务器应该静默成功
	ctx := context.Background()
	err := server.Stop(ctx)
	require.NoError(t, err)
}

// TestGetHTTPServer 测试全局 HTTP 服务器单例
func TestGetHTTPServer(t *testing.T) {
	// 重置全局实例（仅用于测试）
	// 注意：这里我们测试 GetHTTPServer 返回非 nil
	server := GetHTTPServer()
	assert.NotNil(t, server)
	assert.Equal(t, ":9090", server.Addr())
}

// TestStartMetricsServer 测试启动全局指标服务器
func TestStartMetricsServer(t *testing.T) {
	// 先停止可能存在的全局服务器
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	StopMetricsServer(ctx)
	cancel()

	err := StartMetricsServer()
	require.NoError(t, err)

	// 验证服务器运行
	server := GetHTTPServer()
	assert.True(t, server.IsRunning())

	// 清理
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	StopMetricsServer(ctx)
	cancel()
}

// TestMetricsEndpoint 测试 /metrics 端点返回有效数据
func TestMetricsEndpoint(t *testing.T) {
	// 先停止可能存在的全局服务器
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	StopMetricsServer(ctx)
	cancel()

	// 注册一些测试指标
	testCounter := RegisterCounter("test_counter_total", "Test counter", []string{"label1"})
	testCounter.Inc("value1")
	testGauge := RegisterGauge("test_gauge", "Test gauge", []string{"label2"})
	testGauge.Set(42, "value2")

	// 启动服务器
	err := StartMetricsServer()
	require.NoError(t, err)
	server := GetHTTPServer()
	assert.True(t, server.IsRunning())

	// 获取实际监听地址
	addr := server.Addr()
	require.NotEmpty(t, addr)

	// 等待服务器就绪
	time.Sleep(100 * time.Millisecond)

	// 创建 HTTP 客户端
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/metrics", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	// 验证响应状态
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "text/plain; version=0.0.4", resp.Header.Get("Content-Type"))

	// 清理
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	StopMetricsServer(ctx)
	cancel()
}
