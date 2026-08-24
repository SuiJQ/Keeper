# Keeper 架构设计

## 设计原则

1. **收敛性优先**：所有异常在有限步骤内进入确定性终态（Running / Stopped / Fatal_*）
2. **可维护性**：模块边界清晰，接口抽象，拒绝屎山代码
3. **可监测性**：结构化日志，明确错误码，状态变更可追溯
4. **可扩展性**：后端抽象（docker / bwrap / mock），插件化策略
5. **永不阻塞**：所有 I/O 带超时，所有进程派生带超时控制

## 核心接口

### Container

容器运行时抽象，支持多后端实现。

```go
type Container interface {
    Start(ctx context.Context, spec ContainerSpec) (int, error)
    Stop(ctx context.Context, grace time.Duration) error
    Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error)
    Status(ctx context.Context) (*ContainerStatus, error)
    Close() error
}
```

**后端实现**：
- `docker`：Docker 后端，默认运行时，依赖 Docker Engine
- `bwrap`：bubblewrap 后端，兼容选项，依赖内核 UserNS + OverlayFS
- `mock`：模拟后端，用于单元测试

### StateMachine

严格的状态机，防止非法状态转换。

```
created ──start──→ running ──stop──→ stopped
   ↑                  │
   │                  ├── watchdog_timeout ──→ fatal_d_state
   │                  ├── probe_fail ──→ fatal_unsupported_kernel
   │                  ├── container_fail ──→ fatal_container_exec
   │                  └── no_space ──→ fatal_no_space
   │
   └── recover ←── recover ←── recover ←── recover
```

**状态定义**：
- `created`：Agent 已创建，未启动
- `running`：Agent 正在运行
- `stopped`：Agent 已停止，数据保留
- `fatal_*`：不可恢复错误，需人工介入

### Logger

结构化日志接口，支持 JSON 输出。

```go
type Logger interface {
    Info(msg string, fields ...Field)
    Warn(msg string, fields ...Field)
    Error(msg string, fields ...Field)
    Debug(msg string, fields ...Field)
}
```

### KeeperError

分级错误体系，明确错误码和恢复策略。

```go
type KeeperError struct {
    Code    ErrorCode
    Message string
    Err     error
}

type ErrorCode int

const (
    ErrCodeOK ErrorCode = iota
    ErrCodeFatalKernel
    ErrCodeFatalBwrap
    ErrCodeFatalDState
    ErrCodeFatalNoSpace
    ErrCodeFatalBwrapExec
    ErrCodeErrorAgentNotFound
    ErrCodeErrorInvalidState
    ErrCodeErrorStorage
)
```

## 模块设计

### cmd/keeper

CLI 入口，负责参数解析、命令分发。

**命令**：
- `create`：创建 Agent
- `start`：启动 Agent
- `stop`：停止 Agent
- `status`：查询 Agent 状态
- `destroy`：销毁 Agent
- `recover`：恢复 Agent
- `list`：列出所有 Agent
- `inspect`：查看 Agent 详细信息
- `fork`：克隆 Agent
- `cp`：复制文件

### internal/agent

Agent 生命周期管理，负责状态机维护和事件处理。

**职责**：
- 状态转换验证
- 事件记录
- 状态持久化

### internal/container

容器运行时抽象，支持多后端。

**职责**：
- 容器启动/停止/执行
- 资源限制
- 命名空间隔离

### internal/storage

存储管理，负责 OverlayFS、快照、缓存。

**职责**：
- Agent 目录管理
- OverlayFS 挂载/卸载
- 快照创建/回滚
- 缓存清理

### internal/mcp

MCP 控制面，提供结构化控制接口。

**协议**：JSON-RPC 2.0 over Unix Domain Socket

**工具**：
- `keeper.create`
- `keeper.start`
- `keeper.stop`
- `keeper.list`
- `keeper.inspect`
- `keeper.fork`
- `keeper.cp`

### internal/watchdog

看门狗机制，确保容器不会无限期挂起。

**策略**：
- 60s 超时收敛
- D 态进程检测
- 优雅/强制停止

### internal/bootstrap

启动与初始化，负责环境探测。

**探测项**：
- 内核版本
- CONFIG_OVERLAY_FS_USERNS
- bubblewrap 可用性
- Seccomp 支持

## 数据流

### 创建 Agent

```
CLI → createAgent → storage.CreateAgent → 创建目录结构 → 写入 meta.json
```

### 启动 Agent

```
CLI → startAgent → bootstrap.ProbeEnvironment → 环境检查
    → container.Start → bwrap 启动 → 更新状态
```

### 停止 Agent

```
CLI → stopAgent → watchdog.TriggerStop → container.Stop → 更新状态
```

## 存储布局

```
$KEEPER_HOME/
├── agents/
│   └── <agent-name>/
│       ├── meta.json          # Agent 元数据
│       ├── ports.json         # 端口映射
│       ├── rootfs/            # 只读根文件系统
│       ├── upper/             # OverlayFS 可写层
│       ├── work/              # OverlayFS 工作目录
│       └── workspace/         # 持久化工作区
├── cache/
│   └── rootfs/                # 根文件系统缓存
└── config.json                # 全局配置
```

## 安全设计

### 命名空间隔离

- **UserNS**：非 root 用户运行容器
- **PIDNS**：独立的 PID 空间
- **NETNS**：独立的网络空间
- **MountNS**：独立的文件系统视图

### 文件系统隔离

- **OverlayFS**：lower（只读）+ upper（可写）+ work
- **敏感路径保护**：不挂载 /etc/shadow、/root 等

### 系统调用过滤

- **Seccomp BPF**：白名单机制，强制拦截 clone3
- **默认拒绝**：未明确允许的系统调用被拒绝

### 网络隔离

- **SOCKS5 代理**：容器内流量通过代理出站
- **端口转发**：通过 SO_PEERCRED 鉴权

### 控制面鉴权

- **Unix Socket**：MCP Server 仅监听本地
- **SO_PEERCRED**：强制验证客户端 UID

## 性能设计

### 二进制优化

- **静态链接**：单一二进制，无外部依赖
- **体积控制**：≤ 20MB
- **启动优化**：快速初始化，按需加载

### 存储优化

- **硬链接**：同设备 fork 使用硬链接
- **物理拷贝**：跨设备降级为拷贝
- **缓存复用**：rootfs 缓存，避免重复解压

### 资源限制

- **内存**：cgroup v2 内存限制
- **CPU**：cgroup v2 CPU 配额
- **PIDs**：cgroup v2 PID 限制

## 错误处理

### 分级策略

- **Fatal**：立即终止，状态熔断，记录详细日志
- **Error**：终止当前操作，返回错误，不改变整体状态
- **Warn**：记录日志，继续执行

### 收敛保证

- 所有操作带超时
- 所有进程派生带超时控制
- 所有 I/O 带超时控制
- 状态机强制进入确定性终态

## 扩展点

### 容器后端

实现 `Container` 接口即可添加新后端。

### 网络策略

通过 `ContainerSpec.Ports` 配置端口映射，可替换转发实现。

### 日志输出

实现 `Logger` 接口即可替换日志后端。

### 存储驱动

通过 `storage.Store` 接口可替换存储实现。
