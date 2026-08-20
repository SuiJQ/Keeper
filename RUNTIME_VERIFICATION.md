# Mirage v4.0 — 运行时验证报告

## 验证时间
2026-08-19

## 环境信息
- **宿主机内核**：5.10.134-18.0.12.lifsea8.x86_64
- **宿主机 OS**：Debian 12 (bookworm)
- **当前运行环境**：容器化环境（有限的特权和命名空间支持）

## 验证结果

### ✅ 已通过
| 检查项 | 结果 |
|--------|------|
| Go 1.21.13 | 已安装 |
| Bubblewrap 0.11.0 | 已编译安装（满足 ≥0.9.0 要求） |
| strace 6.1 | 已安装 |
| libseccomp-dev 2.5.4 | 已安装 |
| libcap-dev 2.66 | 已安装 |
| meson 1.0.1 + ninja-build 1.11.1 | 已安装 |
| OverlayFS 内核支持 | `/proc/filesystems` 包含 `overlay` |
| UserNS 基础支持 | `unshare --user --map-root-user id` 测试通过 |

### ⚠️ 关键阻塞：OverlayFS + UserNS 不可用
**验证命令**：
```bash
bwrap --unshare-all --as-pid-1 --uid 0 --gid 0 --cap-drop ALL \
  --overlay-src /tmp/rootfs --overlay /tmp/upper /tmp/work / \
  -- /bin/true
```

**错误信息**：
```
bwrap: Can't make overlay mount on /newroot/ with options \
upperdir=/tmp/upper,workdir=/tmp/work,lowerdir=/tmp/rootfs,userxattr: \
Invalid argument
```

**根因分析**：
- 错误中的 `userxattr` 选项是关键线索
- 该选项要求内核 **≥5.11** 且开启 `CONFIG_OVERLAY_FS_USERNS`
- 当前内核 5.10.134 不支持 UserNS 场景下的 OverlayFS extended attributes
- 即使 `/proc/filesystems` 显示 `overlay`，也无法在用户命名空间中使用

### 🔍 内核配置缺失确认
```bash
zcat /proc/config.gz 2>/dev/null | grep OVERLAY_FS
# 输出：
# CONFIG_OVERLAY_FS=y
# CONFIG_OVERLAY_FS_REDIRECT_DIR=y
# CONFIG_OVERLAY_FS_REDIRECT_ALWAYS_FOLLOW=y
# CONFIG_OVERLAY_FS_INDEX=y
# 未找到 CONFIG_OVERLAY_FS_USERNS
```

**结论**：当前宿主机内核 **不满足 Mirage 的最低内核要求（≥5.11 + CONFIG_OVERLAY_FS_USERNS）**。

## 对开发的影响

### 1. 无法在当前环境进行端到端测试
- 无法启动真实的 Mirage 容器
- 所有依赖 UserNS + OverlayFS 的功能无法验证

### 2. 可继续推进的工作
- ✅ **代码开发**：Go 模块初始化、数据结构定义、业务逻辑编写
- ✅ **单元测试**：不依赖内核特性的纯逻辑测试
- ✅ **构建系统**：CGO_ENABLED=0 静态编译、Seccomp BPF 生成
- ✅ **静态分析**：代码审查、架构设计
- ✅ **文档编写**：README、API 文档

### 3. 需要迁移到兼容环境的工作
- ❌ **集成测试**：真实的容器启动/停止/快照/回滚
- ❌ **Seccomp 验证**：`strace -e trace=clone3,getrandom,...` 系统调用验证
- ❌ **MCP 通信测试**：Unix Socket + SO_PEERCRED 鉴权
- ❌ **性能基准**：启动耗时（≤0.5s）、二进制大小（≤20MB）

## 建议行动

### 短期（当前环境）
1. 完成 Go 项目脚手架搭建
2. 实现所有不依赖内核特性的模块
3. 编写全面的单元测试和 Mock

### 中期（寻找兼容环境）
1. **AWS/GCP/Azure 云实例**：选择 Ubuntu 24.04 LTS（内核 6.x+）
2. **本地虚拟机**：Ubuntu 24.04 / Fedora 40+
3. **WSL2**：Windows 11 + WSL2 Ubuntu 24.04（内核 6.x）
4. **容器环境调整**：如当前环境本身运行在 privileged container 中，可能需要特权模式

### 长期（CI/CD）
1. 配置多发行版 CI：
   - Ubuntu 20.04（内核 5.4，需验证是否足够）
   - Ubuntu 22.04（内核 5.15，可能支持）
   - Ubuntu 24.04（内核 6.x，推荐）
   - Alpine Edge（musl + 最新内核）

## 环境变量参考
```bash
export PATH=/usr/local/go/bin:$PATH
export CGO_ENABLED=0
export MIRAGE_HOME=$HOME/.local/share/mirage
export MIRAGE_DISABLE_CROSS_DEVICE_CHECK=1  # NFS/FUSE 环境跳过跨设备检查
```

---

*验证人：QwenPaw QA Agent*
*状态：环境准备完成，核心功能待兼容内核验证*
