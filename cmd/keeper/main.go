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
	appName         = "keeper"
	version         = "0.1.0-dev"
	defaultHome     = ".local/share/keeper"
	stateRunning    = "running"
	stateStopped    = "stopped"
	stateFatalBwrap = "fatal_bwrap_exec"
)

var (
	globalLogger    = log.New(os.Stdout)
	commandHandlers = map[string]func(*config.Config, []string) error{
		"create":   createAgent,
		"start":    startAgent,
		"stop":     stopAgent,
		"run":      runAgentCommand,
		"status":   statusAgent,
		"list":     listAgents,
		"inspect":  inspectAgent,
		"fork":     forkAgent,
		"cp":       copyFile,
		"destroy":  destroyAgent,
		"recover":  recoverAgent,
		"snapshot": snapshotAgent,
		"rollback": rollbackAgent,
	}
)

func main() {
	if err := run(); err != nil {
		globalLogger.Error("application failed", log.Field{Key: "error", Value: err.Error()})
		os.Exit(1)
	}
}

func run() error {
	command, args, err := parseCLI()
	if err != nil {
		return err
	}

	cfg, err := loadConfigForCLI()
	if err != nil {
		return err
	}

	return routeCommand(cfg, command, args)
}

func parseCLI() (string, []string, error) {
	if len(os.Args) < 2 {
		printUsage()
		return "", nil, nil
	}

	command := os.Args[1]
	args := os.Args[2:]
	if command == "" {
		return "", nil, fmt.Errorf("missing command")
	}
	return command, args, nil
}

func loadConfigForCLI() (*config.Config, error) {
	home := getHomeDir()
	cfg, err := config.Load(home)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	logger := globalLogger.WithFields(log.Field{Key: "home", Value: home})
	log.SetGlobal(logger)
	return cfg, nil
}

func routeCommand(cfg *config.Config, command string, args []string) error {
	if handler, ok := commandHandlers[command]; ok {
		return handler(cfg, args)
	}

	switch command {
	case "metrics":
		fmt.Println(metrics.PrometheusFormat())
		return nil
	case "version":
		fmt.Printf("keeper %s\n", version)
		return nil
	case "help":
		printUsage()
		return nil
	}

	return fmt.Errorf("unknown command: %s (try 'help')", command)
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
	name, err := requireAgentName(args, "start")
	if err != nil {
		return err
	}
	return startAgentByName(cfg, name)
}

func stopAgent(cfg *config.Config, args []string) error {
	name, err := requireAgentName(args, "stop")
	if err != nil {
		return err
	}
	return stopAgentByName(cfg, name)
}

func requireAgentName(args []string, verb string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("usage: keeper %s <name>", verb)
	}
	return args[0], nil
}

func withAgentStore(cfg *config.Config) (storage.Store, error) {
	return storage.NewStore(cfg.Home)
}

func startAgentByName(cfg *config.Config, name string) error {
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("starting agent")

	store, err := withAgentStore(cfg)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	meta, err := loadAgentMetaForStart(store, name)
	if err != nil {
		return err
	}
	if meta == nil {
		logger.Info("agent already running")
		fmt.Printf("Agent '%s' is already running\n", name)
		return nil
	}

	return startAgentContainer(store, name, meta, logger)
}

func loadAgentMetaForStart(store storage.Store, name string) (*storage.AgentMeta, error) {
	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		return nil, fmt.Errorf("load agent: %w", err)
	}
	if meta.State == stateRunning {
		return nil, nil
	}
	return meta, nil
}

