package mcp

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
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

	// 发送初始化请求（服务器会拒绝连接并关闭）
	initReq := Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
		Params:  map[string]interface{}{},
	}
	// 注意：Encode 可能成功（数据写入缓冲区），但 Decode 会失败（连接被关闭）
	err = encoder.Encode(initReq)
	if err == nil {
		// 如果 Encode 成功，尝试读取响应应该失败
		var initResp Response
		err = decoder.Decode(&initResp)
	}
	// 连接被拒绝，最终应该返回错误
	assert.Error(t, err)
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

// TestMCPEndToEndToolExecution 测试 MCP Server 实际工具执行（调用 keeper CLI）
func TestMCPEndToEndToolExecution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-mcp-exec-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 创建 KEEPER_HOME 目录和配置文件
	keeperHome := filepath.Join(tmpDir, "keeper-home")
	require.NoError(t, os.MkdirAll(keeperHome, 0755))
	configFile := filepath.Join(keeperHome, "config.json")
	require.NoError(t, os.WriteFile(configFile, []byte(`{"log_level":"info"}`), 0644))

	// 设置环境变量
	os.Setenv("KEEPER_HOME", keeperHome)
	defer os.Unsetenv("KEEPER_HOME")

	// 创建 storage（供 MCP Server 内部使用）
	store, err := storage.NewStore(tmpDir)
	require.NoError(t, err)

	// 将项目 bin 目录加入 PATH，以便 findKeeperBinary 能找到 keeper 二进制
	projectBin := filepath.Join(getProjectRoot(), "bin")
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", projectBin+string(os.PathListSeparator)+originalPath)
	defer os.Setenv("PATH", originalPath)

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

	// 辅助函数：发送请求并接收响应
	sendRequest := func(req Request) Response {
		err := encoder.Encode(req)
		require.NoError(t, err)
		var resp Response
		err = decoder.Decode(&resp)
		require.NoError(t, err)
		return resp
	}

	// 1. 初始化
	initResp := sendRequest(Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
		Params:  map[string]interface{}{},
	})
	assert.Nil(t, initResp.Error)

	// 2. 创建 Agent（通过 MCP 调用 keeper CLI）
	createResp := sendRequest(Request{
		JSONRPC: "2.0",
		ID:      float64(2),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "keeper.create",
			"arguments": map[string]interface{}{"name": "mcp-test-agent"},
		},
	})
	assert.Nil(t, createResp.Error)
	createResult := createResp.Result["content"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, createResult["text"], "created")

	// 3. 列出 Agent（通过 MCP 调用 keeper CLI）
	listResp := sendRequest(Request{
		JSONRPC: "2.0",
		ID:      float64(3),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "keeper.list",
			"arguments": map[string]interface{}{},
		},
	})
	assert.Nil(t, listResp.Error)
	listResult := listResp.Result["content"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, listResult["text"], "mcp-test-agent")

	// 4. 查看 Agent 详情（通过 MCP 调用 keeper CLI）
	inspectResp := sendRequest(Request{
		JSONRPC: "2.0",
		ID:      float64(4),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "keeper.inspect",
			"arguments": map[string]interface{}{"name": "mcp-test-agent"},
		},
	})
	assert.Nil(t, inspectResp.Error)
	inspectResult := inspectResp.Result["content"].([]interface{})[0].(map[string]interface{})
	assert.Contains(t, inspectResult["text"], "mcp-test-agent")
}

// TestMCPEndToEndToolExecutionWithRecursive 测试 MCP cp 工具递归复制
func TestMCPEndToEndToolExecutionWithRecursive(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-mcp-cp-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 创建 KEEPER_HOME 目录和配置文件
	keeperHome := filepath.Join(tmpDir, "keeper-home")
	require.NoError(t, os.MkdirAll(keeperHome, 0755))
	configFile := filepath.Join(keeperHome, "config.json")
	require.NoError(t, os.WriteFile(configFile, []byte(`{"log_level":"info"}`), 0644))

	// 设置环境变量
	os.Setenv("KEEPER_HOME", keeperHome)
	defer os.Unsetenv("KEEPER_HOME")

	// 创建 storage
	store, err := storage.NewStore(tmpDir)
	require.NoError(t, err)

	// 将项目 bin 目录加入 PATH，以便 findKeeperBinary 能找到 keeper 二进制
	projectBin := filepath.Join(getProjectRoot(), "bin")
	originalPath := os.Getenv("PATH")
	os.Setenv("PATH", projectBin+string(os.PathListSeparator)+originalPath)
	defer os.Setenv("PATH", originalPath)

	cfg := ServerConfig{
		SocketPath:  filepath.Join(tmpDir, "mcp.sock"),
		AgentName:   "cp-test-agent",
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

	// 辅助函数
	sendRequest := func(req Request) Response {
		err := encoder.Encode(req)
		require.NoError(t, err)
		var resp Response
		err = decoder.Decode(&resp)
		require.NoError(t, err)
		return resp
	}

	// 1. 初始化
	sendRequest(Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
		Params:  map[string]interface{}{},
	})

	// 2. 创建 Agent
	createResp := sendRequest(Request{
		JSONRPC: "2.0",
		ID:      float64(2),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "keeper.create",
			"arguments": map[string]interface{}{"name": "cp-mcp-agent"},
		},
	})
	assert.Nil(t, createResp.Error)

	// 3. 创建本地源文件
	localSrcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(localSrcDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(localSrcDir, "file.txt"), []byte("hello mcp"), 0644))

	// 4. 递归复制目录到 Agent workspace（通过 MCP）
	cpResp := sendRequest(Request{
		JSONRPC: "2.0",
		ID:      float64(3),
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      "keeper.cp",
			"arguments": map[string]interface{}{"source": localSrcDir, "destination": "cp-mcp-agent:/dest", "recursive": true},
		},
	})
	assert.Nil(t, cpResp.Error)
	// cp 命令成功时无输出，content 应为空文本
	cpContent := cpResp.Result["content"].([]interface{})
	assert.NotEmpty(t, cpContent)
}

// getProjectRoot 从当前文件路径推导项目根目录
func getProjectRoot() string {
	// 方法1: 使用 runtime.Caller
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		dir := filepath.Dir(filename)
		// 上三级: internal/mcp -> internal -> <root>
		for i := 0; i < 3; i++ {
			dir = filepath.Dir(dir)
		}
		// 验证 go.mod 存在
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
	}

	// 方法2: 使用当前工作目录
	wd, err := os.Getwd()
	if err == nil {
		// 向上查找 go.mod
		for {
			if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
				return wd
			}
			parent := filepath.Dir(wd)
			if parent == wd {
				break
			}
			wd = parent
		}
	}

	// fallback: 返回当前目录
	wd, _ = os.Getwd()
	return wd
}
