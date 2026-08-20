# Keeper 朋友建议评估报告

## 评估摘要

朋友的建议整体质量很高，既有清晰的场景驱动，也有对规格书的深入理解。以下逐条评估：

---

## 1. 强烈建议新增的命令

### ① `list` —— 总览命令 ⭐⭐⭐⭐⭐

**合理性**：✅ **极其合理，立即实现**

**理由**：
- 当前只有 `status`（单个），缺少全局视角
- 运维基础需求：一眼看清所有 Agent 状态、端口、运行时间
- 已有 `ListAgents` 接口，实现成本低
- 与 `docker ps`、`kubectl get pods` 等工具心智模型一致

**命名确认**：`keeper list`

**实现要点**：
```go
// 输出格式
NAME     STATE     PORTS          UPTIME
web      running   8080:7860      2h
db       stopped   -              -
worker   created   -              -
```

---

### ② `inspect`（或 `show`）—— 深度调试命令 ⭐⭐⭐⭐⭐

**合理性**：✅ **极其合理，高优先级**

**理由**：
- 直接解决规格书中的调试痛点（st_dev、pgid、cache_key、overlay 路径）
- 纯静态解析，不需要启动 Agent，安全无副作用
- 对排查 `fatal_d_state`、`跨设备回滚拒绝` 至关重要
- 与 `docker inspect` 心智模型一致

**命名决策**：
- 主命令：`keeper inspect <name>`
- 备选：`keeper show <name>`（更简洁）
- **推荐 `inspect`**：更符合 Docker/K8s 生态习惯

**实现要点**：
```bash
# 基本输出
NAME: web
STATE: running
PID: 12345
PGID: 12345
CREATED: 2026-08-20T00:00:00Z

# 设备信息（--verbose）
Rootfs Device ID: 0x801
Upper Device ID: 0x801 (Same: true)
Work Device ID: 0x801
Workspace Device ID: 0x801

# 缓存信息
Cache Key: abc123
Cache URL: https://example.com/rootfs.tar.gz

# 网络
Ports: [{"host":8080,"container":7860}]
```

---

### ③ `cp` —— 宿主机 ↔ 容器文件互传 ⭐⭐⭐⭐

**合理性**：✅ **合理，中优先级**

**理由**：
- 填补 `exec` 之外的空白，用户友好
- 本质是操作 `agents/{name}/workspace/` 目录，实现简单
- AI Agent 场景确实有频繁的文件交换需求（模型权重、日志）

**注意**：
- 由于 workspace 已经是 bind mount 的宿主机目录，用户**可以直接**用 `cp` 访问
- `keeper cp` 命令只是提供了一层语义封装和跨平台一致性
- 需要决定：是否支持目录递归、是否保留权限

**命名确认**：`keeper cp`

**实现要点**：
```bash
# 上传（宿主机 → 容器 workspace）
keeper cp ./model.bin web:/workspace/models/

# 下载（容器 workspace → 宿主机）
keeper cp web:/workspace/output.log ./logs/

# 目录递归
keeper cp -r ./data/ web:/workspace/data/
```

---

## 2. 可以暂缓或不需要的命令

### `update` / `config` —— 不建议实现 ⭐⭐⭐⭐⭐

**朋友的评估完全正确**

**理由**：
- 规格书明确要求"端口映射生效需下次 start"
- `shm_size` 依赖 bwrap `--tmpfs` 参数，无法热更新
- 部分参数需重启、部分无需，语义混乱
- 违反"原子性"原则

**建议**：文档中明确说明"不支持运行时配置修改"，引导用户通过 `stop` → 编辑 `meta.json` → `start`。

---

## 3. 绝对禁止的命令

### `pause` —— 致命毒药 ⭐⭐⭐⭐⭐

**朋友的警告极其重要**

**理由**：
- 与 §3.3 看门狗机制直接冲突
- `pause` 冻结进程 → `.watchdog` 停止更新 → 60s 后触发 `fatal_d_state`
- 可能导致用户误操作引发"宿主机重启"级别的灾难
- 违反规格书"操作永不阻塞"和"确定性终态"原则

**建议**：
1. **代码层面**：不实现 `pause` 命令
2. **文档层面**：显式声明"不支持 pause，会导致看门狗超时熔断"
3. **错误处理**：如果用户在容器内手动执行 `kill -STOP`，看门狗应记录 WARN 但不自杀

---

## 4. 命名统一说明

朋友建议中仍使用 `mirage` 作为命令名，需要统一改为 `keeper`：

| 朋友建议 | 实际命令 | 说明 |
|----------|----------|------|
| `mirage list` | `keeper list` | 总览命令 |
| `mirage inspect` | `keeper inspect` | 深度调试 |
| `mirage cp` | `keeper cp` | 文件互传 |

---

## 5. 实现优先级建议

### Phase 1（立即实现）
1. **`list`** - 填补运维盲区，已有接口
2. **`inspect`** - 调试必需，纯静态

### Phase 2（短期实现）
3. **`fork`** - 之前已评估，高优先级
4. **`cp`** - 用户体验优化

### Phase 3（后续增强）
- 更多网络/存储功能
- 性能优化

---

## 6. 技术实现注意事项

### `list` 命令
```go
func listAgents(ctx context.Context, store storage.Store) error {
    agents, err := store.ListAgents(ctx)
    if err != nil {
        return err
    }
    // 表格输出：NAME STATE PORTS UPTIME
    // 需要读取每个 Agent 的 ports.json 和 meta.json
}
```

### `inspect` 命令
```go
func inspectAgent(ctx context.Context, name string, verbose bool) error {
    meta, err := store.GetAgent(ctx, name)
    if err != nil {
        return err
    }
    
    // 基本字段
    printBasic(meta)
    
    if verbose {
        // 设备信息
        rootfsDev := statDevice(meta.RootfsDir)
        upperDev := statDevice(meta.UpperDir)
        sameDevice := rootfsDev == upperDev
        printDeviceInfo(rootfsDev, upperDev, sameDevice)
    }
}
```

### `cp` 命令
```go
func copyFile(src, dst string) error {
    // 解析 src/dst
    // 如果 src 是 "agent:/path"，从 workspace 读取
    // 如果 dst 是 "agent:/path"，写入 workspace
    // 否则直接文件操作
}
```

---

## 7. 结论

| 建议 | 决策 | 优先级 | 理由 |
|------|------|--------|------|
| `list` | ✅ 采纳 | P0 | 运维必需，填补盲区 |
| `inspect` | ✅ 采纳 | P0 | 调试必需，纯静态 |
| `cp` | ✅ 采纳 | P1 | 用户体验好，实现简单 |
| `update`/`config` | ❌ 拒绝 | - | 语义混乱，违反原子性 |
| `pause` | ❌ 绝对禁止 | - | 与看门狗机制冲突 |

**下一步**：如果确认，我将立即实现 `list` 和 `inspect` 命令。

---

*评估完成时间：2026-08-20*
*评估对象：朋友对 Keeper 命令集的建议*
