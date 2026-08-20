package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"keeper/internal/log"
	"keeper/internal/storage"
	"keeper/pkg/config"
)

const (
	appName     = "keeper"
	version     = "0.1.0-dev"
	defaultHome = ".local/share/keeper"
)

var (
	globalLogger = log.New(os.Stdout)
)

func main() {
	if err := run(); err != nil {
		globalLogger.Error("application failed", log.Field{Key: "error", Value: err.Error()})
		os.Exit(1)
	}
}

func run() error {
	// 解析命令行参数
	if len(os.Args) < 2 {
		printUsage()
		return nil
	}

	command := os.Args[1]
	args := os.Args[2:]

	// 加载配置
	home := getHomeDir()
	cfg, err := config.Load(home)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 初始化日志
	logger := globalLogger.WithFields(log.Field{Key: "home", Value: home})
	log.SetGlobal(logger)

	// 路由命令
	switch command {
	case "create":
		return createAgent(cfg, args)
	case "start":
		return startAgent(cfg, args)
	case "stop":
		return stopAgent(cfg, args)
	case "status":
		return statusAgent(cfg, args)
	case "list":
		return listAgents(cfg, args)
	case "inspect":
		return inspectAgent(cfg, args)
	case "fork":
		return forkAgent(cfg, args)
	case "cp":
		return copyFile(cfg, args)
	case "destroy":
		return destroyAgent(cfg, args)
	case "recover":
		return recoverAgent(cfg, args)
	case "snapshot":
		return snapshotAgent(cfg, args)
	case "rollback":
		return rollbackAgent(cfg, args)
	case "version":
		fmt.Printf("keeper %s\n", version)
		return nil
	case "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command: %s (try 'help')", command)
	}
}

func createAgent(cfg *config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keeper create <name>")
	}

	name := args[0]
	if !isValidName(name) {
		return fmt.Errorf("invalid agent name: %s (must match ^[a-zA-Z0-9_-]{1,32}$)", name)
	}

	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("creating agent")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	meta, err := store.CreateAgent(context.Background(), name)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	logger.Info("agent created", log.Field{Key: "state", Value: meta.State})

	fmt.Printf("Agent '%s' created (state: %s)\n", name, meta.State)
	return nil
}

func startAgent(cfg *config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keeper start <name>")
	}

	name := args[0]
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("starting agent")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	// 加载 agent 元数据
	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// 检查当前状态
	if meta.State == "running" {
		logger.Info("agent already running")
		fmt.Printf("Agent '%s' is already running\n", name)
		return nil
	}

	// 从 created 或 stopped 状态启动
	if meta.State == "created" || meta.State == "stopped" {
		meta.State = "running"
		meta.StartedAt = time.Now().UTC().Format(time.RFC3339)
		meta.PID = os.Getpid() // 简化：使用当前进程 PID
		meta.PGID = fmt.Sprintf("%d", os.Getpid())
		if err := store.UpdateAgent(context.Background(), meta); err != nil {
			return fmt.Errorf("update agent state: %w", err)
		}
		logger.Info("agent started", log.Field{Key: "state", Value: meta.State})
		fmt.Printf("Agent '%s' started\n", name)
		return nil
	}

	// 其他状态不允许直接启动
	return fmt.Errorf("cannot start agent in state: %s", meta.State)
}

func stopAgent(cfg *config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keeper stop <name>")
	}

	name := args[0]
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("stopping agent")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	// 加载 agent 元数据
	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// 检查当前状态
	if meta.State == "stopped" {
		logger.Info("agent already stopped")
		fmt.Printf("Agent '%s' is already stopped\n", name)
		return nil
	}

	if meta.State != "running" {
		return fmt.Errorf("cannot stop agent in state: %s", meta.State)
	}

	// 停止 agent
	meta.State = "stopped"
	meta.StoppedAt = time.Now().UTC().Format(time.RFC3339)
	meta.PID = 0
	meta.PGID = ""

	if err := store.UpdateAgent(context.Background(), meta); err != nil {
		return fmt.Errorf("update agent state: %w", err)
	}

	logger.Info("agent stopped", log.Field{Key: "state", Value: meta.State})
	fmt.Printf("Agent '%s' stopped\n", name)
	return nil
}

