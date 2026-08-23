package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"keeper/internal/container"
	"keeper/internal/log"
	"keeper/internal/mcp"
	"keeper/internal/metrics"
	"keeper/internal/storage"
	"keeper/internal/watchdog"
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
	case "run":
		return runAgentCommand(cfg, args)
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
	case "metrics":
		fmt.Println(metrics.PrometheusFormat())
		return nil
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

	// 从配置读取默认值
	shmSizeMB := cfg.DefaultShmSizeMB
	maxDownloadBytes := cfg.MaxDownloadBytes

	meta, err := store.CreateAgent(context.Background(), name, shmSizeMB, maxDownloadBytes)
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
		// 创建容器运行时
		factory := container.NewBwrapFactory()
		c, err := factory.Create(name)
		if err != nil {
			return fmt.Errorf("create container: %w", err)
		}
		defer func() { _ = c.Close() }()

		// 构建容器规格
		spec := container.ContainerSpec{
			Name:      name,
			Rootfs:    meta.RootfsDir,
			UpperDir:  meta.UpperDir,
			WorkDir:   meta.WorkDir,
			Workspace: meta.Workspace,
			ShmSize:   meta.ShmSizeMB,
			Envvars:   []string{fmt.Sprintf("AGENT_NAME=%s", name)},
		}

		// 启动容器
		pid, err := c.Start(context.Background(), spec)
		if err != nil {
			// 更新状态为 fatal
			meta.State = "fatal_bwrap_exec"
			meta.Error = err.Error()
			_ = store.UpdateAgent(context.Background(), meta)
			return fmt.Errorf("start container: %w", err)
		}

		// 更新状态
		meta.State = "running"
		meta.PID = pid
		meta.PGID = fmt.Sprintf("%d", pid)
		meta.StartedAt = time.Now().UTC().Format(time.RFC3339)
		meta.Error = ""

		if err := store.UpdateAgent(context.Background(), meta); err != nil {
			return fmt.Errorf("update agent state: %w", err)
		}

		logger.Info("agent started", log.Field{Key: "state", Value: meta.State}, log.Field{Key: "pid", Value: pid})
		fmt.Printf("Agent '%s' started (pid: %d)\n", name, pid)
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

	// 创建容器运行时
	factory := container.NewBwrapFactory()
	c, err := factory.Create(name)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	defer func() { _ = c.Close() }()

	// 根据配置设置策略
	if bc, ok := c.(*container.BwrapContainer); ok {
		if seccompStrat, err := container.NewSeccompStrategy(cfg.SeccompStrategy, logger); err != nil {
			logger.Warn("invalid seccomp strategy, using default", log.Field{Key: "error", Value: err.Error()})
		} else {
			bc.SetSeccompStrategy(seccompStrat)
		}
		if overlayStrat, err := container.NewOverlayStrategy(cfg.OverlayStrategy, logger); err != nil {
			logger.Warn("invalid overlay strategy, using default", log.Field{Key: "error", Value: err.Error()})
		} else {
			bc.SetOverlayStrategy(overlayStrat)
		}
	}

	// 停止容器
	grace := 5 * time.Second
	if err := c.Stop(context.Background(), grace); err != nil {
		logger.Warn("stop container error", log.Field{Key: "error", Value: err})
	}

	// 更新状态
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

