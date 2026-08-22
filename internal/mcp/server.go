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
	"syscall"
	"time"

	"keeper/internal/errors"
	"keeper/internal/log"
	"keeper/internal/storage"
)

// Server MCP Server 实现
type Server struct {
	mu          sync.Mutex
	running     bool
	listener    net.Listener
	socketPath  string
	store       storage.Store
	logger      log.Logger
	agentName   string
	allowedUIDs map[uint32]struct{}
	allowedGIDs map[uint32]struct{}
}

// ServerConfig MCP Server 配置
type ServerConfig struct {
	SocketPath  string
	Store       storage.Store
	AgentName   string
	AllowedUIDs []uint32
	AllowedGIDs []uint32
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

	allowedUIDs := make(map[uint32]struct{}, len(cfg.AllowedUIDs))
	for _, uid := range cfg.AllowedUIDs {
		allowedUIDs[uid] = struct{}{}
	}
	allowedGIDs := make(map[uint32]struct{}, len(cfg.AllowedGIDs))
	for _, gid := range cfg.AllowedGIDs {
		allowedGIDs[gid] = struct{}{}
	}

	return &Server{
		socketPath:  cfg.SocketPath,
		store:       store,
		logger:      logger,
		agentName:   cfg.AgentName,
		allowedUIDs: allowedUIDs,
		allowedGIDs: allowedGIDs,
	}, nil
}

// Start 启动 MCP Server
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		RecordMCPConnection("error")
		return fmt.Errorf("MCP server already running")
	}

	// 创建 Unix socket
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		RecordMCPConnection("error")
		return fmt.Errorf("create socket listener: %w", err)
	}
	s.listener = listener
	s.running = true

	// 设置 socket 权限为 0600（仅属主可访问）
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		_ = s.Stop()
		RecordMCPConnection("error")
		return fmt.Errorf("set socket permissions: %w", err)
	}

	s.logger.Info("MCP server started", log.Field{Key: "socket", Value: s.socketPath})

	RecordMCPConnection("success")

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

	RecordMCPConnection("stopped")

	return nil
}

// IsRunning 检查 MCP Server 是否运行中
func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
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

	// SO_PEERCRED 鉴权：获取客户端 UID
	cred, err := getPeerCredentials(conn)
	if err != nil {
		s.logger.Error("failed to get peer credentials", log.Field{Key: "error", Value: err.Error()})
		RecordMCPConnection("auth_error")
		return
	}

	// 记录客户端 UID
	s.logger.Info("client connected",
		log.Field{Key: "pid", Value: cred.Pid},
		log.Field{Key: "uid", Value: cred.Uid},
		log.Field{Key: "gid", Value: cred.Gid})

	// 可配置的 UID/GID 白名单校验
	if err := s.authorize(cred); err != nil {
		s.logger.Warn("rejecting connection", log.Field{Key: "uid", Value: cred.Uid}, log.Field{Key: "gid", Value: cred.Gid}, log.Field{Key: "error", Value: err.Error()})
		RecordMCPConnection("auth_error")
		return
	}

	RecordMCPConnection("accepted")

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

// getPeerCredentials 获取 Unix socket 客户端凭证
func getPeerCredentials(conn net.Conn) (*syscall.Ucred, error) {
	// 仅支持 Unix socket
	if _, ok := conn.LocalAddr().(*net.UnixAddr); !ok {
		return nil, fmt.Errorf("not a unix socket")
	}

	// 获取底层文件描述符
	file, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("not a unix conn")
	}

	// 通过 File() 获取 os.File 以访问文件描述符
	// 注意：不要关闭返回的 os.File，它会自动由 net.UnixConn 管理
	osFile, err := file.File()
	if err != nil {
		return nil, fmt.Errorf("get file from conn: %w", err)
	}
	defer osFile.Close()

	// 使用 SO_PEERCRED 获取凭证
	cred, err := syscall.GetsockoptUcred(int(osFile.Fd()), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		return nil, fmt.Errorf("getsockopt SO_PEERCRED: %w", err)
	}

	return cred, nil
}