func statusAgent(cfg *config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keeper status <name>")
	}

	name := args[0]
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("querying agent status")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	// 加载 agent 元数据
	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// 输出状态
	fmt.Printf("Agent '%s': %s\n", name, meta.State)
	if meta.State == "running" && meta.PID > 0 {
		fmt.Printf("  PID: %d\n", meta.PID)
		fmt.Printf("  PGID: %s\n", meta.PGID)
	}
	if meta.StartedAt != "" {
		fmt.Printf("  Started: %s\n", meta.StartedAt)
	}
	if meta.StoppedAt != "" {
		fmt.Printf("  Stopped: %s\n", meta.StoppedAt)
	}

	return nil
}

func destroyAgent(cfg *config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keeper destroy <name>")
	}

	name := args[0]
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("destroying agent")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	if err := store.DeleteAgent(context.Background(), name); err != nil {
		return fmt.Errorf("destroy agent: %w", err)
	}

	logger.Info("agent destroyed")
	fmt.Printf("Agent '%s' destroyed\n", name)
	return nil
}

func recoverAgent(cfg *config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keeper recover <name>")
	}

	name := args[0]
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("recovering agent")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	// 加载 agent 元数据
	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// 清理残留进程（如果有 PID）
	if meta.PID > 0 {
		if err := killProcess(meta.PID); err != nil {
			logger.Warn("kill residual process failed", log.Field{Key: "pid", Value: meta.PID}, log.Field{Key: "error", Value: err})
		}
	}

	// 重置状态
	meta.State = "stopped"
	meta.PID = 0
	meta.PGID = ""
	meta.Error = ""

	if err := store.UpdateAgent(context.Background(), meta); err != nil {
		return fmt.Errorf("update agent state: %w", err)
	}

	logger.Info("agent recovered", log.Field{Key: "state", Value: meta.State})
	fmt.Printf("Agent '%s' recovered\n", name)
	return nil
}

func snapshotAgent(cfg *config.Config, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: keeper snapshot <name> <snapshot-id>")
	}

	name := args[0]
	snapshotID := args[1]
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("creating snapshot", log.Field{Key: "snapshot_id", Value: snapshotID})

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	// 创建快照
	if err := store.CreateSnapshot(context.Background(), name, snapshotID); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}

	logger.Info("snapshot created")
	fmt.Printf("Snapshot '%s' created for agent '%s'\n", snapshotID, name)
	return nil
}

func rollbackAgent(cfg *config.Config, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: keeper rollback <name> <snapshot-id>")
	}

	name := args[0]
	snapshotID := args[1]
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("rolling back agent", log.Field{Key: "snapshot_id", Value: snapshotID})

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	// 回滚快照
	if err := store.RollbackSnapshot(context.Background(), name, snapshotID); err != nil {
		return fmt.Errorf("rollback snapshot: %w", err)
	}

	logger.Info("agent rolled back")
	fmt.Printf("Agent '%s' rolled back to snapshot '%s'\n", name, snapshotID)
	return nil
}

// 辅助函数

func getHomeDir() string {
	if home := os.Getenv("KEEPER_HOME"); home != "" {
		return home
	}
	return filepath.Join(os.Getenv("HOME"), defaultHome)
}

