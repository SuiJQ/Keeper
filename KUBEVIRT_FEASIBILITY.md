# KubeVirt 方案可行性结论

## 实际环境探测结果

### 1. CPU 虚拟化支持
```
CPU flags: hypervisor
systemd-detect-virt: container-other
```
**结论**：当前环境已运行在虚拟机内（嵌套虚拟化），但宿主机未暴露 KVM 加速。

### 2. 关键设备检查
- `/dev/kvm`：❌ 不存在
- `/dev/vhost-net`、`/dev/vhost-vsock`：❌ 不存在
- `/lib/modules/`：❌ 不存在（无法加载 kvm_intel 模块）

### 3. K8s 权限
- 无法检查 CRD 创建权限
- 无法检查 kube-system 命名空间权限
- **推断**：当前 Pod 可能未绑定 cluster-admin 或足够 RBAC

### 4. 容器特权级别
- CapEff: `00000000a80405fb`（无有效能力）
- Seccomp: 0（禁用，但无实际特权）
- 非特权容器

## 最终结论：KubeVirt 方案**不可行**

### 关键阻塞因素

| 阻塞项 | 当前状态 | 对 KubeVirt 的影响 |
|--------|----------|-------------------|
| **KVM 设备** | 无 /dev/kvm | QEMU 只能使用 TCG（纯软件模拟） |
| **嵌套虚拟化** | 未开启/不可用 | 无法获得硬件加速 |
| **内核模块** | 无 /lib/modules | 无法加载 kvm.ko |
| **K8s 权限** | 未知/可能不足 | 无法创建 KubeVirt CRD |
| **CPU flags** | 有 hypervisor（已是 VM） | 二级嵌套虚拟化性能极差 |

### 性能影响估算
- **KVM 加速**：VM 启动 ~1-5 秒，运行性能接近原生
- **TCG 纯模拟**：VM 启动 ~30-60 秒，运行性能仅为原生 10-20%
- **规格书要求**：启动耗时 ≤0.5 秒
- **结论**：TCG 模式完全无法满足性能要求

### 为什么当前环境已是虚拟机
```
物理宿主（未知）
    │
    └── 一级 VM（K8s 节点）
            │
            └── 二级 VM（当前 Pod 容器）← hypervisor flag
                    │
                    └── KubeVirt 需要三级 VM（无法实现）
```

## 方案调整建议

### 方案优先级（修订）

1. **GitHub Actions（主要方案）**
   - Ubuntu 24.04 运行器（内核 6.x）
   - 原生支持 UserNS + OverlayFS
   - 启动性能可测（~秒级，虽高于 0.5s 但可优化）
   - 零成本、自动化、多矩阵

2. **本地 privileged 容器（备选）**
   - 如果在自己的机器上运行，可用 `docker run --privileged`
   - 或使用 `--device=/dev/kvm` 启用嵌套虚拟化
   - 但当前托管环境不支持

3. **Mock + 接口抽象（开发基础）**
   - 确保代码可测试、可并行开发
   - 不依赖任何内核特性

### KubeVirt 的适用场景
KubeVirt 适合：
- 已有 K8s 集群且开启嵌套虚拟化
- 需要长时间运行的 VM 实例
- 团队有 K8s 运维能力

KubeVirt **不适合**：
- 当前这种受限的容器环境
- 对启动性能有严格要求的场景
- 快速迭代的开发阶段

## 行动建议

### 立即执行
1. **接受现实**：当前环境无法运行真正的 Keeper 容器
2. **启动 GitHub Actions**：创建基础 CI workflow
3. **开始代码开发**：使用 Mock 模式，接口抽象

### 未来可选
1. **申请特权容器**：如果 K8s 集群支持，可申请 `privileged` Pod 并挂载 /dev/kvm
2. **本地开发环境**：使用 Docker Desktop 或 Minikube（支持嵌套虚拟化）
3. **云开发环境**：GitHub Codespaces / GitPod（可能提供更好的虚拟化支持）

## 安全提醒
- 用户提供的 GitHub 令牌已本地保存
- 建议用户轮换令牌（特别是如果该令牌已暴露在对话历史中）
- 使用 Fine-grained tokens 限制权限范围

---

*结论时间：2026-08-20*
*状态：KubeVirt 方案在当前环境不可行，锁定 GitHub Actions 为主要方案*