// UpdateAllowedUIDs 更新允许的 UID 白名单
func (s *Server) UpdateAllowedUIDs(uids []uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.allowedUIDs = make(map[uint32]struct{}, len(uids))
	for _, uid := range uids {
		s.allowedUIDs[uid] = struct{}{}
	}

	s.logger.Info("MCP allowed UIDs updated", log.Field{Key: "uids", Value: uids})
}

// UpdateAllowedGIDs 更新允许的 GID 白名单
func (s *Server) UpdateAllowedGIDs(gids []uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.allowedGIDs = make(map[uint32]struct{}, len(gids))
	for _, gid := range gids {
		s.allowedGIDs[gid] = struct{}{}
	}

	s.logger.Info("MCP allowed GIDs updated", log.Field{Key: "gids", Value: gids})
}

// authorize 校验客户端 UID/GID 白名单
func (s *Server) authorize(cred *syscall.Ucred) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.allowedUIDs) > 0 {
		if _, ok := s.allowedUIDs[cred.Uid]; !ok {
			return fmt.Errorf("uid %d not allowed", cred.Uid)
		}
	}

	if len(s.allowedGIDs) > 0 {
		if _, ok := s.allowedGIDs[cred.Gid]; !ok {
			return fmt.Errorf("gid %d not allowed", cred.Gid)
		}
	}

	return nil
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
			ID:    req.ID,
			Error: &Error{Code: -32602, Message: "invalid params: missing tool name"},
		}
	}

	arguments, ok := req.Params["arguments"].(map[string]interface{})
	if !ok {
		arguments = make(map[string]interface{})
	}

	startTime := time.Now()
	result, err := s.executeTool(ctx, toolName, arguments)
	duration := time.Since(startTime).Seconds()

	if err != nil {
		RecordMCPToolCall(toolName, "error")
		RecordMCPToolCallDuration(toolName, duration)
		return Response{
			ID:    req.ID,
			Error: &Error{Code: -32603, Message: err.Error()},
		}
	}

	RecordMCPToolCall(toolName, "success")
	RecordMCPToolCallDuration(toolName, duration)

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

	// 查找 keeper 二进制文件
	keeperBin, err := findKeeperBinary()
	if err != nil {
		return "", fmt.Errorf("find keeper binary: %w", err)
	}

	// 构建命令参数
	cmdArgs := append([]string{keeperCmd}, keeperArgs...)

	s.logger.Info("invoking keeper",
		log.Field{Key: "bin", Value: keeperBin},
		log.Field{Key: "args", Value: cmdArgs})

	cmd := exec.CommandContext(ctx, keeperBin, cmdArgs...)

	// 如果 context 没有超时，设置默认超时（30 秒）
	if _, hasTimeout := ctx.Deadline(); !hasTimeout {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cmd = exec.CommandContext(ctx, keeperBin, cmdArgs...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		// 区分超时错误和其他错误
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.NewKeeperError(errors.ErrCodeProcess,
				fmt.Sprintf("keeper %s timed out after 30s", keeperCmd), err)
		}
		return "", fmt.Errorf("keeper %s failed: %w\n%s", keeperCmd, err, string(output))
	}

	return string(output), nil
}

// findKeeperBinary 查找 keeper 二进制文件
func findKeeperBinary() (string, error) {
	// 1. 检查 PATH
	if path, err := exec.LookPath("keeper"); err == nil {
		return path, nil
	}

	// 2. 检查常见位置
	candidates := []string{
		filepath.Join("/usr/local/bin", "keeper"),
		filepath.Join("/usr/bin", "keeper"),
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "keeper"),
		filepath.Join(".", "bin", "keeper"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("keeper binary not found in PATH or common locations")
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
