# Mirage v4.0 技术规格书 — 深度分析与项目初始化报告

## 1. 产品核心理解

Mirage 是一个为 AI Agent 设计的**轻量级持久化 Linux 运行时环境**，核心价值主张：
- **单一静态二进制**（≤20MB），内置 bubblewrap，宿主机零依赖
- **关机不丢数据**：OverlayFS + 持久化工作区
- **网络出站隔离**：容器内 SOCKS5 代理
- **结构化控制接口**：内置 MCP Server（抽象 Unix Socket + SO_PEERCRED 鉴权）

### 1.1 设计哲学：收敛性（Convergence）
所有异常状态必须在**有限步骤内**进入**确定性终态**（Running / Stopped / Fatal_*），且操作永不阻塞。这要求：
- 所有 I/O 操作带超时
- 所有进程派生带 PGID 管道确定性获取（3秒超时）
- 看门狗固定 60 秒超时，触发强制 umount + 状态熔断

## 2. 架构分层解析

### 2.1 三层进程模型
```
┌─────────────────────────────────────────┐
│  宿主机主进程（CLI 入口）                 │
│  ├── 环境探测（UserNS + OverlayFS Dry-Run）│
│  ├── 端口预绑定检查                       │
│  ├── 自衍生监控进程（Setpgid=true）        │
│  ├── 管道读取 PGID（3s 超时）             │
│  └── 看门狗协程（LockOSThread，60s 固定）  │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│  监控进程（PID 1  inside container）      │
│  ├── epoll + signalfd 事件循环            │
│  ├── 高优队列：health/status/stop         │
│  ├── 通用池：exec/download                │
│  ├── SOCKS5 代理（127.0.0.1:1080）        │
│  ├── inotify DNS 动态感知                 │
│  └── 僵尸回收（waitpid -1, WNOHANG）      │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│  Agent 用户进程（容器内）                 │
│  └── 通过 MCP Socket 与外部通信           │
└─────────────────────────────────────────┘
```

### 2.2 存储隔离架构
```
MIRAGE_HOME (~/.local/share/mirage/)
├── cache/rootfs/{sha256+ETag+timestamp}/
│   └── rootfs/                    # 只读基础镜像
└── agents/{name}/
    ├── meta.json                  # 状态机持久化
    ├── rootfs/                    # 硬链接或物理拷贝（同设备）
    ├── upper/                     # OverlayFS 可写层
    ├── work/                      # OverlayFS 工作目录（原子重建）
    ├── workspace/                 # 宿主机绑定挂载点
    ├── logs/                      # 磁盘日志
    └── .api_sock / .forward_sock  # Unix Socket 抽象命名空间
```

## 3. 关键技术点拆解

### 3.1 OverlayFS + UserNS 准入探测（Step 0）
- 先 `unshare(CLONE_NEWUSER)` 测试 UserNS
- 再在 `cache/` 下创建临时目录做 bwrap --overlay dry-run
- 失败立即终止，状态置 `fatal_unsupported_kernel`
- **注意**：当前内核 5.10.134 可能缺少 `CONFIG_OVERLAY_FS_USERNS`（需要 5.11+）

### 3.2 PGID 管道确定性获取
```go
ctx, cancel := context.WithTimeout(3*time.Second)
pgidChan := make(chan []byte, 1)
go func() { pgidBytes, _ := io.ReadAll(pr); pgidChan <- pgidBytes }()
select {
case pgidBytes := <-pgidChan: /* success */
case <-ctx.Done(): cmd.Process.Kill(); return error
}
```
**关键点**：父进程通过 `Setpgid: true` 让子进程成为新进程组组长，子进程在容器内写入自己的 PGID 到管道。

### 3.3 看门狗强制收敛
- 独立 OS 线程（`runtime.LockOSThread()`）
- 固定 60 秒超时（移除 GC 动态计算）
- 超时动作：
  1. `runtime.GC()`（排除 GC 死锁）
  2. `syscall.Kill(-pgid, SIGKILL)`
  3. `umount -l ${upper}`（物理隔离）
  4. 状态熔断为 `fatal_d_state`
  5. 删除 pgid + .watchdog，保留 upper/work 用于诊断

### 3.4 原子操作模式
| 场景 | 技术 | 说明 |
|------|------|------|
| work 目录重建 | 重命名隔离法 | `mv work work.purge && mkdir work && timeout 5 rm -rf work.purge` |
| 元数据写入 | AtomicWriteFile | tmpfile + fsync + rename |
| 回滚交换 | renameat2(RENAME_EXCHANGE) | 要求同设备 |
| 缓存清理 | 共享锁 + 活跃 key 扫描 | 仅删除未被任何 meta.json 引用的缓存 |

### 3.5 Seccomp BPF 白名单
- 基础：Go runtime + glibc 依赖
- 显式追加：`getrandom`、`gettid`、`sched_yield`、`rseq`
- **强制拦截**：`clone3`（syscall 435）→ ENOSYS
- ioctl 白名单：TIOCGWINSZ、TCGETS、TCSETS 等终端/IO 相关
- **明确拒绝**：TIOCSTI（0x5412）— 防止终端注入

