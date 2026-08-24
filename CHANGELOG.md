# Changelog

所有项目变更记录在此文件中。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/)。
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [0.1.0-dev] - 2026-08-20

### 已实现

#### CLI 命令
- `create`：创建 Agent（含名称验证）
- `start`：启动 Agent（占位实现）
- `stop`：停止 Agent（占位实现）
- `status`：查询 Agent 状态（占位实现）
- `destroy`：销毁 Agent（含存储清理）
- `recover`：恢复 Agent（占位实现）
- `list`：列出所有 Agent（JSON/表格格式）
- `inspect`：查看 Agent 详情（支持 `--verbose`）
- `fork`：克隆 Agent（硬链接/物理拷贝）
- `cp`：复制文件（宿主机 ↔ Agent）

#### 存储层
- OverlayFS 目录管理
- 快照创建/回滚
- 缓存清理
- 硬链接/物理拷贝

#### 容器运行时
- bwrap 后端框架
- Exec/Status/Stop 方法
- 环境探测（内核、bwrap、Seccomp）

#### 监控与维护
- 看门狗机制（60s 超时）
- D 态检测
- 结构化日志

#### MCP Server
- JSON-RPC 2.0 协议
- 工具注册
- CLI 路由

#### 测试
- storage 包：12 个测试用例
- bootstrap 包：9 个测试用例
- container 包：18 个测试用例（含 mock 后端）
- mcp 包：12 个测试用例
- watchdog 包：9 个测试用例
- cmd/keeper：18 个测试用例

#### CI/CD
- GitHub Actions 多发行版矩阵
- Ubuntu 20.04/22.04/24.04 + Alpine
- Go 1.21

#### 文档
- README.md（项目介绍、使用指南）
- ARCHITECTURE.md（架构设计）
- SECURITY.md（安全设计）
- CONTRIBUTING.md（贡献指南）
- PROGRESS_REPORT.md（进度汇报）
- ENVIRONMENT_RESEARCH.md（环境研究报告）
- KUBEVIRT_RESEARCH.md（KubeVirt 可行性分析）
- RUNTIME_VERIFICATION.md（运行时验证报告）

### 待实现

#### 核心功能
- [ ] bwrap 后端完整实现（OverlayFS 挂载、网络隔离）
- [ ] 环境探测模块（CONFIG_OVERLAY_FS_USERNS 检测）
- [ ] MCP Server socket 权限控制（SO_PEERCRED）
- [ ] 网络子系统（SOCKS5 代理、端口转发）

#### 命令
- [ ] `start` 实际启动逻辑
- [ ] `stop` 实际停止逻辑
- [ ] `status` 实际状态查询逻辑
- [ ] `recover` 实际恢复逻辑
- [ ] `snapshot` 快照逻辑
- [ ] `rollback` 回滚逻辑

#### 测试
- [ ] 集成测试（GitHub Actions 真实环境）
- [ ] 网络模块测试
- [ ] 系统测试（完整启动/停止流程）

#### 文档
- [ ] API 文档（GoDoc）
- [ ] 用户手册
- [ ] 部署指南

## [0.1.0] - 2026-08-24

### 已实现

#### CLI 命令
- `create`：创建 Agent（含名称验证）
- `start`：启动 Agent（bwrap 容器生命周期集成）
- `stop`：停止 Agent（优雅关闭 + 策略清理）
- `status`：查询 Agent 状态（JSON/表格格式）
- `destroy`：销毁 Agent（含存储清理）
- `recover`：恢复 Agent（状态重置 + 残留进程清理）
- `list`：列出所有 Agent
- `inspect`：查看 Agent 详情（支持 `--verbose`）
- `fork`：克隆 Agent（硬链接/物理拷贝）
- `cp`：复制文件（宿主机 ↔ Agent，支持递归目录）
- `snapshot`：创建快照
- `rollback`：回滚快照
- `version`：显示版本信息
- `help`：显示帮助信息

#### 存储层
- OverlayFS 目录管理（rootfs/upper/work/workspace）
- 快照创建/回滚
- 缓存清理
- 硬链接/物理拷贝
- Agent 元数据管理（state/PID/PGID/error）

#### 容器运行时
- bwrap 后端完整实现（OverlayFS 挂载、网络隔离、seccomp）
- MockContainer 双实现（状态机、Exec 记录、错误注入）
- 全局容器注册表（container.Register/Get/Unregister）
- 策略模式（seccomp/overlay/snapshot compression）
- 环境探测（内核、bwrap、Seccomp、CONFIG_OVERLAY_FS_USERNS）

#### 监控与维护
- 看门狗机制（可配置超时 + 检查间隔）
- D 态检测
- 结构化日志（全局 logger + 字段化）
- 指标暴露（Prometheus 格式，可选启用）

#### MCP Server
- JSON-RPC 2.0 协议
- 工具注册（create/start/stop/status/destroy/recover/list/inspect/fork/cp/snapshot/rollback）
- SO_PEERCRED 鉴权（UID/GID allowlist）
- Unix socket 权限控制
- CLI 路由

#### 网络代理
- SOCKS5 代理服务器（RFC 1929 认证、IPv4/IPv6/Domain）
- 端口转发服务器（连接追踪、优雅关闭）
- 网络集成测试

