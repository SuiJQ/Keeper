# 本地 CI 替代方案

> 当 GitHub Actions 不可用或需要快速验证时，可使用以下本地方案替代完整 CI 流程。

## 方案对比

| 方案 | 隔离度 | 内核要求 | 复杂度 | 适用场景 |
|------|--------|----------|--------|----------|
| **Docker 容器** | 中 | 与宿主机一致 | 低 | 快速验证构建和单元测试 |
| **QEMU + Alpine VM** | 高 | 可模拟任意内核 | 中 | 完整集成测试，验证 bwrap/OverlayFS |
| **Multipass/ Lima** | 高 | 真实内核 | 中 | 接近真实环境的快速验证 |
| **直接本地测试** | 无 | 依赖当前内核 | 最低 | 开发阶段快速迭代 |

## 方案一：Docker 容器（推荐日常使用）

### 优势
- 与 GitHub Actions 环境最接近
- 启动快，资源占用低
- 支持多架构镜像（amd64/arm64）

### 使用方法

```bash
# 构建测试镜像
make docker-build

# 在容器中运行完整 CI 流程
make docker-test

# 或手动进入容器
docker run --rm -it -v $(PWD):/workspace -w /workspace keeper:0.1.0-dev sh
# 容器内执行：make ci-test
```

### 限制
- 内核版本与宿主机一致，无法模拟旧内核（如 Ubuntu 20.04 的 5.4）
- 某些内核特性（如 CONFIG_OVERLAY_FS_USERNS）依赖宿主机

## 方案二：QEMU + Alpine VM（完整本地 CI）

### 优势
- 完全独立的内核环境，可模拟各种内核版本
- 支持 Alpine Linux（与 GitHub Actions 矩阵一致）
- 可验证 bwrap、OverlayFS、Seccomp 等内核特性

### 快速开始

#### 1. 安装 QEMU

```bash
# Ubuntu/Debian
sudo apt-get install qemu-system-x86_64 qemu-utils

# 或下载静态构建
wget https://github.com/qemu/qemu/releases/download/v9.0.0/qemu-x86_64-static.tar.xz
```

#### 2. 下载 Alpine 镜像

```bash
# 下载 Alpine 虚拟化镜像（virt）
curl -o alpine-virt.qcow2 https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-virt-3.20.0-x86_64.iso

# 或使用云镜像
curl -o alpine-cloud.qcow2 https://dl-cdn.alpinelinux.org/alpine/v3.20/releases/x86_64/alpine-cloud-init-3.20.0-x86_64.qcow2
```

#### 3. 启动 VM

```bash
# 使用 virt 镜像启动（需要 ISO）
qemu-system-x86_64 \
  -m 2G \
  -smp 2 \
  -drive file=alpine-virt.qcow2,format=qcow2 \
  -netdev user,id=net0,hostfwd=tcp::2222-:22 \
  -device e1000,netdev=net0 \
  -nographic \
  -kernel /path/to/alpine-virt-3.20.0-x86_64.iso \
  -append "console=ttyS0"
```

#### 4. 在 VM 中运行测试

```bash
# SSH 进入 VM
ssh -p 2222 root@localhost

# 安装依赖
apk add --no-cache go bubblewrap strace libseccomp-dev

# 克隆仓库并测试
git clone https://github.com/SuiJQ/Keeper.git
cd Keeper
make ci-test
```

### 自动化脚本

创建 `scripts/local-ci-qemu.sh`：

