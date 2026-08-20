package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"keeper/internal/log"
	"keeper/internal/storage"
)

// Server MCP Server 实现
type Server struct {
	mu         sync.Mutex
	running    bool
	listener   net.Listener
	socketPath string
	store      storage.Store
	logger     log.Logger
	agentName  string
}

// ServerConfig MCP Server 配置
type ServerConfig struct {
	SocketPath string
	Store      storage.Store
	AgentName  string
}

// NewServer 创建 MCP Server 实例
func NewServer(cfg ServerConfig, logger log.Logger) (*Server, error) {
	if cfg.SocketPath == "" {
		cfg.SocketPath = defaultSocketPath(cfg.AgentName)
	}

	if logger == nil {
		logger = log.Global()
	}

	// 确保 socket 目录存在
	socketDir := socketDir(cfg.SocketPath)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}

	// 删除已有 socket 文件
	if _, err := os.Stat(cfg.SocketPath); err == nil {
		if err := os.Remove(cfg.SocketPath); err != nil {
			return nil, fmt.Errorf("remove old socket: %w", err)
		}
	}

	// 创建存储实例（使用临时目录）
	store, err := storage.NewStore("/tmp/keeper-mcp-" + cfg.AgentName)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	return &Server{
		socketPath: cfg.SocketPath,
		store:      store,
		logger:     logger,
		agentName:  cfg.AgentName,
	}, nil
}

// Start 启动 MCP Server
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("MCP server already running")
	}

	// 创建 Unix socket
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("create socket listener: %w", err)
	}
	s.listener = listener
	s.running = true

	// 设置 socket 权限为 0600（仅属主可访问）
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		s.Stop()
		return fmt.Errorf("set socket permissions: %w", err)
	}

	s.logger.Info("MCP server started", log.Field{Key: "socket", Value: s.socketPath})

	go s.acceptLoop(ctx)

	return nil
}

// Stop 停止 MCP Server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.running = false

	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			s.logger.Error("error closing MCP listener", log.Field{Key: "error", Value: err.Error()})
		}
	}

	// 清理 socket 文件
	if _, err := os.Stat(s.socketPath); err == nil {
		if err := os.Remove(s.socketPath); err != nil {
			s.logger.Error("error removing socket file", log.Field{Key: "error", Value: err.Error()})
		}
	}

	s.logger.Info("MCP server stopped")
	return nil
}

// acceptLoop 接受连接循环
func (s *Server) acceptLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				if s.running {
					s.logger.Error("accept error", log.Field{Key: "error", Value: err.Error()})
				}
				continue
			}
			go s.handleConnection(ctx, conn)
		}
	}
}

// handleConnection 处理单个连接
func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			s.logger.Debug("decode error", log.Field{Key: "error", Value: err.Error()})
			return
		}

		resp := s.handleRequest(ctx, req)
		if err := encoder.Encode(resp); err != nil {
			s.logger.Error("encode error", log.Field{Key: "error", Value: err.Error()})
			return
		}
	}
}

// handleRequest 处理单个请求
func (s *Server) handleRequest(ctx context.Context, req Request) Response {
	s.logger.Debug("handling MCP request",
		log.Field{Key: "method", Value: req.Method},
		log.Field{Key: "id", Value: req.ID})

	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolCall(ctx, req)
	case "shutdown":
		return s.handleShutdown(req)
	default:
		return Response{
			ID:    req.ID,
			Error: &Error{Code: -32601, Message: "method not found"},
		}
	}
}

// handleInitialize 处理初始化请求
func (s *Server) handleInitialize(req Request) Response {
	return Response{
		ID: req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "keeper-mcp",
				"version": "0.1.0",
			},
		},
	}
}

