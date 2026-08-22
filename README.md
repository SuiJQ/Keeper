# Keeper

**Keeper** 为 AI Agent 提供持久化、隔离、轻量级 Linux 运行时环境。

> 项目已正式命名为 **Keeper**（原 Mirage），所有代码、文档、目录、二进制输出均统一使用该命名。

## 核心特性

- **单一静态二进制**（≤20MB），内置 bubblewrap，宿主机零依赖
- **关机不丢数据**：OverlayFS + 持久化工作区
- **网络出站隔离**：容器内 SOCKS5 代理
- **结构化控制接口**：内置 MCP Server（JSON-RPC 2.0 over Unix Socket）
- **收敛性设计**：所有异常进入确定性终态，操作永不阻塞

## 当前状态

- ✅ CLI 命令：`create` / `start` / `stop` / `status` / `destroy` / `recover` / `list` / `inspect` / `fork` / `cp`
- ✅ 存储层：OverlayFS 目录管理、快照、回滚、缓存清理
- ✅ 容器运行时：bwrap 后端框架（Exec/Status/Stop）
- ✅ 环境探测：内核 CONFIG_OVERLAY_FS_USERNS、bwrap、Seccomp 检测
- ✅ 看门狗：60s 超时收敛、D 态检测、优雅/强制停止
- ✅ MCP Server：JSON-RPC 2.0 协议、工具注册、CLI 路由
- ✅ 测试覆盖：storage / bootstrap / container / mcp / watchdog / cmd/keeper
- ✅ CI：GitHub Actions 多发行版矩阵（Ubuntu 20.04/22.04/24.04 + Alpine）

## 设计原则

1. **收敛性优先**：所有异常在有限步骤内进入确定性终态（Running / Stopped / Fatal_*）
2. **可维护性**：模块边界清晰，接口抽象，拒绝屎山代码
3. **可监测性**：结构化日志，明确错误码，状态变更可追溯
4. **可扩展性**：后端抽象（bwrap / kubevirt / mock），插件化策略
5. **永不阻塞**：所有 I/O 带超时，所有进程派生带超时控制

## 快速开始

### 环境要求

- Linux 内核 ≥ 5.11（推荐 6.x）
- 开启 `CONFIG_OVERLAY_FS_USERNS`
- 支持 UserNS、OverlayFS、renameat2

### 构建

```bash
# 构建二进制
make build

# 运行测试
make test
```

### 使用

```bash
# 创建 Agent
./bin/keeper create my-agent

# 启动 Agent
./bin/keeper start my-agent

# 查询所有 Agent
./bin/keeper list

# 查看 Agent 详情
./bin/keeper inspect my-agent
./bin/keeper inspect my-agent --verbose

# 停止 Agent
./bin/keeper stop my-agent

# 克隆 Agent
./bin/keeper fork my-agent my-agent-copy

# 复制文件（宿主机 ↔ Agent）
./bin/keeper cp ./model.bin my-agent:/workspace/models/
./bin/keeper cp -r ./data my-agent:/workspace/
./bin/keeper cp my-agent:/workspace/result.txt ./

# 销毁 Agent（删除所有数据）
./bin/keeper destroy my-agent
```

## 配置

Keeper 支持通过 `config.json` 进行运行时配置，配置文件位于 Keeper 数据目录（默认 `~/.local/share/keeper/config.json`）。首次运行会自动生成默认配置。

### 配置项说明

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `log_level` | string | `info` | 日志级别：debug/info/warn/error/fatal |
| `max_download_bytes` | int64 | `1073741824` | 最大下载字节数（1GB） |
| `default_shm_size_mb` | int | `64` | 默认共享内存大小（MB） |
| `download_timeout` | string | `5m` | 下载超时时间（Go duration 格式） |
| `watchdog_timeout` | string | `60s` | 看门狗超时时间 |
| `watchdog_check_interval` | string | `5s` | 看门狗检查间隔 |
| `mcp_allowed_uids` | []uint32 | `[]` | MCP Server 允许的 UID 白名单 |
| `mcp_allowed_gids` | []uint32 | `[]` | MCP Server 允许的 GID 白名单 |
| `seccomp_strategy` | string | `default` | Seccomp 策略：default/whitelist/blacklist/allow_all |
| `overlay_strategy` | string | `default` | OverlayFS 策略：default/overlayfs |
| `snapshot_compression_level` | int | `6` | 快照压缩级别（1-9） |
| `network_forward_max_connections` | int | `0` | 每个端口转发的最大并发连接数（0=不限制） |
| `network_forward_connect_timeout` | string | `5s` | 端口转发连接超时时间 |
| `downloader_threads` | int | `4` | 下载器默认线程数 |
| `downloader_chunk_size` | int64 | `1048576` | 下载器分块大小（字节） |
| `downloader_retry_delay` | string | `100ms` | 下载器重试间隔 |
| `storage_max_snapshots` | int | `0` | 每个 Agent 保留的最大快照数（0=不限制） |
| `storage_prune_interval` | string | `1h` | 存储清理间隔 |
| `bwrap_enable_userns` | bool | `true` | 是否启用 UserNS |
| `bwrap_enable_seccomp` | bool | `true` | 是否启用 Seccomp |
| `metrics_enabled` | bool | `true` | 是否启用 metrics server |
| `metrics_listen_addr` | string | `:9090` | metrics server 监听地址 |

