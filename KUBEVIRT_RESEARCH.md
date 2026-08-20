# Keeper 环境问题研究 — KubeVirt 方案分析

## 1. KubeVirt 技术概述

### 1.1 什么是 KubeVirt
KubeVirt 是一个 Kubernetes 扩展，允许在 Pod 中运行虚拟机（VM），同时保留原生的 Kubernetes 工作流。

**核心架构**：
```
┌─────────────────────────────────────┐
│          Kubernetes API             │
├─────────────────────────────────────┤
│   KubeVirt Controller/Operator      │
│   (virt-controller, virt-handler)   │
├─────────────────────────────────────┤
│   Pod 中的 libvirt + QEMU 容器     │
│   ┌─────────────────────────────┐   │
│   │  VM（独立内核）              │   │
│   │  - 内核版本可自定义         │   │
│   │  - 完整虚拟化支持           │   │
│   │  - 可启用 KVM（若宿主机支持）│   │
│   └─────────────────────────────┘   │
└─────────────────────────────────────┘
```

### 1.2 关键特性
- **独立内核**：VM 运行完全独立的内核（可运行 Ubuntu 24.04 + 内核 6.x）
- **KVM 加速**：若宿主机支持，可启用硬件虚拟化加速
- **持久化存储**：支持 PVC（Persistent Volume Claim）作为 VM 磁盘
- **网络模型**：支持多种网络策略（Bridge/Macvtap/SRIOV）
- **生命周期管理**：通过 Kubernetes API 管理 VM 的启动/停止/快照

## 2. KubeVirt 解决内核问题的原理

### 2.1 传统容器限制
```
宿主内核 5.10.134（无 CONFIG_OVERLAY_FS_USERNS）
    │
    ├── 容器 1（bwrap） ❌ OverlayFS 失败
    ├── 容器 2（bwrap） ❌ OverlayFS 失败
    └── 所有容器共享内核限制
```

### 2.2 KubeVirt 方案
```
宿主内核 5.10.134（限制依然存在）
    │
    └── Pod 中的 QEMU VM
            │
            ├── 客户机内核 6.8.0（Ubuntu 24.04）✅ 支持 UserNS+OverlayFS
            ├── 客户机 RootFS ✅ 完整 Linux 环境
            └── 在 VM 内部运行 Keeper
                    ├── bwrap --overlay ✅ 成功
                    ├── OverlayFS+UserNS ✅ 可用
                    └── Seccomp BPF ✅ 完全支持
```

### 2.3 核心优势
| 限制 | 传统容器 | KubeVirt VM |
|------|----------|-------------|
| 内核版本 | 固定 5.10.134 | 可运行 6.8+ |
| CONFIG_OVERLAY_FS_USERNS | ❌ 缺失 | ✅ 可启用 |
| UserNS 支持 | ❌ 被剥夺 | ✅ 完整支持 |
| Seccomp BPF | ✅ 支持 | ✅ 支持 |
| 硬件访问 | 受限 | 可通过 PCI passthrough |
| 启动开销 | ~ms | ~秒级（冷启动） |

## 3. 在当前 K8s 环境中的可行性分析

### 3.1 当前环境检查
```bash
# 已确认信息
- 运行在 Kubernetes 集群中（Pod UID: 206bcd8fb...）
- 使用 containerd 运行时
- OverlayFS snapshotter
- 非特权容器（CapEff: 0）
- 无 KubeVirt 组件（无 CRDs、无 virt 命名空间）
```

### 3.2 部署 KubeVirt 的前提条件

| 必要条件 | 当前状态 | 说明 |
|-----------|----------|------|
| K8s 集群版本 | 未知 | 需要 ≥1.21（推荐 1.28+） |
| 集群权限 | 未知 | 需要 cluster-admin 或特定 RBAC |
| 宿主机内核 | 5.10.134 | 支持 KVM（需验证） |
| 嵌套虚拟化 | 未知 | 关键瓶颈（见下文） |
| 存储类 | 未知 | 需要支持 RWX 的 PVC |
| 网络插件 | 未知 | 需要支持 Multus 或 CNI 配置 |

### 3.3 关键阻塞：嵌套虚拟化（Nested Virtualization）

**问题**：当前容器本身运行在虚拟化环境中（通过 cgroup 和 /proc/cgroup 推断），要在此容器内再运行 QEMU/KVM，需要嵌套虚拟化支持。

