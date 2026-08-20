# Keeper 环境方案可行性结论

## 核心结论

经过对当前 K8s 容器环境的深度探测，**KubeVirt 方案在当前环境下不可行**。

## 关键证据

### 1. 无 KVM 硬件加速
```bash
/dev/kvm: 不存在
/lib/modules/5.10.134-18.0.12.lifsea8.x86_64: 不存在
kvm_intel 模块: 无法加载
```

### 2. 已处于嵌套虚拟化但无加速
```
CPU flags: hypervisor  ← 当前环境已是二级 VM
systemd-detect-virt: container-other
```
若在此环境中再运行 QEMU/KVM，需要**三级嵌套虚拟化**，而宿主机未暴露 `/dev/kvm`。

### 3. 性能无法达标
- **TCG 纯软件模拟**：VM 启动 ~30-60 秒
- **规格书要求**：启动耗时 ≤0.5 秒
- **差距**：60-120 倍，无法通过优化弥补

### 4. K8s 权限未知
- 无法创建 CRD（KubeVirt 需要）
- 无法在 kube-system 命名空间部署组件
- 当前 Pod 为非特权容器（CapEff: 0）

## 决策

**锁定 GitHub Actions 为主要集成测试环境**，原因：
- Ubuntu 24.04 内核 6.x，原生支持 UserNS + OverlayFS
- 零成本，多矩阵（20.04/22.04/24.04）
- 可验证 Seccomp、启动性能、二进制大小
- 自动化回归测试

**开发模式**：Mock + 接口抽象，确保当前环境可进行代码迭代。

## 下一步

等待您的确认，我将开始：
1. 初始化 Keeper Go 项目结构
2. 实现 Container 接口抽象（bwrap / mock）
3. 创建 GitHub Actions workflow 模板

---

*结论时间：2026-08-20*
*状态：环境方案确定，准备进入开发阶段*