```bash
#!/bin/bash
set -euo pipefail

ALPINE_VERSION="${ALPINE_VERSION:-3.20}"
KERNEL_VERSION="${KERNEL_VERSION:-6.6}"
MEMORY="${MEMORY:-2G}"
SSH_PORT="${SSH_PORT:-2222}"

echo "启动 Alpine VM (内核 ${KERNEL_VERSION})..."

# 检查是否已存在
if ! virsh list --state-running | grep -q "keeper-ci"; then
  # 创建 VM（使用 libvirt/qemu-kvm 更稳定）
  virt-install \
    --name keeper-ci \
    --memory ${MEMORY} \
    --vcpus 2 \
    --disk path=/var/lib/libvirt/images/keeper-ci.qcow2,size=10 \
    --os-variant alpine \
    --network network=default \
    --graphics none \
    --console pty,target_type=serial \
    --location https://dl-cdn.alpinelinux.org/alpine/v${ALPINE_VERSION}/releases/x86_64/alpine-virt-${ALPINE_VERSION}.0-x86_64.iso \
    --initrd-inject=cloud-init.iso \
    --extra-args="console=ttyS0"
fi

echo "等待 SSH 就绪..."
until ssh -p ${SSH_PORT} root@localhost echo ok 2>/dev/null; do
  sleep 2
done

echo "在 VM 中运行 CI..."
ssh -p ${SSH_PORT} root@localhost "cd /root/Keeper && make ci-test"

echo "清理..."
virsh destroy keeper-ci 2>/dev/null || true
virsh undefine keeper-ci 2>/dev/null || true
```

## 方案三：Multipass/Lima（推荐 macOS/Windows）

### Multipass (Ubuntu 官方)

```bash
# 安装
sudo apt install multipass

# 启动 VM
multipass launch --name keeper-ci --mem 4G --disk 20G 22.04

# 进入 VM
multipass exec keeper-ci -- bash

# 在 VM 内运行测试
git clone https://github.com/SuiJQ/Keeper.git
cd Keeper
make ci-test
```

### Lima (macOS/Linux)

```bash
# 安装
brew install lima

# 启动 VM
limactl start default

# 进入 VM
lima

# 在 VM 内运行测试
make ci-test
```

## 方案四：直接本地测试（开发阶段）

### 前置条件
- Linux 内核 >= 5.11
- 开启 `CONFIG_OVERLAY_FS_USERNS`
- 安装 bubblewrap、strace、libseccomp-dev

```bash
# 安装依赖
sudo apt-get install -y bubblewrap strace libseccomp-dev

# 运行测试
make ci-test

# 仅运行单元测试（跳过需要真实 bwrap 的测试）
make test-short
```

## 推荐工作流

1. **日常开发**：直接本地运行 `make test` 和 `make lint`
2. **提交前验证**：运行 `make ci-local`（tidy + lint + vet + test）
3. **需要完整 CI 验证**：
   - 优先使用 Docker：`make docker-test`
   - 需要内核特性验证：使用 QEMU + Alpine VM
4. **GitHub Actions**：推送后自动运行完整矩阵测试

## 环境探测

项目已内置环境探测功能，可自动检测内核特性：

```bash
# 运行环境探测
go run ./cmd/keeper probe

# 或使用 keeper 二进制
./bin/keeper probe
```

输出示例：
```json
{
  "kernel_version": "6.8.0",
  "overlay_fs_user_ns": true,
  "bwrap_available": true,
  "seccomp_available": true,
  "missing_features": []
}
```

## 故障排查

### Docker 中 bwrap 失败

Docker 默认不支持嵌套 UserNS。如果需要在 Docker 中运行 bwrap：

```bash
# 使用 --privileged 模式（不推荐用于生产）
docker run --privileged ...

# 或配置 Docker daemon
# /etc/docker/daemon.json:
{
  "userns-remap": "default",
  "default-ulimit": {"nofile": 65536}
}
```

### QEMU 性能慢

- 使用 `-cpu host` 启用 KVM 加速（需要 /dev/kvm）
- 增加内存和 CPU 核心数
- 使用 `qcow2` 格式并启用缓存

```bash
# 启用 KVM
qemu-system-x86_64 \
  -enable-kvm \
  -cpu host \
  -m 4G \
  -smp 4 \
  ...
```

## 总结

| 场景 | 推荐方案 |
|------|----------|
| 日常开发 | 直接本地 `make test` |
| 提交前检查 | `make ci-local` |
| 快速 CI 验证 | `make docker-test` |
| 完整内核特性验证 | QEMU + Alpine VM |
| 团队统一环境 | Docker 镜像 |
| 永久 CI | GitHub Actions |