func startAgentContainer(store storage.Store, name string, meta *storage.AgentMeta, logger log.Logger) error {
	if meta.State != "created" && meta.State != stateStopped {
		return fmt.Errorf("cannot start agent in state: %s", meta.State)
	}

	factory := container.NewBwrapFactory()
	c, err := factory.Create(name)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}

	spec := buildAgentContainerSpec(meta, name)
	pid, err := c.Start(context.Background(), spec)
	if err != nil {
		_ = c.Close()
		meta.State = stateFatalBwrap
		meta.Error = err.Error()
		_ = store.UpdateAgent(context.Background(), meta)
		return fmt.Errorf("start container: %w", err)
	}

	container.Register(name, c)
	updateAgentRunningMeta(meta, pid)

	if err := store.UpdateAgent(context.Background(), meta); err != nil {
		container.Unregister(name)
		_ = c.Close()
		return fmt.Errorf("update agent state: %w", err)
	}

	logger.Info("agent started", log.Field{Key: "state", Value: meta.State}, log.Field{Key: "pid", Value: pid})
	fmt.Printf("Agent '%s' started (pid: %d)\n", name, pid)
	return nil
}

func buildAgentContainerSpec(meta *storage.AgentMeta, name string) container.Spec {
	return container.Spec{
		Name:      name,
		Rootfs:    meta.RootfsDir,
		UpperDir:  meta.UpperDir,
		WorkDir:   meta.WorkDir,
		Workspace: meta.Workspace,
		ShmSize:   meta.ShmSizeMB,
		Envvars:   []string{fmt.Sprintf("AGENT_NAME=%s", name)},
	}
}

func updateAgentRunningMeta(meta *storage.AgentMeta, pid int) {
	meta.State = stateRunning
	meta.PID = pid
	meta.PGID = fmt.Sprintf("%d", pid)
	meta.StartedAt = time.Now().UTC().Format(time.RFC3339)
	meta.Error = ""
}

func stopAgentByName(cfg *config.Config, name string) error {
	logger := globalLogger.WithFields(log.Field{Key: "agent_name", Value: name})
	logger.Info("stopping agent")

	store, err := withAgentStore(cfg)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	meta, err := loadAgentMetaForStop(store, name)
	if err != nil {
		return err
	}
	if meta == nil {
		logger.Info("agent already stopped")
		fmt.Printf("Agent '%s' is already stopped\n", name)
		return nil
	}

	return stopAgentContainer(cfg, store, name, meta, logger)
}

func loadAgentMetaForStop(store storage.Store, name string) (*storage.AgentMeta, error) {
	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		return nil, fmt.Errorf("load agent: %w", err)
	}
	if meta.State == stateStopped {
		return nil, nil
	}
	if meta.State != stateRunning {
		return nil, fmt.Errorf("cannot stop agent in state: %s", meta.State)
	}
	return meta, nil
}

func stopAgentContainer(cfg *config.Config, store storage.Store, name string, meta *storage.AgentMeta, logger log.Logger) error {
	c := getContainerForStop(name)

	applyStopContainerStrategies(c, cfg, logger)

	grace := 5 * time.Second
	if err := c.Stop(context.Background(), grace); err != nil {
		logger.Warn("stop container error", log.Field{Key: "error", Value: err})
	}

	container.Unregister(name)
	finalizeAgentStop(store, meta)

	logger.Info("agent stopped", log.Field{Key: "state", Value: meta.State})
	fmt.Printf("Agent '%s' stopped\n", name)
	return nil
}

func getContainerForStop(name string) container.Container {
	c, registered := container.Get(name)
	if !registered {
		factory := container.NewBwrapFactory()
		c, _ = factory.Create(name)
	}
	return c
}

func applyStopContainerStrategies(c container.Container, cfg *config.Config, logger log.Logger) {
	bc, ok := c.(*container.BwrapContainer)
	if !ok {
		return
	}
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

func finalizeAgentStop(store storage.Store, meta *storage.AgentMeta) {
	meta.State = stateStopped
	meta.StoppedAt = time.Now().UTC().Format(time.RFC3339)
	meta.PID = 0
	meta.PGID = ""
	_ = store.UpdateAgent(context.Background(), meta)
}

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

	meta, err := ensureRunningAgent(cfg, store, name)
	if err != nil {
		return err
	}

	runCtx, err := buildRunContext(cfg, name, logger)
	if err != nil {
		return err
	}
	defer runCtx.Close()

	registerReloadHandler(cfg, runCtx.MCP, runCtx.Watchdog, runCtx.Container, logger)

	if err := runCtx.Start(context.Background()); err != nil {
		return err
	}

	metrics.RegisterCounter("keeper_agent_starts_total", "Total number of agent starts", []string{"agent_name", "state"}).Inc(name, "created")
	logger.Info("agent running (MCP + Watchdog + Metrics active)")
	fmt.Printf("Agent '%s' is running (pid: %d)\n", name, meta.PID)
	fmt.Println("Press Ctrl+C to stop")

	waitForAgentShutdown(cfg, logger, runCtx.MCP, name)
	return nil
}

