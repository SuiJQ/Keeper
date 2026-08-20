# Keeper Fork 命令评估与命名研究

## 1. 需求合理性评估

### 1.1 核心需求：Agent 克隆
**结论：合理且必要**

对于 AI Agent 场景，`fork` 确实是刚需：
- Agent 本质是**状态机**，调试好的 Agent A 需要克隆出 A/B 测试版本
- 文件系统状态（upper 层 + workspace）包含了完整的运行时状态
- 避免重复 `create` + 安装依赖的繁琐流程

**类比**：
- Git: `git clone` vs `git commit + git branch`
- 进程: `fork()` 创建进程副本
- 容器: `docker commit` + `docker run` vs `docker create` + 手动配置

### 1.2 与现有命令的关系
```
create  → 从零创建（空白 rootfs）
   ↓
start   → 启动运行
   ↓
stop    → 停止
   ↓
fork    → 克隆为新的 Agent（补全生命周期）
   ↓
destroy → 销毁
```

`fork` 恰好补全了生命周期的最后一块拼图，与 `snapshot`/`rollback` 形成互补：
- `snapshot`：备份与回滚（时间维度）
- `fork`：并行派生（空间维度）

## 2. 命名研究：Copy vs Fork

### 2.1 语义对比

| 维度 | Copy | Fork |
|------|------|------|
| **直观性** | ⭐⭐⭐⭐⭐ 直白 | ⭐⭐⭐ 需要理解 Unix 语义 |
| **精确性** | ⭐⭐⭐ 通用复制 | ⭐⭐⭐⭐⭐ 强调"派生"、"分支" |
| **Agent 语境** | ⭐⭐⭐ 文件复制 | ⭐⭐⭐⭐⭐ 进程/状态派生 |
| **Unix 传统** | ⭐⭐ cp 命令 | ⭐⭐⭐⭐⭐ fork() 系统调用 |
| **避免混淆** | ⭐⭐⭐⭐⭐ 无歧义 | ⭐⭐⭐ 可能联想到进程 fork |

### 2.2 用户体验分析

**Copy 的优势**：
- 新手友好，一看就懂
- 无认知负担

**Copy 的劣势**：
- 过于通用，丢失 Agent 语境
- 可能被误解为简单的文件复制（scp 式的）

**Fork 的优势**：
- 强调**派生关系**：Agent B 是从 Agent A 派生出来的
- 暗示**独立性**：fork 后的进程/Agent 是独立的
- 符合 Agent 作为"状态机"的语义
- 与 `create` 形成对比：create 是从零创建，fork 是从现有实例派生

**Fork 的劣势**：
- 需要文档解释（但这是好事，强制用户理解语义）
- 可能被认为暗示进程级别复制（实际上我们是文件系统级别）

### 2.3 竞品对比

| 工具 | 复制命令 | 语义 |
|------|----------|------|
| Git | `git clone` | 克隆仓库（代码 + 历史） |
| Docker | `docker commit` + `docker create` | 从镜像创建容器 |
| VM | `virt-clone` / `vagrant clone` | 克隆虚拟机 |
| Process | `fork()` | 创建进程副本 |
| Agent | `keeper fork` | 克隆 Agent 状态 |

**观察**：在虚拟化和容器领域，"clone" 或 "fork" 是常见选择，强调**完整状态的复制**。

### 2.4 推荐：`fork`

**理由**：
1. **语义精确**：在 Agent 生命周期中，`fork` 表示从现有 Agent 派生新 Agent
2. **符合 Unix 传统**：`fork()` 是进程创建的核心系统调用，用户有认知基础
3. **区分度**：与 `copy`（文件复制）、`snapshot`（备份）形成差异化
4. **可扩展性**：未来如果支持进程级隔离，`fork` 的语义也能覆盖

**折中方案**：
- 主命令：`keeper fork <source> <target>`
- 别名：`keeper clone <source> <target>`（兼容 Git 用户）
- 不推荐：`keeper copy`（太泛，丢失 Agent 语义）

## 3. 实现策略评估

### 3.1 前置校验（合理）

```go
// 源 Agent 必须处于 stopped 或 created 状态
if source.State != StateStopped && source.State != StateCreated {
    return &KeeperError{
        Code: ErrCodeAgent,
        Message: "cannot fork running agent, stop it first or use snapshot",
        Recoverable: true,
    }
}
```

**评估**：✅ 合理
- 防止 Dirty Copy（upper 层数据不一致）
- 引导用户使用 `snapshot` 作为替代方案
- 符合规格书"悲观探测"原则

### 3.2 存储引擎复用（合理）

```go
// 复用 rootfs 创建逻辑
func (s *Storage) Fork(source, target string) error {
    // 同设备：cp -al（硬链接）
    // 跨设备：cp -pRL（物理拷贝）
    // ENOSPC 熔断
}
```

**评估**：✅ 合理
- 复用现有代码，避免重复
- 保持原子性保证
- 继承 ENOSPC 熔断机制

**注意**：需要复制三个目录：
- `agents/{name}/rootfs` → `agents/{new_name}/rootfs`
- `agents/{name}/upper` → `agents/{new_name}/upper`
- `agents/{name}/workspace` → `agents/{new_name}/workspace`

### 3.3 运行时残留清理（关键！）

```go
// 必须删除的运行时文件
runtimeFiles := []string{
    "pgid",
    ".watchdog",
    ".api_sock",
    ".forward_sock",
}

// 必须重置的文件
resetFiles := []string{
    "ports.json",  // 默认清空，避免端口冲突
    "meta.json",   // 更新 name、state、created_at
}
```

**评估**：✅ **极其关键**
- 防止 Socket 文件冲突（抽象命名空间唯一）
- 防止端口占用（同一主机端口不能重复绑定）
- 防止 PID 残留导致信号发送错误

