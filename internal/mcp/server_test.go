package mcp

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerCreate(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.False(t, func() bool { server.mu.Lock(); defer server.mu.Unlock(); return server.running }())
}

func TestServerStartStop(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-start.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// 验证 socket 文件存在
	_, err = os.Stat(cfg.SocketPath)
	assert.NoError(t, err)

	// 验证监听器正在运行
	server.mu.Lock()
	running := server.running
	server.mu.Unlock()
	assert.True(t, running)

	// 停止
	err = server.Stop()
	assert.NoError(t, err)

	// 验证已停止
	server.mu.Lock()
	running = server.running
	server.mu.Unlock()
	assert.False(t, running)
}

func TestServerStartAlreadyRunning(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-already.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// 再次启动应该失败
	err = server.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// 清理
	server.Stop()
}

func TestServerStopNotRunning(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-notrunning.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)

	// 停止未运行的服务器应该返回 nil
	err = server.Stop()
	assert.NoError(t, err)
}

func TestDefaultSocketPath(t *testing.T) {
	path := defaultSocketPath("my-agent")
	assert.Equal(t, "/tmp/keeper-my-agent.sock", path)
}

func TestSocketDir(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"/tmp/keeper.sock", "/tmp"},
		{"/var/run/keeper.sock", "/var/run"},
		{"keeper.sock", "/tmp"},
		{"/a/b/c.sock", "/a/b"},
	}

	for _, tc := range cases {
		result := socketDir(tc.input)
		assert.Equal(t, tc.expected, result, "input: %s", tc.input)
	}
}

func TestHandleInitialize(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-init.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)

	req := Request{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	}

	resp := server.handleRequest(context.Background(), req)
	assert.NotNil(t, resp)
	assert.Equal(t, 1, resp.ID)
	assert.NotNil(t, resp.Result)
	assert.Equal(t, "2024-11-05", resp.Result["protocolVersion"])
}

func TestHandleToolsList(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-tools.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)

	req := Request{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}

	resp := server.handleRequest(context.Background(), req)
	assert.NotNil(t, resp)
	assert.Equal(t, 2, resp.ID)
	assert.NotNil(t, resp.Result)
	tools, ok := resp.Result["tools"].([]Tool)
	assert.True(t, ok)
	assert.NotEmpty(t, tools)
}

func TestHandleToolCall(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-call.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)

	req := Request{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "keeper.create",
			"arguments": map[string]interface{}{"name": "test-agent"},
		},
	}

	resp := server.handleRequest(context.Background(), req)
	assert.NotNil(t, resp)
	assert.Equal(t, 3, resp.ID)
	// 由于没有实际调用 keeper 命令，会返回错误
	assert.NotNil(t, resp.Error)
}

func TestHandleShutdown(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-shutdown.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	req := Request{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "shutdown",
	}

	resp := server.handleRequest(context.Background(), req)
	assert.NotNil(t, resp)
	assert.Equal(t, 4, resp.ID)
	assert.NotNil(t, resp.Result)

	// 等待服务器停止
	time.Sleep(100 * time.Millisecond)

	server.mu.Lock()
	running := server.running
	server.mu.Unlock()
	assert.False(t, running)
}

func TestHandleUnknownMethod(t *testing.T) {
	cfg := ServerConfig{
		SocketPath: "/tmp/test-keeper-unknown.sock",
		AgentName:  "test",
	}

	server, err := NewServer(cfg, nil)
	require.NoError(t, err)

	req := Request{
		JSONRPC: "2.0",
		ID:      5,
		Method:  "unknown.method",
	}

	resp := server.handleRequest(context.Background(), req)
	assert.NotNil(t, resp)
	assert.Equal(t, 5, resp.ID)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32601, resp.Error.Code)
}

func TestMapMCPToKeeper(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]interface{}
		expected string
		argCount int
		wantErr  bool
	}{
		{
			name:     "keeper.create",
			args:     map[string]interface{}{"name": "agent1"},
			expected: "create",
			argCount: 1,
		},
		{
			name:     "keeper.start",
			args:     map[string]interface{}{"name": "agent1"},
			expected: "start",
			argCount: 1,
		},
		{
			name:     "keeper.stop",
			args:     map[string]interface{}{"name": "agent1"},
			expected: "stop",
			argCount: 1,
		},
		{
			name:     "keeper.list",
			args:     map[string]interface{}{},
			expected: "list",
			argCount: 0,
		},
		{
			name:     "keeper.inspect",
			args:     map[string]interface{}{"name": "agent1", "verbose": true},
			expected: "inspect",
			argCount: 2,
		},
		{
			name:     "keeper.fork",
			args:     map[string]interface{}{"source": "src", "target": "dst"},
			expected: "fork",
			argCount: 2,
		},
		{
			name:     "keeper.cp",
			args:     map[string]interface{}{"source": "/a", "destination": "b:/c", "recursive": true},
			expected: "cp",
			argCount: 3,
		},
		{
			name:     "keeper.cp",
			args:     map[string]interface{}{"source": "/a", "destination": "b:/c"},
			expected: "cp",
			argCount: 3, // recursive 为空字符串 ""
		},
		{
			name:    "keeper.create missing name",
			args:    map[string]interface{}{},
			wantErr: true,
		},
		{
			name:    "unknown tool",
			args:    map[string]interface{}{},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		cmd, args, err := mapMCPToKeeper(tc.name, tc.args)
		if tc.wantErr {
			assert.Error(t, err, "case: %s", tc.name)
		} else {
			assert.NoError(t, err, "case: %s", tc.name)
			assert.Equal(t, tc.expected, cmd, "case: %s", tc.name)
			if tc.argCount > 0 {
				assert.NotEmpty(t, args, "case: %s", tc.name)
			}
			assert.Len(t, args, tc.argCount, "case: %s", tc.name)
		}
	}
}