**检测方法**：
```bash
# 在容器内检查
cat /proc/cpuinfo | grep flags | head -1 | grep -o "vmx\|svm"
# 若输出 vmx (Intel) 或 svm (AMD)，说明 CPU 支持虚拟化扩展

# 检查 /dev/kvm
ls -la /dev/kvm 2>/dev/null || echo "No /dev/kvm"
```

**风险**：
- 即使 CPU 支持 vmx/svm，K8s 宿主机可能未启用嵌套虚拟化
- 无 /dev/kvm 设备时，QEMU 只能使用 TCG（纯软件模拟），性能极差
- TCG 模式下运行完整 Linux VM，启动时间可能 >30 秒，违反规格书 ≤0.5s 要求

## 4. KubeVirt 实施路径（假设条件满足）

### 4.1 架构设计
```
 keeper/
 ├── cmd/keeper/                    # CLI 入口（保持不变）
 ├── internal/
 │   ├── vm/                        # NEW: KubeVirt 管理模块
 │   │   ├── client.go              # 调用 K8s API 创建/删除 VM
 │   │   ├── lifecycle.go           # VM 启动/停止/重启
 │   │   └── ssh.go                 # 通过 kubectl virt console 或 SSH 通信
 │   ├── container/                 # 修改：根据环境选择后端
 │   │   ├── interface.go           # Container interface
 │   │   ├── bwrap.go               # 传统 bwrap 实现
 │   │   └── kubevirt.go            # NEW: KubeVirt 实现
 │   └── ...
 └── deploy/
     └── kubevirt/
         ├── vm-image.yaml          # KubeVirt VM 镜像定义
         ├── storageclass.yaml      # 持久化存储配置
         └── networkpolicy.yaml     # 网络策略
```

### 4.2 VM 镜像准备
```dockerfile
# 基于 Ubuntu 24.04 构建 Keeper VM 镜像
FROM ubuntu:24.04

# 安装依赖
RUN apt-get update && apt-get install -y \
    bubblewrap \
    golang-1.21 \
    libseccomp-dev \
    && rm -rf /var/lib/apt/lists/*

# 复制 Keeper 二进制
COPY keeper /usr/local/bin/keeper

# 配置 systemd 或 init
RUN systemctl enable keeper

# 暴露 MCP Socket 和端口转发
EXPOSE 1080
```

**构建工具**：
- `docker buildx` 构建多架构镜像
- `virtctl image-upload` 上传至容器 registry
- 或使用 `docker save` + `virtctl image-upload` 导入

### 4.3 VM 生命周期管理
```go
// internal/vm/kubevirt.go
type KubeVirtManager struct {
    clientset *virtv1clientset.Clientset
    namespace string
}

func (k *KubeVirtManager) Start(name string) error {
    // 1. 创建/获取 VM 实例
    // 2. 调用 virtctl start vm
    // 3. 等待 VM Running 状态
    // 4. 通过 kubectl port-forward 或 Service 暴露端口
}

func (k *KubeVirtManager) Stop(name string) error {
    // 1. 调用 virtctl stop vm
    // 2. 等待 VM Stopped 状态
}
```

## 5. 方案对比

| 维度 | GitHub Actions | KubeVirt | Mock/本地开发 |
|------|----------------|----------|---------------|
| **环境一致性** | ⭐⭐⭐⭐⭐ 标准化运行器 | ⭐⭐⭐⭐ 接近生产 | ⭐⭐ 完全模拟 |
| **内核版本** | 6.x (Ubuntu 24.04) | 可自定义（6.x+） | N/A |
| **UserNS+OverlayFS** | ✅ 原生支持 | ✅ VM 内支持 | ❌ Mock |
| **启动性能** | CI 启动 ~秒级 | VM 启动 ~10-30s | 瞬时 |
| **开发便利性** | 需推送代码触发 | 需集群权限 | 本地即时 |
| **调试能力** | 有限（日志） | 中等（console/ssh） | 完全本地 |
| **成本** | 免费（开源） | 集群资源消耗 | 零 |
| **实施复杂度** | 低 | 高 | 低 |
| **持久化** | 无（每次 fresh） | PVC 支持 | 文件系统 |

## 6. 关键风险与缓解

### 6.1 KubeVirt 方案风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| **嵌套虚拟化不可用** | 无法运行 KVM，TCG 性能差 | 提前检测，失败则回退到 GitHub Actions |
| **K8s 集群权限不足** | 无法创建 CRD、VM | 使用 GitHub Actions 作为主要方案 |
| **存储类不支持 RWX** | VM 无法持久化 | 使用 HostPath 或临时存储（仅测试） |
| **网络策略限制** | 无法暴露端口 | 使用 NodePort/LoadBalancer 或端口转发 |
| **VM 镜像维护** | 每次内核更新需重建 | 自动化构建流程（GitHub Actions） |