**增强建议**：
- `ports.json` 应强制清空，而非继承
- 如果用户想保留端口映射，应在 `start` 前通过 `port-map` 显式指定

### 3.4 元数据修改

```go
// 新 Agent 元数据
newMeta := &Meta{
    Name:       target,
    State:      StateCreated,  // 强制 created，而非 stopped
    CreatedAt:  time.Now(),
    PGID:       "",            // 清空
    CacheKey:   source.CacheKey, // 继承，避免缓存误删
    Ports:      []PortMapping{}, // 清空
}
```

**评估**：✅ 合理
- `state = created`：新 Agent 未经过内核探测，不应直接进入 stopped
- `cache_key` 继承：确保 `cache prune` 不会误删共享的 rootfs
- `pgid` 清空：无运行时进程

### 3.5 Work 目录处理（正确）

```go
// 不复制 work/ 目录，由 start 时重建
// 规格书 §3.1 Step 4：work 目录原子重建
```

**评估**：✅ 正确
- `work/` 是 OverlayFS 临时工作目录，包含临时文件
- `start` 时会自动执行"重命名隔离法"重建
- 避免复制临时垃圾文件

## 4. 潜在风险与缓解

### 4.1 风险：磁盘空间爆炸

**场景**：用户频繁 fork 大 Agent，每个副本占用完整磁盘空间

**缓解**：
- 同设备使用硬链接（cp -al），多个 Agent 共享 rootfs 页
- 提供 `keeper list` 显示每个 Agent 的磁盘占用
- 文档说明：fork 会占用额外空间

### 4.2 风险：缓存键冲突

**场景**：fork 后的 Agent 修改了 upper 层，导致 cache_key 失效

**缓解**：
- `cache_key` 继承源 Agent，表示底层 rootfs 相同
- 如果用户想更换 rootfs，使用 `migrate` 命令

### 4.3 风险：名称冲突

**场景**：目标 Agent 名称已存在

**缓解**：
```go
if _, err := os.Stat(targetDir); err == nil {
    return &KeeperError{
        Code: ErrCodeAgent,
        Message: fmt.Sprintf("agent '%s' already exists", target),
        Recoverable: false,
    }
}
```

### 4.4 风险：跨设备复制性能

**场景**：MIRAGE_HOME 跨设备，fork 时降级为物理拷贝

**缓解**：
- 记录 WARN 日志，告知用户性能影响
- 建议用户将 MIRAGE_HOME 挂载在高速设备

## 5. 与 Snapshot 的对比

| 维度 | Snapshot | Fork |
|------|----------|------|
| **目的** | 备份与回滚 | 并行派生 |
| **时间维度** | 时间点快照 | 空间克隆 |
| **使用场景** | 升级前备份、回滚 | A/B 测试、并发任务 |
| **操作频率** | 低（关键时刻） | 高（日常开发） |
| **磁盘开销** | 额外副本 | 额外副本 |
| **元数据** | 生成 snapshot_id | 新 Agent 完整元数据 |

**结论**：两者互补，不可互相替代。

## 6. 推荐实现方案

### 6.1 命令签名
```bash
keeper fork <source> <target>
```

### 6.2 执行流程
```
1. 参数验证（名称格式、source != target）
2. 源 Agent 存在性检查
3. 源 Agent 状态检查（stopped/created）
4. 目标 Agent 冲突检查（不能已存在）
5. 设备探测（源 rootfs/upper/workspace 是否同设备）
6. 存储复制：
   a. rootfs：cp -al 或 cp -pRL
   b. upper：cp -al 或 cp -pRL
   c. workspace：cp -al 或 cp -pRL
   d. 跳过 work/ 目录
7. 运行时文件清理（pgid、.watchdog、.api_sock、.forward_sock）
8. ports.json 重置为空
9. meta.json 原子写入（name、created_at、state=created、pgid=""、cache_key 继承）
10. 创建 backups/ 目录结构（空）
```

### 6.3 错误处理
- **源 Agent running**：返回 `ErrCodeAgent`，提示先 stop 或 snapshot
- **目标已存在**：返回 `ErrCodeAgent`，拒绝覆盖
- **ENOSPC**：返回 `ErrFatalNoSpace`，清理残留
- **跨设备且用户禁用检查**：降级为物理拷贝 + WARN 日志

### 6.4 日志规范
```json
{
    "level": "info",
    "stage": "fork",
    "agent_name": "target",
    "source": "source",
    "state": "created",
    "duration_ms": 150,
    "copy_mode": "hardlink"  // 或 "physical"
}
```

## 7. 结论

### 7.1 需求评估
**朋友的评估非常合理**，`fork` 命令是 Keeper 生命周期的必要补充。

### 7.2 命名决策
**推荐使用 `fork`**，理由：
1. 语义精确，强调 Agent 派生关系
2. 符合 Unix 传统和 Agent 状态机语义
3. 与 `create` 形成明确对比
4. 可扩展性强

**实现时可添加 `clone` 作为别名**，兼容 Git 用户习惯。

### 7.3 实现优先级
**高优先级**，建议在以下功能之后立即实现：
1. ✅ 基础框架（已完成）
2. ⏳ bwrap 后端
3. ⏳ 环境探测
4. ⏳ **fork 命令**

### 7.4 下一步
1. 实现 `internal/storage/fork.go`
2. 在 `cmd/keeper/main.go` 注册 `fork` 子命令
3. 编写单元测试（Mock 存储后端）
4. 在 GitHub Actions 中集成集成测试

---

*评估完成时间：2026-08-20*
*评估对象：Keeper Fork 命令设计与命名*
