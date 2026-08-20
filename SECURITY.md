# Keeper 安全设计

## 威胁模型

Keeper 为 AI Agent 提供隔离的 Linux 运行时环境，主要威胁包括：

1. **容器逃逸**：Agent 突破容器隔离，访问宿主机资源
2. **权限提升**：Agent 通过漏洞获取更高权限
3. **数据泄露**：Agent 访问其他 Agent 的数据
4. **资源耗尽**：Agent 消耗过多宿主资源
5. **控制面未授权访问**：未授权进程调用 MCP Server

## 安全机制

### 1. 命名空间隔离

Keeper 使用 Linux 命名空间实现进程隔离。

**UserNS**：
- 容器内以非 root 用户运行
- 使用 UserNS 映射 UID/GID
- 避免容器内 root 映射到宿主机 root

**PIDNS**：
- 独立的 PID 空间
- 容器内 PID 1 为宿主机进程的 PID 1
- 避免 PID 冲突

**NETNS**：
- 独立的网络命名空间
- 容器内网络与宿主机隔离
- 通过代理出站

**MountNS**：
- 独立的文件系统视图
- 通过 OverlayFS 实现可写层
- 敏感路径只读挂载

### 2. 文件系统隔离

**OverlayFS**：
```
lower (只读) + upper (可写) + work = merged
```

- lower：只读根文件系统
- upper：可写层，所有修改写入此处
- work：OverlayFS 内部工作目录
- merged：挂载点，容器看到的视图

**敏感路径保护**：
- `/etc/shadow`：只读挂载，避免密码修改
- `/root`：只读挂载，避免访问其他用户数据
- `/proc`：限制挂载，避免敏感信息泄露

### 3. 系统调用过滤

**Seccomp BPF**：
- 白名单机制，仅允许明确列出的系统调用
- 强制拦截 `clone3`（防止创建新命名空间）
- 拦截 `mount`（防止重新挂载文件系统）
- 拦截 `unshare`（防止脱离命名空间）

**默认拒绝策略**：
- 未明确允许的系统调用被拒绝
- 拒绝时返回 `EPERM` 错误

### 4. 网络隔离

**SOCKS5 代理**：
- 容器内所有出站流量通过代理
- 代理运行在宿主机，受策略控制
- 可限制目标地址、端口、协议

**端口转发**：
- 通过 SO_PEERCRED 鉴权
- 仅允许指定用户访问
- 防止端口占用和冲突

### 5. 控制面鉴权

**Unix Socket**：
- MCP Server 仅监听本地 Unix Socket
- 不监听 TCP 端口
- 避免网络攻击

**SO_PEERCRED**：
- 强制验证客户端 UID
- 仅允许指定用户调用 MCP
- 防止未授权访问

**Socket 权限**：
- 创建后设置 `0600` 权限
- 仅属主可读写
- 避免其他用户访问

### 6. 资源限制

**cgroup v2**：
- 内存限制：防止内存耗尽
- CPU 配额：防止 CPU 饥饿
- PID 限制：防止进程爆炸
- I/O 限制：防止磁盘滥用

**看门狗机制**：
- 60s 超时收敛
- D 态进程检测
- 优雅/强制停止

### 7. 数据保护

**Agent 隔离**：
- 每个 Agent 独立目录
- 默认权限 `0700`
- 无跨 Agent 访问

**敏感信息脱敏**：
- 日志中不记录密钥、Token
- 环境变量通过配置注入
- 避免硬编码凭证

## 安全配置

### 最小权限原则

- 容器内用户 UID 10000（非 0）
- 宿主机目录权限 `0700`
- Socket 权限 `0600`

### 安全审计

- 所有操作记录结构化日志
- 日志包含 UID、PID、时间戳
- 支持审计追踪

### 安全更新

- 定期更新内核、bubblewrap
- 关注 CVE 公告
- 及时打补丁

## 安全建议

### 运行环境

1. **内核版本**：≥ 5.11，推荐 6.x
2. **内核配置**：开启 `CONFIG_OVERLAY_FS_USERNS`、`CONFIG_USER_NS`
3. **系统更新**：保持内核和系统库最新
4. **防火墙**：限制宿主机网络访问

### 部署建议

1. **专用宿主机**：Keeper 宿主机不应运行其他关键服务
2. **网络隔离**：Keeper 宿主机放在独立网络段
3. **监控告警**：监控 Keeper 进程、资源使用
4. **备份策略**：定期备份 Agent 数据

### 开发建议

1. **代码审查**：安全相关代码需严格审查
2. **模糊测试**：对输入进行模糊测试
3. **渗透测试**：定期进行安全测试
4. **依赖审计**：定期扫描依赖漏洞

## 安全事件响应

### 发现漏洞

1. **隔离**：立即隔离受影响的 Agent
2. **分析**：收集日志、取证分析
3. **修复**：应用补丁或升级
4. **验证**：确认漏洞已修复

### 容器逃逸

1. **立即停止**：停止所有 Agent
2. **检查系统**：检查宿主机是否被入侵
3. **清理**：清理受影响的文件
4. **加固**：更新安全配置

## 安全资源

- [Kernel Self Protection Project](https://kernsec.org/)
- [bubblewrap Security](https://github.com/containers/bubblewrap/blob/main/README.md)
- [OverlayFS Security](https://www.kernel.org/doc/html/latest/filesystems/overlayfs.html)
- [Seccomp](https://www.kernel.org/doc/html/latest/userspace-api/seccomp_filter.html)
