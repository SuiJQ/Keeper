# Keeper API Reference

This document describes the primary Go packages and exported types/functions
in Keeper. It is intended for contributors and advanced users who need to
embed or extend Keeper programmatically.

## internal/agent

State machine and Agent lifecycle.

### Types

- `State` — Agent lifecycle state enumeration.
- `StateMachine` — Thread-safe state transition tracker.
- `Agent` — High-level agent entity with state and metadata.

### Functions

- `NewStateMachine(initial State) *StateMachine`
- `NewAgent(name string) *Agent`
- `(*Agent).UpdateState(to State) error`
- `(*Agent).StateString() string`
- `(*Agent).TransitionCount() int64`

## internal/container

Container runtime abstraction and bwrap backend.

### Interfaces

- `Container` — Start/Stop/Exec/Status/Close lifecycle.
- `Factory` — Creates container runtimes by name.
- `LogStrategy` / `NetworkStrategy` / `OverlayStrategy` / `ResourceStrategy` / `SeccompStrategy` — Pluggable strategies.

### Key Types

- `Spec` — Container specification (rootfs, workspace, shm, ports, env).
- `Status` — Runtime status (state, pid, pgid, uptime, ports).
- `ExecRequest` / `ExecResponse` — Command execution contract.
- `PortMapping` — Host/container port pair.
- `BwrapFactory` — Default bubblewrap-backed factory.

## internal/storage

OverlayFS management, snapshots, caching, and pruning.

### Functions

- `NewStore(dir string) (*Store, error)`
- `(*Store).Create(name string, spec Spec) (Container, error)`
- `(*Store).Snapshot(name string) (string, error)`
- `(*Store).Rollback(name, snapshotID string) error`
- `(*Store).Prune() error`

## internal/mcp

MCP server (JSON-RPC 2.0 over Unix socket).

### Types

- `Server` — MCP server instance.
- `ServerConfig` — Socket path, store, agent name, UID/GID allowlists.

### Functions

- `NewServer(cfg ServerConfig, logger log.Logger) (*Server, error)`
- `(*Server).Start() error`
- `(*Server).Stop() error`

## pkg/config

Configuration loading, validation, and hot-reload.

### Types

- `Config` — Central configuration struct.

### Functions

- `Load(home string) (*Config, error)`
- `LoadDefaultIfExists() (*Config, error)`
- `(*Config).Save() error`
- `(*Config).Validate() error`
- `(*Config).OnReload(fn func(*Config))`

## internal/log

Structured logging with trace/span IDs.

### Functions

- `New(output io.Writer) Logger`
- `Global() Logger`
- `SetGlobal(logger Logger)`
- `GenerateTraceID() string`
- `GenerateSpanID() string`
- `NextTraceID() string`
- `NextSpanID() string`

## internal/errors

Typed error codes and recoverability.

### Types

- `ErrorCode` — Numeric error code enumeration.
- `KeeperError` — Structured error with code, message, cause, recoverability.

### Functions

- `NewKeeperError(code ErrorCode, message string, cause error) *KeeperError`
- `IsKeeperError(err error) (*KeeperError, bool)`
- `IsRecoverable(err error) bool`

## internal/metrics

Prometheus metrics and HTTP server.

### Functions

- `NewHTTPServer(addr string) *HTTPServer`
- `GetHTTPServer() *HTTPServer`

## internal/network

Network manager, port forwarder, and SOCKS5 server.

### Types

- `PortForward` — Host/container port forward config.
- `SOCKS5Proxy` — SOCKS5 listener config with optional auth.
- `Manager` — Network configuration holder.

### Functions

- `NewManager() *Manager`
- `IsPortInUse(port int) bool`