type agentRunContext struct {
	Container     container.Container
	MCP           *mcp.Server
	Watchdog      *watchdog.Watchdog
	ownsContainer bool
	cfg           *config.Config
}

func (r *agentRunContext) Close() {
	if r == nil {
		return
	}
	if r.ownsContainer && r.Container != nil {
		_ = r.Container.Close()
	}
}

func (r *agentRunContext) Start(ctx context.Context) error {
	if err := r.MCP.Start(ctx); err != nil {
		return fmt.Errorf("start mcp server: %w", err)
	}
	if err := r.Watchdog.Start(ctx); err != nil {
		return fmt.Errorf("start watchdog: %w", err)
	}
	return startAgentMetrics(r.cfg, r.MCP, r.Watchdog)
}

func buildRunContext(cfg *config.Config, name string, logger log.Logger) (*agentRunContext, error) {
	var c container.Container
	var ownsContainer bool

	if registered, ok := container.Get(name); ok {
		c = registered
	} else {
		var err error
		c, err = createAgentContainer(name)
		if err != nil {
			return nil, err
		}
		ownsContainer = true
	}

	if bc, ok := c.(*container.BwrapContainer); ok {
		applyContainerStrategies(bc, cfg, logger)
	}

	mcpServer, err := createAgentMCPServer(cfg, name, logger)
	if err != nil {
		if ownsContainer {
			_ = c.Close()
		}
		return nil, err
	}

	wd, err := createAgentWatchdog(cfg, logger)
	if err != nil {
		_ = mcpServer.Stop()
		if ownsContainer {
			_ = c.Close()
		}
		return nil, err
	}

	return &agentRunContext{
		Container:     c,
		MCP:           mcpServer,
		Watchdog:      wd,
		ownsContainer: ownsContainer,
		cfg:           cfg,
	}, nil
}

func createAgentContainer(name string) (container.Container, error) {
	factory := container.NewBwrapFactory()
	c, err := factory.Create(name)
	if err != nil {
		return nil, fmt.Errorf("create container: %w", err)
	}
	return c, nil
}

