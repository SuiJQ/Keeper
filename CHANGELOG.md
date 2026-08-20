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

### 已知问题

- 当前环境无法运行真实 bwrap 容器（内核 5.10 未开启 CONFIG_OVERLAY_FS_USERNS）
- KubeVirt 方案不可行（无 /dev/kvm、无 K8s 集群创建 CRD 权限）
- 集成测试必须通过 GitHub Actions（Ubuntu 24.04 运行器）验证

---

*最后更新：2026-08-20*