### 热加载

配置文件支持热加载：修改 `config.json` 后，Keeper 会在下次检查周期自动重载配置。部分配置（如 `watchdog_timeout`、`mcp_allowed_uids`）支持动态生效。

### 示例配置

```json
{
  "log_level": "info",
  "default_shm_size_mb": 64,
  "watchdog_timeout": "60s",
  "network_forward_max_connections": 10,
  "downloader_threads": 4,
  "storage_max_snapshots": 10,
  "metrics_enabled": true,
  "metrics_listen_addr": ":9090"
}
```

## 项目结构

```
keeper/
├── cmd/
│   └── keeper/           # CLI 入口
├── internal/
│   ├── agent/            # Agent 生命周期管理
│   ├── container/        # 容器运行时抽象
│   │   └── bwrap.go      # bubblewrap 后端
│   ├── storage/          # 存储管理（OverlayFS、快照）
│   ├── mcp/              # MCP 控制面
│   ├── watchdog/         # 看门狗机制
│   ├── bootstrap/        # 启动与初始化
│   │   └── probe.go      # 环境探测
│   ├── log/              # 结构化日志
│   └── errors/           # 错误定义
├── pkg/
│   └── config/           # 配置管理
├── .github/
│   └── workflows/
│       └── ci.yml        # GitHub Actions CI
└── PROGRESS_REPORT.md    # 进度汇报
```

## 架构设计

### 核心接口

- **Container**：容器运行时抽象（bwrap / kubevirt / mock）
- **StateMachine**：严格的状态机，防止非法状态转换
- **Logger**：结构化日志接口，支持 JSON 输出
- **KeeperError**：分级错误体系，明确错误码和恢复策略

### 状态机

```
created ──start──→ running ──stop──→ stopped
   ↑                  │
   │                  ├── watchdog_timeout ──→ fatal_d_state
   │                  ├── probe_fail ──→ fatal_unsupported_kernel
   │                  ├── bwrap_fail ──→ fatal_bwrap_exec
   │                  └── no_space ──→ fatal_no_space
   │
   └── recover ←── recover ←── recover ←── recover
```

### 错误处理策略

- **Fatal**：立即终止，状态熔断，记录详细日志
- **Error**：终止当前操作，返回错误，不改变整体状态
- **Warn**：记录日志，继续执行
- **所有错误显式汇报**，禁止静默失败

## 开发

### 前置要求

- Go 1.21+
- Make
- golangci-lint（可选）

### 构建

```bash
make build
```

### 测试

```bash
# 单元测试
make test

# 查看覆盖率
make test-cover
```

### 代码检查

```bash
make lint
make fmt
make vet
```

## 测试策略

### 分层测试

1. **单元测试**：Mock Container，纯逻辑测试
2. **集成测试**：GitHub Actions（真实内核环境）
3. **系统测试**：完整启动/停止/快照流程

### CI 矩阵

- Ubuntu 20.04 (内核 5.4)
- Ubuntu 22.04 (内核 5.15)
- Ubuntu 24.04 (内核 6.x)
- Alpine Linux
- Go 1.21

## 安全

- **Seccomp BPF**：系统调用白名单，强制拦截 clone3
- **Unix Socket 鉴权**：MCP Socket 使用 SO_PEERCRED 验证 UID
- **OverlayFS 隔离**：Upper/Work 目录隔离，无敏感路径挂载
- **收敛性设计**：所有异常进入确定性终态，杜绝不确定恢复

## 性能指标

| 指标 | 标准 |
|------|------|
| 二进制大小 | ≤ 20 MB |
| 启动耗时 | ≤ 0.5 秒 |
| 外部依赖 | 宿主机无需任何软件 |
| OverlayFS 探测 | 不支持时立即 fatal_unsupported_kernel |
| D 状态熔断 | umount -l 后置 fatal_d_state |

## 文档

- [进度汇报](PROGRESS_REPORT.md)
- [架构设计](ARCHITECTURE.md)
- [环境研究报告](ENVIRONMENT_RESEARCH.md)
- [KubeVirt 可行性分析](KUBEVIRT_RESEARCH.md)
- [运行时验证报告](RUNTIME_VERIFICATION.md)

## 许可证

MIT

---

**注意**：本项目当前处于早期开发阶段（v0.1.0-dev），API 可能随时变更。
