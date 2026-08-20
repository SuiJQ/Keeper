package network

import (
	"io"
	"testing"

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