func isValidName(name string) bool {
	if len(name) < 1 || len(name) > 32 {
		return false
	}
	// 必须以字母或数字开头
	first := rune(name[0])
	if !((first >= 'a' && first <= 'z') ||
		(first >= 'A' && first <= 'Z') ||
		(first >= '0' && first <= '9')) {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func listAgents(cfg *config.Config, args []string) error {
	logger := globalLogger
	logger.Info("listing agents")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	agents, err := store.ListAgents(context.Background())
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	if len(agents) == 0 {
		fmt.Println("No agents found.")
		return nil
	}

	// 表头
	fmt.Printf("%-20s %-10s %-20s %-10s\n", "NAME", "STATE", "PORTS", "CREATED")
	fmt.Println("--------------------------------------------------------------------------------------")

	for _, meta := range agents {
		portsStr := "-"
		if len(meta.Ports) > 0 {
			portStrs := make([]string, 0, len(meta.Ports))
			for _, p := range meta.Ports {
				portStrs = append(portStrs, fmt.Sprintf("%d:%d", p.Host, p.Container))
			}
			portsStr = strings.Join(portStrs, ", ")
		}

		created := meta.CreatedAt
		if len(created) > 10 {
			created = created[:10]
		}

		fmt.Printf("%-20s %-10s %-20s %-10s\n", meta.Name, meta.State, portsStr, created)
	}

	return nil
}

func inspectAgent(cfg *config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keeper inspect <name> [--verbose]")
	}

	name := args[0]
	verbose := false
	for _, arg := range args[1:] {
		if arg == "--verbose" {
			verbose = true
		}
	}

	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("inspecting agent")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		return fmt.Errorf("agent '%s' not found: %w", name, err)
	}

	// 基本信息
	fmt.Printf("NAME:       %s\n", meta.Name)
	fmt.Printf("STATE:      %s\n", meta.State)
	fmt.Printf("CREATED:    %s\n", meta.CreatedAt)
	fmt.Printf("SHM_SIZE:   %d MB\n", meta.ShmSizeMB)
	fmt.Printf("MAX_DOWNLOAD: %d MB\n", meta.MaxDownloadBytes/(1024*1024))

	if meta.PGID != "" {
		fmt.Printf("PGID:       %s\n", meta.PGID)
	}

	if meta.CacheKey != "" {
		fmt.Printf("CACHE_KEY:  %s\n", meta.CacheKey)
	}

	if meta.CacheURL != "" {
		fmt.Printf("CACHE_URL:  %s\n", meta.CacheURL)
	}

	// 端口
	if len(meta.Ports) > 0 {
		fmt.Println("PORTS:")
		for _, p := range meta.Ports {
			fmt.Printf("  - %d -> %d\n", p.Host, p.Container)
		}
	}

	// 路径
	fmt.Println("PATHS:")
	fmt.Printf("  ROOTFS:  %s\n", meta.RootfsDir)
	fmt.Printf("  UPPER:   %s\n", meta.UpperDir)
	fmt.Printf("  WORK:    %s\n", meta.WorkDir)
	fmt.Printf("  WORKSPACE: %s\n", meta.Workspace)
	fmt.Printf("  BACKUPS: %s\n", meta.BackupsDir)
	fmt.Printf("  LOGS:    %s\n", meta.LogsDir)

	// 详细设备信息
	if verbose {
		fmt.Println("\nDEVICE INFO:")
		rootfsDev := statDevice(meta.RootfsDir)
		upperDev := statDevice(meta.UpperDir)
		workDev := statDevice(meta.WorkDir)
		workspaceDev := statDevice(meta.Workspace)

		fmt.Printf("  Rootfs Device ID:  0x%x\n", rootfsDev)
		fmt.Printf("  Upper Device ID:   0x%x (Same as rootfs: %v)\n", upperDev, rootfsDev == upperDev)
		fmt.Printf("  Work Device ID:    0x%x (Same as rootfs: %v)\n", workDev, rootfsDev == workDev)
		fmt.Printf("  Workspace Device ID: 0x%x (Same as rootfs: %v)\n", workspaceDev, rootfsDev == workspaceDev)

		// 检查端口文件
		portsPath := filepath.Join(filepath.Dir(meta.RootfsDir), "ports.json")
		if data, err := os.ReadFile(portsPath); err == nil {
			fmt.Printf("\nPORTS_JSON: %s\n", string(data))
		}
	}

	return nil
}

func statDevice(path string) uint64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Sys().(*syscall.Stat_t).Dev
}

func forkAgent(cfg *config.Config, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: keeper fork <source> <target>")
	}

	source := args[0]
	target := args[1]

	if !isValidName(source) || !isValidName(target) {
		return fmt.Errorf("invalid agent name format (must match ^[a-zA-Z0-9_-]{1,32}$)")
	}

	if source == target {
		return fmt.Errorf("source and target must be different")
	}

	logger := globalLogger.WithFields(
		log.Field{Key: "source", Value: source},
		log.Field{Key: "target", Value: target},
	)
	logger.Info("forking agent")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	meta, err := store.ForkAgent(context.Background(), source, target)
	if err != nil {
		return fmt.Errorf("fork agent: %w", err)
	}

	logger.Info("agent forked successfully", log.Field{Key: "state", Value: meta.State})
	fmt.Printf("Agent '%s' forked from '%s' (state: %s)\n", target, source, meta.State)
	return nil
}

