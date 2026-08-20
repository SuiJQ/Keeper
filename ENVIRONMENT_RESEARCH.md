# Keeper 环境问题研究报告

## 当前环境精确画像

### 运行时环境
- **容器平台**：Kubernetes (Pod UID: 206bcd8fb13421f413a6c542d2f65725e824161ab1973f3f)
- **宿主机内核**：5.10.134-18.0.12.lifsea8.x86_64
- **容器运行时**：containerd (OverlayFS snapshotter)
- **当前用户**：root (UID 0)
- **Seccomp**：禁用 (Seccomp: 0)
- **特权级别**：非特权容器（CapEff: 0，无有效能力）

### 关键限制
1. **内核版本不足**：5.10.134 < 要求的 5.11+
2. **CONFIG_OVERLAY_FS_USERNS 缺失**：无法在 UserNS 中使用 OverlayFS
3. **UserNS 能力被剥夺**：unshare 后 CapPrm=0，无法执行特权操作
4. **无法安装内核模块**：容器内无 /lib/modules，无法更新内核
5. **无法升级内核**：apt-cache 显示有 6.1.0 包，但无法安装（权限/空间限制）

## 已尝试方案及结果

### 方案 1：内核升级
**尝试**：`apt-get install linux-image-6.1.0-*`
**结果**：❌ 不可行
- 环境为容器化部署，无法修改内核
- 即使安装成功，也无法重启加载新内核

### 方案 2：Bubblewrap 版本升级
**尝试**：编译安装 bwrap 0.11.0（支持 --overlay 语法）
**结果**：⚠️ 部分解决
- 新版本支持正确的 --overlay 参数格式
- 但仍需内核支持 `userxattr`（5.11+）
- 错误信息：`Can't make overlay mount... userxattr: Invalid argument`

### 方案 3：降级 OverlayFS 方案
**尝试**：使用 --tmp-overlay 或 --ro-overlay
**结果**：❌ 不可行
- 这些选项仍需 --overlay-src 且依赖内核特性
- 无法绕过 UserNS + OverlayFS 的限制

### 方案 4：容器内虚拟化
**尝试**：安装 qemu-user-static
**结果**：⚠️ 有限价值
- 可以运行其他架构的静态二进制
- 但无法提供真正的内核虚拟化（无 KVM 支持）
- 不能解决宿主机内核特性缺失问题

## 可行解决方案

### 方案 A：GitHub Actions CI/CD（推荐）

**原理**：利用 GitHub 托管的 Ubuntu 24.04 运行器
- 内核版本：6.x+（满足 ≥5.11 要求）
- 默认开启 CONFIG_OVERLAY_FS_USERNS
- 提供完整的 UserNS + OverlayFS 支持

**实施步骤**：
1. 创建 `.github/workflows/test.yml`
2. 配置矩阵测试（Ubuntu 20.04/22.04/24.04）
3. 在 CI 中执行：
   - 环境探测（UserNS + OverlayFS Dry-Run）
   - 集成测试（容器启动/停止/快照）
   - Seccomp 验证（strace）
   - 性能基准（启动耗时、二进制大小）

**优势**：
- 零成本（开源项目每月 2000 分钟免费额度）
- 多发行版覆盖
- 自动化回归测试
- 与 GitHub 仓库深度集成

**局限**：
- 无法进行交互式调试
- 工作流有运行时间限制（6 小时）
- 需要网络访问 GitHub

### 方案 B：代码 Mock + 分层架构

**原理**：将内核依赖抽象为接口，开发阶段使用 Mock

**实施**：
```go
// internal/container/container.go
type Container interface {
    Start() error
    Stop() error
    Exec(cmd string) ([]byte, error)
}

// internal/container/mock.go
type MockContainer struct{}
func (m *MockContainer) Start() error { return nil }
// ...

// internal/container/bwrap.go
type BwrapContainer struct{ /* real impl */ }
```

**优势**：
- 可在当前环境完成大部分代码开发
- 单元测试不受内核限制
- 架构更清晰，易于测试

**局限**：
- 无法进行端到端测试
- Mock 实现可能与真实行为有偏差
- 增加开发复杂度

### 方案 C：本地虚拟机（备选）

**要求**：用户本地有 Ubuntu 24.04 虚拟机
- 内核 6.x+，支持 CONFIG_OVERLAY_FS_USERNS
- 可以完整测试所有功能
- 作为 CI 的补充（交互式调试）

### 方案 D：内核特性检测 + 优雅降级

**原理**：运行时检测内核支持，不支持则提供替代方案

**可能的降级策略**：
- 不使用 OverlayFS，改用目录硬链接/软链接
- 不使用 UserNS，改用普通进程（牺牲隔离性）
- 提供 `--privileged` 模式（需宿主机支持）

**风险**：
- 降级后安全性降低
- 数据持久化机制改变
- 可能违反规格书要求

## 关于 GitHub 令牌的安全提醒

**⚠️ 重要**：您提供的 GitHub 个人访问令牌（PAT）包含在对话中。

**建议**：
1. **立即轮换该令牌**：访问 GitHub Settings → Developer settings → Personal access tokens → 删除旧令牌并生成新令牌
2. **使用 Fine-grained tokens**：如果需要进行 CI/CD，建议使用细粒度令牌，限制仓库和权限范围
3. **使用 GitHub Actions secrets**：将令牌存储在仓库 Secrets 中，而非硬编码在代码或配置里

**替代方案**：
- 公开仓库可以使用 GitHub Actions 免费运行器，无需令牌
- 私有仓库可以使用 `GITHUB_TOKEN`（自动生成，无需手动创建 PAT）

## 推荐行动路线

### 第一阶段：代码开发（当前环境）
1. 初始化 Go 模块（go mod init keeper）
2. 实现所有不依赖内核特性的模块：
   - 数据结构定义（meta.json、状态机）
   - MCP 协议解析（HTTP/1.1 + JSON-RPC 2.0）
   - 配置管理
   - 日志系统
   - 快照元数据管理
3. 编写全面的单元测试（使用 Mock）
4. 实现 Seccomp BPF 生成（纯计算，不依赖内核）

### 第二阶段：CI/CD 集成（GitHub Actions）
1. 创建测试工作流
2. 在 Ubuntu 24.04 运行器上执行：
   - 环境探测测试
   - 容器生命周期测试
   - 网络子系统测试
3. 性能基准测试
4. 多架构交叉编译验证

### 第三阶段：用户态验证（可选）
1. 在本地 Ubuntu 24.04 虚拟机上测试
2. 交互式调试和性能调优

## 结论

**当前环境无法运行真正的 Keeper 容器**，但可以通过以下组合实现项目目标：
- **代码开发**：在当前环境进行（Mock + 单元测试）
- **集成测试**：GitHub Actions（Ubuntu 24.04）
- **性能调优**：本地虚拟机

这种方式不会损失质量或性能，只是将测试环节从开发环境迁移到 CI/CD。

---

*研究完成时间：2026-08-19*
*基于：Keeper (原 Mirage) v4.0 技术规格书*
