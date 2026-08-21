package mcp

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"keeper/internal/log"
	"keeper/internal/storage"
)

// TestMCPEndToEndFullFlow 测试 MCP Server 端到端完整流程
func TestMCPEndToEndFullFlow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-mcp-e2e-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 创建 storage
	store, err := storage.NewStore(tmpDir)
	require.NoError(t, err)

	// 创建 MCP Server
	cfg := ServerConfig{
		SocketPath:  filepath.Join(tmpDir, "mcp.sock"),
		AgentName:   "test-agent",
		Store:       store,
		AllowedUIDs: []uint32{0, uint32(os.Getuid())},
		AllowedGIDs: []uint32{0, uint32(os.Getgid())},
	}

	server, err := NewServer(cfg, log.New(os.Stderr))
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 连接 Unix Socket
	conn, err := net.Dial("unix", cfg.SocketPath)
	require.NoError(t, err)
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// 1. 初始化请求
	initReq := Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
		Params:  map[string]interface{}{},
	}
	err = encoder.Encode(initReq)
	require.NoError(t, err)

	var initResp Response
	err = decoder.Decode(&initResp)
	require.NoError(t, err)
	assert.Equal(t, float64(1), initResp.ID)
	assert.Nil(t, initResp.Error)
	assert.NotNil(t, initResp.Result)
	assert.Equal(t, "2024-11-05", initResp.Result["protocolVersion"])
	assert.Equal(t, "keeper-mcp", initResp.Result["serverInfo"].(map[string]interface{})["name"])

	// 2. 列出工具
	listReq := Request{
		JSONRPC: "2.0",
		ID:      float64(2),
		Method:  "tools/list",
		Params:  map[string]interface{}{},
	}
	err = encoder.Encode(listReq)
	require.NoError(t, err)

	var listResp Response
	err = decoder.Decode(&listResp)
	require.NoError(t, err)
	assert.Equal(t, float64(2), listResp.ID)
	assert.Nil(t, listResp.Error)
	assert.NotNil(t, listResp.Result)
	assert.NotEmpty(t, listResp.Result["tools"])

	// 3. 调用不存在的工具（应该返回错误）
	callReq := Request{
		JSONRPC: "2.0",
		ID:      float64(3),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "nonexistent.tool",
			"arguments": map[string]interface{}{},
		},
	}
	err = encoder.Encode(callReq)
	require.NoError(t, err)

	var callResp Response
	err = decoder.Decode(&callResp)
	require.NoError(t, err)
	assert.Equal(t, float64(3), callResp.ID)
	assert.NotNil(t, callResp.Error)
	assert.Equal(t, -32603, callResp.Error.Code)
}

// TestMCPEndToEndAuth 测试 MCP Server 授权端到端
func TestMCPEndToEndAuth(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-mcp-auth-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStore(tmpDir)
	require.NoError(t, err)

	// 创建 MCP Server，配置允许的 UID/GID
	cfg := ServerConfig{
		SocketPath:  filepath.Join(tmpDir, "mcp.sock"),
		AgentName:   "test-agent",
		Store:       store,
		AllowedUIDs: []uint32{9999}, // 只允许特定 UID
		AllowedGIDs: []uint32{9999}, // 只允许特定 GID
	}

	server, err := NewServer(cfg, log.New(os.Stderr))
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	// 连接 Unix Socket（当前进程 UID/GID 不在白名单中）
	conn, err := net.Dial("unix", cfg.SocketPath)
	require.NoError(t, err)
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// 发送初始化请求（应该被拒绝）
	initReq := Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
		Params:  map[string]interface{}{},
	}
	err = encoder.Encode(initReq)
	require.NoError(t, err)

	// 读取响应（连接被拒绝，会收到错误响应或连接重置）
	var initResp Response
	err = decoder.Decode(&initResp)
	if err != nil {
		// 连接被重置，视为授权成功
		return
	}

	assert.NotNil(t, initResp.Error)
	assert.Contains(t, initResp.Error.Message, "not allowed")
}

// TestMCPEndToEndShutdown 测试 MCP Server 关闭流程
func TestMCPEndToEndShutdown(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-mcp-shutdown-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStore(tmpDir)
	require.NoError(t, err)

	cfg := ServerConfig{
		SocketPath:  filepath.Join(tmpDir, "mcp.sock"),
		AgentName:   "test-agent",
		Store:       store,
		AllowedUIDs: []uint32{0, uint32(os.Getuid())},
		AllowedGIDs: []uint32{0, uint32(os.Getgid())},
	}

	server, err := NewServer(cfg, log.New(os.Stderr))
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// 连接 Unix Socket
	conn, err := net.Dial("unix", cfg.SocketPath)
	require.NoError(t, err)

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// 发送初始化请求
	initReq := Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
		Params:  map[string]interface{}{},
	}
	err = encoder.Encode(initReq)
	require.NoError(t, err)

	var initResp Response
	err = decoder.Decode(&initResp)
	require.NoError(t, err)
	assert.Nil(t, initResp.Error)

	// 发送关闭请求
	shutdownReq := Request{
		JSONRPC: "2.0",
		ID:      float64(2),
		Method:  "shutdown",
		Params:  map[string]interface{}{},
	}
	err = encoder.Encode(shutdownReq)
	require.NoError(t, err)

	var shutdownResp Response
	err = decoder.Decode(&shutdownResp)
	require.NoError(t, err)
	assert.Equal(t, float64(2), shutdownResp.ID)
	assert.Nil(t, shutdownResp.Error)

	// 等待服务器关闭
	time.Sleep(100 * time.Millisecond)

	// 验证服务器已停止（通过检查 listener 是否关闭）
	server.mu.Lock()
	running := server.running
	server.mu.Unlock()
	assert.False(t, running)

	conn.Close()
}

// TestMCPEndToEndInvalidMethod 测试 MCP Server 无效方法
func TestMCPEndToEndInvalidMethod(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-mcp-invalid-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStore(tmpDir)
	require.NoError(t, err)

	cfg := ServerConfig{
		SocketPath:  filepath.Join(tmpDir, "mcp.sock"),
		AgentName:   "test-agent",
		Store:       store,
		AllowedUIDs: []uint32{0, uint32(os.Getuid())},
		AllowedGIDs: []uint32{0, uint32(os.Getgid())},
	}

	server, err := NewServer(cfg, log.New(os.Stderr))
	require.NoError(t, err)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer server.Stop()

	conn, err := net.Dial("unix", cfg.SocketPath)
	require.NoError(t, err)
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	// 发送无效方法请求
	req := Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "invalid.method",
		Params:  map[string]interface{}{},
	}
	err = encoder.Encode(req)
	require.NoError(t, err)

	var resp Response
	err = decoder.Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, float64(1), resp.ID)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32601, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "method not found")
}