### 6.2 GitHub Actions 方案风险

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| **运行器可用性** | CI 偶发故障 | 多发行版矩阵，自动重试 |
| **启动耗时测量** | 共享运行器噪声大 | 多次运行取中位数，排除冷启动 |
| ** Secrets 管理** | 令牌泄露风险 | 使用 GitHub 自动生成的 GITHUB_TOKEN |

## 7. 推荐策略：混合方案

### 7.1 分层测试策略
```
┌──────────────────────────────────────────┐
│          GitHub Actions (主要)            │
│  - Ubuntu 20.04/22.04/24.04 矩阵        │
│  - 集成测试（bwrap + 真实内核）          │
│  - Seccomp 验证（strace）                │
│  - 性能基准（启动耗时、二进制大小）      │
└──────────────────────────────────────────┘
                    ▲
                    │ 镜像/代码同步
                    │
┌──────────────────────────────────────────┐
│          KubeVirt (补充)                  │
│  - 仅在嵌套虚拟化可用时启用              │
│  - 长时间运行测试（内存泄漏、D 状态）     │
│  - 交互式调试（virtctl console）          │
└──────────────────────────────────────────┘
                    ▲
                    │ 代码/镜像同步
                    │
┌──────────────────────────────────────────┐
│          本地开发（Mock）                 │
│  - 快速迭代业务逻辑                      │
│  - 单元测试（Mock Container 后端）       │
│  - 架构设计和文档编写                    │
└──────────────────────────────────────────┘
```

### 7.2 环境自动选择逻辑
```go
// internal/runtime/selector.go
type Runtime string

const (
    RuntimeBwrap     Runtime = "bwrap"
    RuntimeKubeVirt  Runtime = "kubevirt"
    RuntimeMock      Runtime = "mock"
)

func SelectRuntime() Runtime {
    // 1. 检查内核版本
    if kernelVersion >= "5.11" && overlayFSUsersNSEnabled {
        return RuntimeBwrap
    }
    
    // 2. 检查 KubeVirt 可用性
    if isKubeVirtAvailable() && isNestedVirtualizationEnabled() {
        return RuntimeKubeVirt
    }
    
    // 3. 降级到 Mock（仅开发/测试）
    return RuntimeMock
}
```

## 8. 立即行动建议

### 8.1 短期（本周）
1. **确认 K8s 集群权限**：尝试安装 KubeVirt（需要 cluster-admin）
   ```bash
   # 检查当前权限
   kubectl auth can-i create crd
   kubectl auth can-i create pods --namespace=kube-system
   ```
2. **检测嵌套虚拟化**：
   ```bash
   cat /proc/cpuinfo | grep flags | head -1 | grep -o "vmx\|svm"
   ls -la /dev/kvm 2>/dev/null
   ```
3. **启动 GitHub Actions 配置**：创建基础 workflow 文件

### 8.2 中期（2-4 周）
1. 完成 Keeper 核心模块开发（Mock 模式）
2. 实现 Runtime 自动选择逻辑
3. 在 GitHub Actions 中跑通 bwrap 集成测试
4. （可选）在 K8s 集群部署 KubeVirt POC

### 8.3 长期（1-2 月）
1. 完成所有后端实现（bwrap + KubeVirt）
2. 多环境测试覆盖
3. 性能优化和文档完善

## 9. 结论

**KubeVirt 是一个理论上可行的方案**，可以完全绕过宿主内核限制，在 VM 中运行支持完整特性的客户机内核。

**但在当前环境中存在重大不确定性**：
1. 缺乏 K8s 集群安装权限（无法验证）
2. 嵌套虚拟化支持未知（关键瓶颈）
3. 实施和维护成本高

**推荐策略**：
- **主要方案**：GitHub Actions（确定性高、成本低、实施快）
- **补充方案**：KubeVirt（仅在集群权限和嵌套虚拟化确认可用时启用）
- **开发基础**：Mock + 接口抽象（确保代码可测试）

这样可以保证项目在不受环境限制的情况下高质量推进，同时保留未来利用 KubeVirt 进行深度测试的可能性。

---

*研究完成时间：2026-08-20*
*分析对象：KubeVirt 在 Keeper 项目中的适用性*