func copyFile(cfg *config.Config, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: keeper cp [-r] <source> <destination>")
	}

	recursive := false
	var src, dst string

	// 解析参数
	for _, arg := range args {
		switch arg {
		case "-r", "--recursive":
			recursive = true
		default:
			if src == "" {
				src = arg
			} else if dst == "" {
				dst = arg
			}
		}
	}

	if src == "" || dst == "" {
		return fmt.Errorf("usage: keeper cp [-r] <source> <destination>")
	}

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	// 解析源和目标
	srcAgent, srcPath, srcIsAgent := parseAgentPath(src)
	dstAgent, dstPath, dstIsAgent := parseAgentPath(dst)

	switch {
	case srcIsAgent && dstIsAgent:
		return fmt.Errorf("cannot copy between two agents directly, use local path as intermediate")
	case srcIsAgent:
		// 从 agent workspace 下载到本地
		meta, err := store.GetAgent(context.Background(), srcAgent)
		if err != nil {
			return fmt.Errorf("source agent '%s' not found: %w", srcAgent, err)
		}
		cleanSrc := strings.TrimPrefix(srcPath, "/")
		srcFull := filepath.Join(meta.Workspace, cleanSrc)
		return copyLocalToLocal(srcFull, dst)
	case dstIsAgent:
		// 从本地上传到 agent workspace
		meta, err := store.GetAgent(context.Background(), dstAgent)
		if err != nil {
			return fmt.Errorf("destination agent '%s' not found: %w", dstAgent, err)
		}
		cleanDst := strings.TrimPrefix(dstPath, "/")
		dstFull := filepath.Join(meta.Workspace, cleanDst)
		if recursive {
			return copyDirRecursive(src, dstFull)
		}
		return copyLocalToLocal(src, dstFull)
	default:
		// 本地到本地
		if recursive {
			return copyDirRecursive(src, dst)
		}
		return copyLocalToLocal(src, dst)
	}
}

// parseAgentPath 解析 "agent:path" 格式，返回 agent name、path、是否为 agent 路径
func parseAgentPath(s string) (string, string, bool) {
	// 查找第一个冒号
	idx := strings.Index(s, ":")
	if idx <= 0 {
		return "", "", false
	}

	agentName := s[:idx]
	path := s[idx+1:]

	// 简单验证 agent 名格式
	if !isValidName(agentName) {
		return "", "", false
	}

	// 如果用户输入了容器内的 /workspace 前缀，去掉它
	// 支持 /workspace/ 和 /workspace 两种形式
	if strings.HasPrefix(path, "/workspace/") {
		path = strings.TrimPrefix(path, "/workspace/")
	} else if path == "/workspace" {
		path = ""
	}

	return agentName, path, true
}

func copyLocalToLocal(src, dst string) error {
	// 检查源是否存在
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}

	// 确保目标目录存在
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	if srcInfo.IsDir() {
		// 目录复制
		return copyDirRecursive(src, dst)
	}

	// 文件复制
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	// 保留权限
	srcMode := srcInfo.Mode()
	if err := os.Chmod(dst, srcMode); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}

	return nil
}

func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		// 文件复制
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}

		return os.Chmod(targetPath, info.Mode())
	})
}

// killProcess 终止进程
func killProcess(pid int) error {
	if pid <= 0 {
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	// 发送 SIGTERM
	if err := proc.Signal(os.Interrupt); err != nil {
		return err
	}

	// 等待 5s
	done := make(chan error, 1)
	go func() {
		_, err := proc.Wait()
		done <- err
	}()

	select {
	case <-time.After(5 * time.Second):
		// 强制终止
		if err := proc.Kill(); err != nil {
			return err
		}
		<-done
	case err := <-done:
		return err
	}

	return nil
}

func printUsage() {
	fmt.Fprintf(os.Stdout, `keeper - AI Agent 轻量级 Linux 运行时环境

版本: %s

用法:
  keeper <command> [arguments]

命令:
  create <name>              创建新 Agent
  start <name>               启动 Agent
  stop <name>                停止 Agent
  status <name>              查询 Agent 状态
  list                       列出所有 Agent
  inspect <name> [--verbose] 查看 Agent 详细信息
  fork <source> <target>     克隆 Agent
  cp <src> <dst>             在宿主机和 Agent workspace 之间复制文件
  destroy <name>             销毁 Agent（删除所有数据）
  recover <name>             恢复 Agent（清理残留状态）
  snapshot <name> <id>       创建快照
  rollback <name> <id>       回滚到快照
  version                    显示版本信息
  help                       显示帮助信息

环境变量:
  KEEPER_HOME                数据根目录（默认: ~/.local/share/keeper）

示例:
  keeper create my-agent
  keeper start my-agent
  keeper list
  keeper inspect my-agent --verbose
  keeper fork my-agent my-agent-copy
  keeper cp ./model.bin my-agent:/workspace/models/
  keeper stop my-agent
`, version)
}