// handleToolsList 处理工具列表请求
func (s *Server) handleToolsList(req Request) Response {
	tools := []Tool{
		{
			Name:        "keeper.create",
			Description: "创建新 Agent",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Agent 名称",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "keeper.start",
			Description: "启动 Agent",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Agent 名称",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "keeper.stop",
			Description: "停止 Agent",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Agent 名称",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "keeper.list",
			Description: "列出所有 Agent",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "keeper.inspect",
			Description: "查看 Agent 详细信息",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Agent 名称",
					},
					"verbose": map[string]interface{}{
						"type":        "boolean",
						"description": "显示详细设备信息",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "keeper.fork",
			Description: "克隆 Agent",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source": map[string]interface{}{
						"type":        "string",
						"description": "源 Agent 名称",
					},
					"target": map[string]interface{}{
						"type":        "string",
						"description": "目标 Agent 名称",
					},
				},
				"required": []string{"source", "target"},
			},
		},
		{
			Name:        "keeper.cp",
			Description: "在宿主机和 Agent workspace 之间复制文件",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"source": map[string]interface{}{
						"type":        "string",
						"description": "源路径（本地路径或 agent:path）",
					},
					"destination": map[string]interface{}{
						"type":        "string",
						"description": "目标路径（本地路径或 agent:path）",
					},
					"recursive": map[string]interface{}{
						"type":        "boolean",
						"description": "递归复制目录",
					},
				},
				"required": []string{"source", "destination"},
			},
		},
	}

	return Response{
		ID: req.ID,
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

// handleToolCall 处理工具调用请求
func (s *Server) handleToolCall(ctx context.Context, req Request) Response {
	toolName, ok := req.Params["name"].(string)
	if !ok {
		return Response{
			ID: req.ID,
			Error: &Error{Code: -32602, Message: "invalid params: missing tool name"},
		}
	}

	arguments, ok := req.Params["arguments"].(map[string]interface{})
	if !ok {
		arguments = make(map[string]interface{})
	}

	// TODO: 实际调用对应的 keeper 命令
	result, err := s.executeTool(ctx, toolName, arguments)
	if err != nil {
		return Response{
			ID: req.ID,
			Error: &Error{Code: -32603, Message: err.Error()},
		}
	}

	return Response{
		ID: req.ID,
		Result: map[string]interface{}{
			"content": []Content{
				{
					Type: "text",
					Text: result,
				},
			},
		},
	}
}

// handleShutdown 处理关闭请求
func (s *Server) handleShutdown(req Request) Response {
	go func() {
		if err := s.Stop(); err != nil {
			s.logger.Error("error stopping MCP server", log.Field{Key: "error", Value: err.Error()})
		}
	}()
	return Response{
		ID: req.ID,
		Result: map[string]interface{}{
			"message": "shutting down",
		},
	}
}

// executeTool 执行工具（调用 keeper CLI）
func (s *Server) executeTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	s.logger.Info("executing MCP tool",
		log.Field{Key: "tool", Value: name},
		log.Field{Key: "args", Value: fmt.Sprintf("%v", args)})

	// 映射 MCP 工具名到 keeper 命令
	keeperCmd, keeperArgs, err := mapMCPToKeeper(name, args)
	if err != nil {
		return "", fmt.Errorf("map tool: %w", err)
	}

	// 构建 keeper 二进制路径
	keeperBin := filepath.Join("/tmp", "keeper-mcp-bin", "keeper")
	if _, err := os.Stat(keeperBin); err != nil {
		// fallback：尝试当前目录的 bin/keeper
		keeperBin = filepath.Join(".", "bin", "keeper")
	}

	// 构建命令参数
	cmdArgs := append([]string{keeperCmd}, keeperArgs...)

	s.logger.Info("invoking keeper",
		log.Field{Key: "bin", Value: keeperBin},
		log.Field{Key: "args", Value: cmdArgs})

	cmd := exec.CommandContext(ctx, keeperBin, cmdArgs...)
	cmd.Dir = "/run/csi/mount-root/nas/4079184d856ecc166ed19d4887083405/workspaces/QwenPaw_QA_Agent_0.2/mirage"

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("keeper %s failed: %w\n%s", keeperCmd, err, string(output))
	}

	return string(output), nil
}

// mapMCPToKeeper 将 MCP 工具调用映射为 keeper CLI 参数
func mapMCPToKeeper(name string, args map[string]interface{}) (string, []string, error) {
	switch name {
	case "keeper.create":
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return "", nil, fmt.Errorf("missing required arg: name")
		}
		return "create", []string{name}, nil

	case "keeper.start":
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return "", nil, fmt.Errorf("missing required arg: name")
		}
		return "start", []string{name}, nil

	case "keeper.stop":
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return "", nil, fmt.Errorf("missing required arg: name")
		}
		return "stop", []string{name}, nil

	case "keeper.list":
		return "list", nil, nil

	case "keeper.inspect":
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return "", nil, fmt.Errorf("missing required arg: name")
		}
		verbose := ""
		if v, ok := args["verbose"].(bool); ok && v {
			verbose = "--verbose"
		}
		return "inspect", []string{name, verbose}, nil

	case "keeper.fork":
		source, ok1 := args["source"].(string)
		target, ok2 := args["target"].(string)
		if !ok1 || !ok2 || source == "" || target == "" {
			return "", nil, fmt.Errorf("missing required args: source, target")
		}
		return "fork", []string{source, target}, nil

	case "keeper.cp":
		src, ok1 := args["source"].(string)
		dst, ok2 := args["destination"].(string)
		if !ok1 || !ok2 || src == "" || dst == "" {
			return "", nil, fmt.Errorf("missing required args: source, destination")
		}
		recursive := ""
		if v, ok := args["recursive"].(bool); ok && v {
			recursive = "-r"
		}
		return "cp", []string{recursive, src, dst}, nil

	default:
		return "", nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// Socket 路径管理

func defaultSocketPath(agentName string) string {
	return fmt.Sprintf("/tmp/keeper-%s.sock", agentName)
}

func socketDir(socketPath string) string {
	// 获取 socket 文件的目录
	lastSep := -1
	for i := len(socketPath) - 1; i >= 0; i-- {
		if socketPath[i] == '/' {
			lastSep = i
			break
		}
	}
	if lastSep < 0 {
		return "/tmp"
	}
	return socketPath[:lastSep]
}

// MCP 协议类型定义

// Request MCP 请求
type Request struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// Response MCP 响应
type Response struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *Error                 `json:"error,omitempty"`
}

// Error MCP 错误
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool MCP 工具定义
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Content 响应内容
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