// runAgentCommand 运行 Agent（前台模式，启动容器 + MCP + Watchdog）
func runAgentCommand(cfg *config.Config, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: keeper run <name>")
	}

	name := args[0]
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("running agent")

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	// 加载 agent 元数据
	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		return fmt.Errorf("load agent: %w", err)
	}

	// 如果 Agent 不存在，自动创建
	if meta == nil {
		logger.Info("agent does not exist, creating")
		if err := createAgent(cfg, []string{name}); err != nil {
			return fmt.Errorf("create agent: %w", err)
		}
		meta, err = store.GetAgent(context.Background(), name)
		if err != nil {
			return fmt.Errorf("load agent after create: %w", err)
		}
	}

	// 如果 Agent 已停止，先启动
	if meta.State == "stopped" {
		logger.Info("agent is stopped, starting")
		if err := startAgent(cfg, []string{name}); err != nil {
			return fmt.Errorf("start agent: %w", err)
		}
		meta, err = store.GetAgent(context.Background(), name)
		if err != nil {
			return fmt.Errorf("load agent after start: %w", err)
		}
	}

	// 如果 Agent 已在运行，直接进入监控模式
	if meta.State == "running" {
		logger.Info("agent is already running, entering monitor mode")
	} else {
		return fmt.Errorf("cannot run agent in state: %s", meta.State)
	}

	// 创建容器运行时
	factory := container.NewBwrapFactory()
	c, err := factory.Create(name)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	defer func() { _ = c.Close() }()

	// 根据配置设置策略
	if bc, ok := c.(*container.BwrapContainer); ok {
		if seccompStrat, err := container.NewSeccompStrategy(cfg.SeccompStrategy, logger); err != nil {
			logger.Warn("invalid seccomp strategy, using default", log.Field{Key: "error", Value: err.Error()})
		} else {
			bc.SetSeccompStrategy(seccompStrat)
		}
		if overlayStrat, err := container.NewOverlayStrategy(cfg.OverlayStrategy, logger); err != nil {
			logger.Warn("invalid overlay strategy, using default", log.Field{Key: "error", Value: err.Error()})
		} else {
			bc.SetOverlayStrategy(overlayStrat)
		}
	}

	// 启动 MCP Server（使用配置中的授权设置）
	mcpServer, err := mcp.NewServer(mcp.ServerConfig{
		SocketPath:  filepath.Join(cfg.Home, "agents", name, "mcp.sock"),
		AgentName:   name,
		AllowedUIDs: cfg.MCPAllowedUIDs,
		AllowedGIDs: cfg.MCPAllowedGIDs,
	}, logger)
	if err != nil {
		return fmt.Errorf("create mcp server: %w", err)
	}

	// 启动看门狗（使用配置中的超时设置）
	watchdogTimeout, err := cfg.WatchdogTimeoutDuration()
	if err != nil {
		return fmt.Errorf("parse watchdog timeout: %w", err)
	}
	watchdogInterval, err := cfg.WatchdogCheckIntervalDuration()
	if err != nil {
		return fmt.Errorf("parse watchdog check interval: %w", err)
	}

	wd := watchdog.NewWatchdog(watchdog.Config{
		Timeout:       watchdogTimeout,
		CheckInterval: watchdogInterval,
	}, logger)

	// 注册配置热加载回调
	cfg.OnReload(func(newCfg *config.Config) {
		// 注意：日志级别不支持热加载，需要重启进程生效
		// logger.SetLevel(newCfg.LogLevel)

		// 更新 MCP Server 授权配置
		mcpServer.UpdateAllowedUIDs(newCfg.MCPAllowedUIDs)
		mcpServer.UpdateAllowedGIDs(newCfg.MCPAllowedGIDs)

		// 更新看门狗超时配置
		newTimeout, err := newCfg.WatchdogTimeoutDuration()
		if err == nil {
			wd.UpdateTimeout(newTimeout)
		}
		newInterval, err := newCfg.WatchdogCheckIntervalDuration()
		if err == nil {
			wd.UpdateCheckInterval(newInterval)
		}

		// 更新容器策略配置
		if bc, ok := c.(*container.BwrapContainer); ok {
			if seccompStrat, err := container.NewSeccompStrategy(newCfg.SeccompStrategy, logger); err != nil {
				logger.Warn("invalid seccomp strategy, using default", log.Field{Key: "error", Value: err.Error()})
			} else {
				bc.SetSeccompStrategy(seccompStrat)
			}
			if overlayStrat, err := container.NewOverlayStrategy(newCfg.OverlayStrategy, logger); err != nil {
				logger.Warn("invalid overlay strategy, using default", log.Field{Key: "error", Value: err.Error()})
			} else {
				bc.SetOverlayStrategy(overlayStrat)
			}
			if networkStrat, err := container.NewNetworkStrategy("default", logger); err != nil {
				logger.Warn("invalid network strategy, using default", log.Field{Key: "error", Value: err.Error()})
			} else {
				bc.SetNetworkStrategy(networkStrat)
			}
			if resourceStrat, err := container.NewResourceStrategy("default", logger); err != nil {
				logger.Warn("invalid resource strategy, using default", log.Field{Key: "error", Value: err.Error()})
			} else {
				bc.SetResourceStrategy(resourceStrat)
			}
			if logStrat, err := container.NewLogStrategy("default", logger); err != nil {
				logger.Warn("invalid log strategy, using default", log.Field{Key: "error", Value: err.Error()})
			} else {
				bc.SetLogStrategy(logStrat)
			}
		}

		logger.Info("configuration reloaded",
			log.Field{Key: "log_level", Value: newCfg.LogLevel},
			log.Field{Key: "shm_size_mb", Value: newCfg.DefaultShmSizeMB},
			log.Field{Key: "watchdog_timeout", Value: newCfg.WatchdogTimeout},
			log.Field{Key: "seccomp_strategy", Value: newCfg.SeccompStrategy},
			log.Field{Key: "overlay_strategy", Value: newCfg.OverlayStrategy},
			log.Field{Key: "snapshot_compression_level", Value: newCfg.SnapshotCompressionLevel})
	})

	// 注册 Agent 到看门狗
	wd.RegisterAgent(name, meta.PID)

	// 启动服务
	ctx := context.Background()
	if err := mcpServer.Start(ctx); err != nil {
		return fmt.Errorf("start mcp server: %w", err)
	}
	defer func() { _ = mcpServer.Stop() }()

	if err := wd.Start(ctx); err != nil {
		return fmt.Errorf("start watchdog: %w", err)
	}
	defer wd.Stop()

	// 启动指标服务器
	metricsListenAddr := cfg.MetricsListenAddr
	if !cfg.MetricsEnabled {
		metricsListenAddr = "" // 禁用
	}
	if metricsListenAddr != "" {
		metricsServer := metrics.NewHTTPServer(metricsListenAddr)
		metricsServer.SetHealthCheck(func() error {
			// 检查 MCP Server 是否运行
			if !mcpServer.IsRunning() {
				return fmt.Errorf("mcp server not running")
			}
			return nil
		})
		metricsServer.SetReadyCheck(func() error {
			// 检查看门狗是否运行
			if !wd.IsRunning() {
				return fmt.Errorf("watchdog not running")
			}
			return nil
		})
		if err := metricsServer.Start(); err != nil {
			return fmt.Errorf("start metrics server: %w", err)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsServer.Stop(shutdownCtx)
		}()
	}

	// 记录 agent 启动指标
	agentStartCounter := metrics.RegisterCounter("keeper_agent_starts_total", "Total number of agent starts", []string{"agent_name", "state"})
	agentStartCounter.Inc(name, "created")

	logger.Info("agent running (MCP + Watchdog + Metrics active)")
	fmt.Printf("Agent '%s' is running (pid: %d)\n", name, meta.PID)
	fmt.Println("Press Ctrl+C to stop")

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// 启动配置热加载 goroutine
	reloadDone := make(chan struct{})
	go func() {
		defer close(reloadDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := cfg.ReloadIfChanged(); err != nil {
					logger.Warn("config reload failed", log.Field{Key: "error", Value: err.Error()})
				}
			case <-sigCh:
				return
			}
		}
	}()

	<-sigCh

	logger.Info("received stop signal, shutting down")
	fmt.Println("\nShutting down...")

	// 等待热加载 goroutine 结束
	<-reloadDone

	// 停止看门狗
	wd.Stop()

	// 停止 MCP Server
	if err := mcpServer.Stop(); err != nil {
		logger.Warn("stop mcp server error", log.Field{Key: "error", Value: err})
	}

	// 停止容器
	if err := stopAgent(cfg, []string{name}); err != nil {
		logger.Warn("stop agent error", log.Field{Key: "error", Value: err})
	}

	logger.Info("agent stopped")
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
	if err := store.CreateSnapshot(context.Background(), name, snapshotID, cfg.SnapshotCompressionLevel); err != nil {
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

func listAgents(cfg *config.Config, _ []string) error {
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

	// 清理路径，防止路径遍历
	path = filepath.Clean(path)

	// 如果清理后路径包含 ".."，拒绝请求
	if strings.Contains(path, "..") {
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
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

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

		dstFile, err := os.Create(targetPath)
		if err != nil {
			_ = srcFile.Close()
			return err
		}

		_, copyErr := io.Copy(dstFile, srcFile)
		srcFileCloseErr := srcFile.Close()
		dstFileCloseErr := dstFile.Close()

		if copyErr != nil {
			return copyErr
		}
		if srcFileCloseErr != nil {
			return srcFileCloseErr
		}
		if dstFileCloseErr != nil {
			return dstFileCloseErr
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
	_, _ = fmt.Fprintf(os.Stdout, `keeper - AI Agent 轻量级 Linux 运行时环境

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
