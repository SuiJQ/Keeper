package network

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"keeper/internal/log"
)

func TestForwarderCreate(t *testing.T) {
	logger := &testLogger{}
	f := NewForwarder(logger)
	require.NotNil(t, f)
}

func TestForwarderAddForward(t *testing.T) {
	f := NewForwarder(nil)

	// 添加不冲突的端口转发
	err := f.AddForward(&PortForward{Host: 8080, Container: 80})
	require.NoError(t, err)

	// 重复添加应该失败
	err = f.AddForward(&PortForward{Host: 8080, Container: 80})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already forwarded")
}

func TestForwarderStartMultipleForwards(t *testing.T) {
	f := NewForwarder(nil)

	// 添加多个端口转发
	err := f.AddForward(&PortForward{Host: 18081, Container: 8081})
	require.NoError(t, err)

	err = f.AddForward(&PortForward{Host: 18082, Container: 8082})
	require.NoError(t, err)

	// 启动转发
	err = f.Start()
	require.NoError(t, err)

	// 验证有两个监听器
	assert.Len(t, f.listeners, 2)

	// 停止转发
	f.Stop()
}

func TestForwarderStartFailure(t *testing.T) {
	f := NewForwarder(nil)

	// 添加一个可能被占用的端口（使用特权端口需要 root）
	// 这里使用一个随机高端口，并监听它以确保冲突
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// 添加冲突的端口转发应该在 AddForward 时就失败
	err = f.AddForward(&PortForward{Host: port, Container: 8080})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already in use")
}

func TestForwarderStopWithoutStart(t *testing.T) {
	f := NewForwarder(nil)

	// 停止未启动的转发器应该安全
	f.Stop()
}

func TestForwarderConcurrentAccess(t *testing.T) {
	f := NewForwarder(nil)

	// 并发添加端口转发
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(port int) {
			_ = f.AddForward(&PortForward{Host: port, Container: 8080})
			done <- true
		}(18090 + i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestBufferPool(t *testing.T) {
	// 测试缓冲池可以正常获取和放回
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	buf := *bufPtr
	assert.Len(t, buf, 32*1024)
}

func TestIoCopyTimeout(t *testing.T) {
	// 创建一个慢速读取的连接
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// 设置读取超时
	go func() {
		time.Sleep(100 * time.Millisecond)
		server.Write([]byte("test"))
	}()

	// 使用 bufferPool 进行复制
	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	// 设置超时
	client.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

	buf := *bufPtr
	_, err := client.Read(buf)
	assert.Error(t, err) // 应该超时
}

// testLogger 测试用日志记录器
type testLogger struct{}

func (l *testLogger) Debug(msg string, fields ...log.Field)     {}
func (l *testLogger) Info(msg string, fields ...log.Field)      {}
func (l *testLogger) Warn(msg string, fields ...log.Field)      {}
func (l *testLogger) Error(msg string, fields ...log.Field)     {}
func (l *testLogger) Fatal(msg string, fields ...log.Field)     {}
func (l *testLogger) WithFields(fields ...log.Field) log.Logger { return l }
func (l *testLogger) Sync() error                               { return nil }
func (l *testLogger) SetOutput(w io.Writer)                     {}

// TestNetworkMetricsRecording 测试网络指标记录
func TestNetworkMetricsRecording(t *testing.T) {
	RecordPortForward("success")
	RecordPortForward("error")
	RecordPortForwardDuration(0.5)

	RecordProxyConnection("success")
	RecordProxyConnection("error")
	RecordProxyConnectionDuration(1.2)

	RecordDataTransfer("upload", 1024)
	RecordDataTransfer("download", 2048)
}

// TestNetworkMetricsConcurrent 并发测试网络指标
func TestNetworkMetricsConcurrent(t *testing.T) {
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			RecordPortForward("success")
			RecordPortForwardDuration(0.1)
			RecordProxyConnection("success")
			RecordDataTransfer("upload", int64(id*100))
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestNetworkMetricsEdgeCases 测试边界情况
func TestNetworkMetricsEdgeCases(t *testing.T) {
	// 测试零值
	RecordPortForwardDuration(0)
	RecordProxyConnectionDuration(0)
	RecordDataTransfer("upload", 0)

	// 测试大值
	RecordDataTransfer("download", 999999999)
}