### 3.6 MCP 鉴权：SO_PEERCRED
- 抽象 Unix Socket（Linux 抽象命名空间，非文件系统路径）
- accept 后立即 `getsockopt(SO_PEERCRED)` 获取对端 UID
- 不匹配直接 `conn.Close()`，不返回任何 HTTP 错误码，不解析任何指令
- 审计日志记录 PID/UID

## 4. 依赖清单与安装状态

### 4.1 已安装依赖
| 依赖 | 版本 | 状态 | 用途 |
|------|------|------|------|
| Go | 1.21.13 | ✅ 已安装 | 编译 |
| Bubblewrap | 0.9.0 | ✅ 已编译安装 | 容器隔离 |
| strace | 6.1 | ✅ 已安装 | CI 系统调用验证 |
| libseccomp-dev | 2.5.4 | ✅ 已安装 | Seccomp BPF 编译 |
| libcap-dev | 2.66 | ✅ 已安装 | 能力管理 |
| meson | 1.0.1 | ✅ 已安装 | bubblewrap 构建 |
| ninja-build | 1.11.1 | ✅ 已安装 | 构建加速 |

### 4.2 内核要求检查
| 检查项 | 要求 | 当前 | 状态 |
|--------|------|------|------|
| 内核版本 | ≥5.11 | 5.10.134 | ⚠️ 略低 |
| OverlayFS | 支持 | 支持 | ✅ |
| CONFIG_OVERLAY_FS_USERNS | 必须开启 | 未知（无法读取 config） | ⚠️ 需运行时验证 |
| UserNS | 支持 | 支持（unshare 测试通过） | ✅ |
| renameat2 | 支持 | 假设支持 | ✅ |

### 4.3 潜在风险
1. **内核版本 5.10.134 < 5.11**：规格书明确要求 5.11+ 以匹配 userxattr 支持。但运行时 Dry-Run 是唯一真理源，实际启动时会自动检测。
2. **CONFIG_OVERLAY_FS_USERNS**：当前无法通过 `/proc/config.gz` 确认（可能 gzip 未编译进内核）。需要运行时 probeOverlayFS() 验证。

## 5. 项目目录结构规划

```
mirage/
├── cmd/
│   ├── mirage/                    # CLI 入口
│   │   └── main.go
│   └── monitord/                  # 容器内 PID1 监控进程
│       └── main.go
├── internal/
│   ├── agent/                     # Agent 生命周期管理
│   ├── container/                 # bwrap 封装
│   ├── overlay/                   # OverlayFS 操作
│   ├── seccomp/                   # Seccomp BPF 生成
│   ├── mcp/                       # MCP Server + SO_PEERCRED
│   ├── network/                   # SOCKS5 + 端口转发
│   ├── snapshot/                  # 快照管理
│   ├── cache/                     # rootfs 缓存管理
│   └── watchdog/                  # 看门狗
├── pkg/
│   ├── config/                    # 配置解析
│   └── proto/                     # 协议定义（如有）
├── embedded/                      # 内嵌资源
│   ├── bwrap/                     # bwrap 二进制（≥0.9.0）
│   └── seccomp.bpf                # Seccomp BPF 字节码
├── go.mod
├── go.sum
└── README.md
```

## 6. 开发优先级与建议

### Phase 1：基础设施验证
1. **环境探测模块**：UserNS + OverlayFS Dry-Run（规格书 §3.1 Step 0）
2. **bwrap 执行降级**：memfd_create → 备用路径 → 熔断（§12.2）
3. **PGID 管道获取**：3秒超时 + 确定性写入（附录 A）

### Phase 2：核心生命周期
1. **create**：缓存 + 同设备硬链接/跨设备拷贝 + ENOSPC 熔断
2. **start**：端口预绑定 → 前置清理 → DNS 生成 → work 重建 → 衍生监控 → 看门狗
3. **stop**：SIGTERM → 5s → SIGKILL → 取消转发 → 清理 pgid

### Phase 3：高级特性
1. **MCP 控制面**：抽象 Unix Socket + SO_PEERCRED 鉴权
2. **Seccomp BPF**：clone3 拦截 + ioctl 白名单
3. **快照与回滚**：renameat2(RENAME_EXCHANGE) + 同设备预检
4. **网络子系统**：SOCKS5 + FD 传递 + 动态空闲超时

## 7. 下一步行动

1. **运行时验证**：在当前环境执行 bwrap dry-run 确认 OverlayFS+UserNS 可用性
2. **初始化 Go 模块**：`go mod init mirage` 并创建基础目录结构
3. **实现环境探测**：作为第一个可运行的功能模块
4. **CI 准备**：配置 Ubuntu 20.04/22.04/24.04 + Alpine 交叉编译验证

---

*分析完成时间：2026-08-19*
*基于规格书：Mirage 个人持久版 v4.0 —— 完整技术规格书（收敛交付版）*