func createAgentMCPServer(cfg *config.Config, name string, logger log.Logger) (*mcp.Server, error) {
	mcpServer, err := mcp.NewServer(mcp.ServerConfig{
		SocketPath:  filepath.Join(cfg.Home, "agents", name, "mcp.sock"),
		AgentName:   name,
		AllowedUIDs: cfg.MCPAllowedUIDs,
		AllowedGIDs: cfg.MCPAllowedGIDs,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("create mcp server: %w", err)
	}
	return mcpServer, nil
}

func createAgentWatchdog(cfg *config.Config, logger log.Logger) (*watchdog.Watchdog, error) {
	watchdogTimeout, err := cfg.WatchdogTimeoutDuration()
	if err != nil {
		return nil, fmt.Errorf("parse watchdog timeout: %w", err)
	}
	watchdogInterval, err := cfg.WatchdogCheckIntervalDuration()
	if err != nil {
		return nil, fmt.Errorf("parse watchdog check interval: %w", err)
	}
	return watchdog.NewWatchdog(watchdog.Config{
		Timeout:       watchdogTimeout,
		CheckInterval: watchdogInterval,
	}, logger), nil
}

func applyContainerStrategies(bc *container.BwrapContainer, cfg *config.Config, logger log.Logger) {
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

func registerReloadHandler(cfg *config.Config, mcpServer *mcp.Server, wd *watchdog.Watchdog, c container.Container, logger log.Logger) {
	cfg.OnReload(func(newCfg *config.Config) {
		mcpServer.UpdateAllowedUIDs(newCfg.MCPAllowedUIDs)
		mcpServer.UpdateAllowedGIDs(newCfg.MCPAllowedGIDs)
		if newTimeout, err := newCfg.WatchdogTimeoutDuration(); err == nil {
			wd.UpdateTimeout(newTimeout)
		}
		if newInterval, err := newCfg.WatchdogCheckIntervalDuration(); err == nil {
			wd.UpdateCheckInterval(newInterval)
		}
		if bc, ok := c.(*container.BwrapContainer); ok {
			applyContainerStrategies(bc, newCfg, logger)
		}
		logger.Info("configuration reloaded",
			log.Field{Key: "log_level", Value: newCfg.LogLevel},
			log.Field{Key: "shm_size_mb", Value: newCfg.DefaultShmSizeMB},
			log.Field{Key: "watchdog_timeout", Value: newCfg.WatchdogTimeout},
			log.Field{Key: "seccomp_strategy", Value: newCfg.SeccompStrategy},
			log.Field{Key: "overlay_strategy", Value: newCfg.OverlayStrategy},
			log.Field{Key: "snapshot_compression_level", Value: newCfg.SnapshotCompressionLevel})
	})
}

func startAgentMetrics(cfg *config.Config, mcpServer *mcp.Server, wd *watchdog.Watchdog) error {
	metricsListenAddr := cfg.MetricsListenAddr
	if !cfg.MetricsEnabled || metricsListenAddr == "" {
		return nil
	}
	metricsServer := metrics.NewHTTPServer(metricsListenAddr)
	metricsServer.SetHealthCheck(func() error {
		if !mcpServer.IsRunning() {
			return fmt.Errorf("mcp server not running")
		}
		return nil
	})
	metricsServer.SetReadyCheck(func() error {
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
	return nil
}

func waitForAgentShutdown(cfg *config.Config, logger log.Logger, mcpServer *mcp.Server, name string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

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
	<-reloadDone

	if err := mcpServer.Stop(); err != nil {
		logger.Warn("stop mcp server error", log.Field{Key: "error", Value: err})
	}
	if err := stopAgent(cfg, []string{name}); err != nil {
		logger.Warn("stop agent error", log.Field{Key: "error", Value: err})
	}

	logger.Info("agent stopped")
	fmt.Printf("Agent '%s' stopped\n", name)
}

func ensureRunningAgent(cfg *config.Config, store storage.Store, name string) (*storage.AgentMeta, error) {
	meta, err := store.GetAgent(context.Background(), name)
	if err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("load agent: %w", err)
		}
		// Agent not found, create it
		if err := createAgent(cfg, []string{name}); err != nil {
			return nil, fmt.Errorf("create agent: %w", err)
		}
		meta, err = store.GetAgent(context.Background(), name)
		if err != nil {
			return nil, fmt.Errorf("load agent after create: %w", err)
		}
	}
	if meta.State == stateStopped {
		if err := startAgent(cfg, []string{name}); err != nil {
			return nil, fmt.Errorf("start agent: %w", err)
		}
		meta, err = store.GetAgent(context.Background(), name)
		if err != nil {
			return nil, fmt.Errorf("load agent after start: %w", err)
		}
	}
	if meta.State != stateRunning {
		return nil, fmt.Errorf("cannot run agent in state: %s", meta.State)
	}
	return meta, nil
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
	if meta.State == stateRunning && meta.PID > 0 {
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
	meta.State = stateStopped
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
	return isValidFirstRune(rune(name[0])) && isValidRunes(name)
}

func isValidFirstRune(first rune) bool {
	return (first >= 'a' && first <= 'z') ||
		(first >= 'A' && first <= 'Z') ||
		(first >= '0' && first <= '9')
}

func isValidRunes(name string) bool {
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
	verbose := hasVerboseFlag(args[1:])

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

	printAgentSummary(meta)
	if verbose {
		printAgentDeviceInfo(meta)
	}

	return nil
}

func hasVerboseFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--verbose" {
			return true
		}
	}
	return false
}

