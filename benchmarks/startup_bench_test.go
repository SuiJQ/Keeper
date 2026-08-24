package benchmarks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"keeper/internal/container"
	"keeper/internal/mcp"
	"keeper/internal/storage"
	"keeper/internal/watchdog"
	"keeper/pkg/config"
)

// BenchmarkStartupCLI measures end-to-end CLI invocation overhead for a trivial command.
// This is a stable proxy for process cold-start + config load + routing cost.
func BenchmarkStartupCLI(b *testing.B) {
	b.ReportAllocs()

	// Build keeper binary once before the benchmark loop.
	binPath := filepath.Join("..", "bin", "bench-keeper")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/keeper") // #nosec G204
	buildCmd.Dir = mustProjectRoot(b)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		b.Fatalf("build keeper failed: %v\n%s", err, out)
	}
	defer func() { _ = os.Remove(binPath) }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		cmd := exec.Command(binPath, "version") // #nosec G204
		_ = cmd.Run()
		b.ReportMetric(time.Since(start).Seconds(), "s/op")
	}
}

// BenchmarkStartupInProcess measures in-process cold-start path:
// config load -> store create -> agent create -> container create -> start.
// Uses mock container to avoid docker/bwrap dependency in benchmarks.
func BenchmarkStartupInProcess(b *testing.B) {
	b.ReportAllocs()
	b.StopTimer()

	tmpDir := b.TempDir()
	cfg := config.DefaultConfig()
	cfg.Home = tmpDir
	cfg.BinDir = filepath.Join(tmpDir, "bin")
	cfg.CacheDir = filepath.Join(tmpDir, "cache")
	cfg.AgentsDir = filepath.Join(tmpDir, "agents")
	cfg.ContainerRuntime = "mock"
	for _, dir := range []string{cfg.Home, cfg.BinDir, cfg.CacheDir, cfg.AgentsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			b.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	store, err := storage.NewStore(cfg.Home)
	if err != nil {
		b.Fatalf("create store: %v", err)
	}

	agentName := "bench-startup-agent"
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		// Reset state per iteration without affecting timing.
		b.StopTimer()
		_ = store.DeleteAgent(context.Background(), agentName)

		meta, err := store.CreateAgent(context.Background(), agentName, 64, 1024*1024)
		if err != nil {
			b.Fatalf("create agent: %v", err)
		}

		factory, err := container.NewFactory(cfg.ContainerRuntime)
		if err != nil {
			b.Fatalf("create factory: %v", err)
		}

		c, err := factory.Create(agentName)
		if err != nil {
			b.Fatalf("create container: %v", err)
		}

		spec := container.Spec{
			Name:      agentName,
			Rootfs:    meta.RootfsDir,
			UpperDir:  meta.UpperDir,
			WorkDir:   meta.WorkDir,
			Workspace: meta.Workspace,
			ShmSize:   meta.ShmSizeMB,
			Envvars:   []string{"AGENT_NAME=" + agentName},
		}

		_, err = c.Start(context.Background(), spec)
		if err != nil {
			b.Fatalf("start container: %v", err)
		}

		// Create MCP server (do not start listener to avoid socket churn in benchmark).
		_, err = mcp.NewServer(mcp.ServerConfig{
			SocketPath:  filepath.Join(cfg.Home, "agents", agentName, "mcp.sock"),
			AgentName:   agentName,
			AllowedUIDs: cfg.MCPAllowedUIDs,
			AllowedGIDs: cfg.MCPAllowedGIDs,
		}, nil)
		if err != nil {
			b.Fatalf("create mcp server: %v", err)
		}

		watchdogTimeout, _ := time.ParseDuration(cfg.WatchdogTimeout)
		watchdogInterval, _ := time.ParseDuration(cfg.WatchdogCheckInterval)
		wd := watchdog.NewWatchdog(watchdog.Config{
			Timeout:       watchdogTimeout,
			CheckInterval: watchdogInterval,
		}, nil)

		_ = wd
		_ = c.Close()
		b.StartTimer()
	}
}

func mustProjectRoot(b *testing.B) string {
	b.Helper()
	// benchmarks/ is under workspace root, so parent is project root.
	root, err := filepath.Abs("..")
	if err != nil {
		b.Fatalf("resolve project root: %v", err)
	}
	return root
}