#### 配置管理
- 热加载（ReloadIfChanged + OnReload 回调）
- 完整配置项（shm_size、download、watchdog、mcp、seccomp、overlay、metrics、network）
- 默认配置 + 用户配置合并

#### 测试
- 整体覆盖率 ~78.7%，`cmd/keeper` 80.0%
- storage 包：12+ 测试用例
- bootstrap 包：9+ 测试用例
- container 包：28+ 测试用例（含 mock 后端 + bwrap 集成测试）
- mcp 包：12+ 测试用例
- watchdog 包：9+ 测试用例
- network 包：12+ 测试用例
- cmd/keeper：40+ 测试用例
- 集成测试：8 个端到端场景
- 基准测试：配置加载、存储操作、启动延迟

#### CI/CD
- GitHub Actions 多发行版矩阵
- Ubuntu 24.04 + Go build cache
- Test / Lint / Build (amd64/arm64) / Kernel-compatibility / Bootstrap / bwrap-integration
- bwrap 验证策略：Mock 覆盖状态机 + 真实 bwrap 按内核条件跳过

#### 文档
- README.md（项目介绍、使用指南）
- API.md（API 参考）
- RELEASE_CHECKLIST.md（发布检查清单）
- ARCHITECTURE.md（架构设计）
- SECURITY.md（安全设计）
- CONTRIBUTING.md（贡献指南）
- examples/（basic_usage.sh、network_proxy.sh、snapshot_rollback.sh）
- benchmarks/（README.md、startup_bench_test.go）

### 已知问题

- 真实 bwrap 完整生命周期依赖内核 `CONFIG_OVERLAY_FS_USERNS=y`，当前 GitHub-hosted runner 和本地环境均未开启，完整生命周期集成测试在 CI 中条件跳过
- 自托管 runner 或具备该内核配置的专用环境可验证真实 bwrap 行为

---

## [0.1.0-dev] - 2026-08-20

### 已实现

#### CLI 命令
- `create`：创建 Agent（含名称验证）
- `start`：启动 Agent（占位实现）
- `stop`：停止 Agent（占位实现）
- `status`：查询 Agent 状态（占位实现）
- `destroy`：销毁 Agent（含存储清理）
- `recover`：恢复 Agent（占位实现）
- `list`：列出所有 Agent（JSON/表格格式）
- `inspect`：查看 Agent 详情（支持 `--verbose`）
- `fork`：克隆 Agent（硬链接/物理拷贝）
- `cp`：复制文件（宿主机 ↔ Agent）

#### 存储层
- OverlayFS 目录管理
- 快照创建/回滚
- 缓存清理
- 硬链接/物理拷贝

#### 容器运行时
- bwrap 后端框架
- Exec/Status/Stop 方法
- 环境探测（内核、bwrap、Seccomp）

#### 监控与维护
- 看门狗机制（60s 超时）
- D 态检测
- 结构化日志

#### MCP Server
- JSON-RPC 2.0 协议
- 工具注册
- CLI 路由

#### 测试
- storage 包：12 个测试用例
- bootstrap 包：9 个测试用例
- container 包：18 个测试用例（含 mock 后端）
- mcp 包：12 个测试用例
- watchdog 包：9 个测试用例
- cmd/keeper：18 个测试用例

#### CI/CD
- GitHub Actions 多发行版矩阵
- Ubuntu 20.04/22.04/24.04 + Alpine
- Go 1.21

#### 文档
- README.md（项目介绍、使用指南）
- ARCHITECTURE.md（架构设计）
- SECURITY.md（安全设计）
- CONTRIBUTING.md（贡献指南）
- PROGRESS_REPORT.md（进度汇报）
- ENVIRONMENT_RESEARCH.md（环境研究报告）
- KUBEVIRT_RESEARCH.md（KubeVirt 可行性分析）
- RUNTIME_VERIFICATION.md（运行时验证报告）

### 待实现

#### 核心功能
- [x] bwrap 后端完整实现（OverlayFS 挂载、网络隔离）
- [x] 环境探测模块（CONFIG_OVERLAY_FS_USERNS 检测）
- [x] MCP Server socket 权限控制（SO_PEERCRED）
- [x] 网络子系统（SOCKS5 代理、端口转发）

#### 命令
- [x] `start` 实际启动逻辑
- [x] `stop` 实际停止逻辑
- [x] `status` 实际状态查询逻辑
- [x] `recover` 实际恢复逻辑
- [x] `snapshot` 快照逻辑
- [x] `rollback` 回滚逻辑

#### 测试
- [x] 集成测试（GitHub Actions 真实环境）
- [x] 网络模块测试
- [x] 系统测试（完整启动/停止流程）

#### 文档
- [x] API 文档（GoDoc）
- [x] 用户手册
- [x] 部署指南

### 已知问题

- 当前环境无法运行真实 bwrap 容器（内核 5.10 未开启 CONFIG_OVERLAY_FS_USERNS）
- KubeVirt 方案不可行（无 /dev/kvm、无 K8s 集群创建 CRD 权限）
- 集成测试必须通过 GitHub Actions（Ubuntu 24.04 运行器）验证

---

*最后更新：2026-08-24*