func printAgentSummary(meta *storage.AgentMeta) {
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

	if len(meta.Ports) > 0 {
		fmt.Println("PORTS:")
		for _, p := range meta.Ports {
			fmt.Printf("  - %d -> %d\n", p.Host, p.Container)
		}
	}

	fmt.Println("PATHS:")
	fmt.Printf("  ROOTFS:    %s\n", meta.RootfsDir)
	fmt.Printf("  UPPER:     %s\n", meta.UpperDir)
	fmt.Printf("  WORK:      %s\n", meta.WorkDir)
	fmt.Printf("  WORKSPACE: %s\n", meta.Workspace)
	fmt.Printf("  BACKUPS:   %s\n", meta.BackupsDir)
	fmt.Printf("  LOGS:      %s\n", meta.LogsDir)
}

func printAgentDeviceInfo(meta *storage.AgentMeta) {
	fmt.Println("\nDEVICE INFO:")
	rootfsDev := statDevice(meta.RootfsDir)
	upperDev := statDevice(meta.UpperDir)
	workDev := statDevice(meta.WorkDir)
	workspaceDev := statDevice(meta.Workspace)

	fmt.Printf("  Rootfs Device ID:   0x%x\n", rootfsDev)
	fmt.Printf("  Upper Device ID:    0x%x (Same as rootfs: %v)\n", upperDev, rootfsDev == upperDev)
	fmt.Printf("  Work Device ID:     0x%x (Same as rootfs: %v)\n", workDev, rootfsDev == workDev)
	fmt.Printf("  Workspace Device ID: 0x%x (Same as rootfs: %v)\n", workspaceDev, rootfsDev == workspaceDev)

	portsPath := safeJoin(filepath.Dir(meta.RootfsDir), "ports.json")
	data, err := os.ReadFile(portsPath) // #nosec G304
	if err == nil {
		fmt.Printf("\nPORTS_JSON: %s\n", string(data))
	}
}

func safeJoin(dir, name string) string {
	return filepath.Join(dir, filepath.Base(name))
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

	recursive, src, dst, err := parseCopyArgs(args)
	if err != nil {
		return err
	}

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		return fmt.Errorf("create store: %w", err)
	}

	return copyBetweenPaths(store, src, dst, recursive)
}

func parseCopyArgs(args []string) (bool, string, string, error) {
	recursive := false
	var src, dst string
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
		return false, "", "", fmt.Errorf("usage: keeper cp [-r] <source> <destination>")
	}
	return recursive, src, dst, nil
}

func copyBetweenPaths(store storage.Store, src, dst string, recursive bool) error {
	srcAgent, srcPath, srcIsAgent := parseAgentPath(src)
	dstAgent, dstPath, dstIsAgent := parseAgentPath(dst)

	switch {
	case srcIsAgent && dstIsAgent:
		return fmt.Errorf("cannot copy between two agents directly, use local path as intermediate")
	case srcIsAgent:
		meta, err := store.GetAgent(context.Background(), srcAgent)
		if err != nil {
			return fmt.Errorf("source agent '%s' not found: %w", srcAgent, err)
		}
		srcFull := filepath.Join(meta.Workspace, strings.TrimPrefix(srcPath, "/"))
		return copyLocalToLocal(srcFull, dst)
	case dstIsAgent:
		meta, err := store.GetAgent(context.Background(), dstAgent)
		if err != nil {
			return fmt.Errorf("destination agent '%s' not found: %w", dstAgent, err)
		}
		dstFull := filepath.Join(meta.Workspace, strings.TrimPrefix(dstPath, "/"))
		if recursive {
			return copyDirRecursive(src, dstFull)
		}
		return copyLocalToLocal(src, dstFull)
	default:
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
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}

	if srcInfo.IsDir() {
		return copyDirRecursive(src, dst)
	}

	return copyFileToPath(src, dst, srcInfo.Mode())
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

		return copyFileToPath(path, targetPath, info.Mode())
	})
}

func copyFileToPath(src, dst string, srcMode os.FileMode) error {
	srcFile, err := os.Open(src) // #nosec G304
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}
	dstFile, err := os.Create(dst) // #nosec G304
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}

	return os.Chmod(dst, srcMode)
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
