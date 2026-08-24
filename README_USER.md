# Keeper

**Keeper** 为 AI Agent 提供持久化、隔离、轻量级 Linux 运行时环境。

> 项目已正式命名为 **Keeper**（原 Mirage），所有代码、文档、目录、二进制输出均统一使用该命名。

## 快速开始

### 环境要求

- Linux 内核 ≥ 5.11（推荐 6.x）
- 开启 `CONFIG_OVERLAY_FS_USERNS`
- 支持 UserNS、OverlayFS、renameat2

### 下载与构建

```bash
# 克隆仓库
git clone https://github.com/SuiJQ/Keeper.git
cd Keeper

# 构建二进制
make build

# 运行测试
make test
```

### 使用示例

```bash
# 创建 Agent
./bin/keeper create my-agent

# 启动 Agent
./bin/keeper start my-agent

# 查询所有 Agent
./bin/keeper list

# 查看 Agent 详情
./bin/keeper inspect my-agent
./bin/keeper inspect my-agent --verbose

# 停止 Agent
./bin/keeper stop my-agent

# 克隆 Agent
./bin/keeper fork my-agent my-agent-copy

# 复制文件（宿主机 ↔ Agent）
./bin/keeper cp ./model.bin my-agent:/workspace/models/
./bin/keeper cp -r ./data my-agent:/workspace/
./bin/keeper cp my-agent:/workspace/result.txt ./

# 销毁 Agent（删除所有数据）
./bin/keeper destroy my-agent
```

## 基础配置

Keeper 支持通过 `config.json` 进行运行时配置，配置文件位于 Keeper 数据目录（默认 `~/.local/share/keeper/config.json`）。首次运行会自动生成默认配置。

### 常用配置项

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `log_level` | string | `info` | 日志级别：debug/info/warn/error/fatal |
| `watchdog_timeout` | string | `60s` | 看门狗超时时间 |
| `watchdog_check_interval` | string | `5s` | 看门狗检查间隔 |
| `downloader_threads` | int | `4` | 下载器默认线程数 |
| `metrics_enabled` | bool | `true` | 是否启用 metrics server |
| `metrics_listen_addr` | string | `:9090` | metrics server 监听地址 |

### 示例配置

```json
{
  "log_level": "info",
  "watchdog_timeout": "60s",
  "downloader_threads": 4,
  "metrics_enabled": true,
  "metrics_listen_addr": ":9090"
}
```

## 故障排查

### 内核不支持

如果启动失败并提示 `fatal_unsupported_kernel`，说明当前内核不支持 UserNS + OverlayFS 组合。请升级内核到 5.11+（推荐 6.x）。

### bwrap 失败

如果提示 `fatal_bwrap_exec`，请确认：
1. 已安装 bubblewrap：`sudo apt-get install bubblewrap`（Ubuntu/Debian）或 `sudo apk add bubblewrap`（Alpine）
2. 内核开启 `CONFIG_OVERLAY_FS_USERNS`

### 权限问题

确保当前用户有权限创建 Unix Socket 和 OverlayFS 目录。建议不要在 root 用户下运行 Keeper。

## 文档

- [开发者指南](README_DEV.md)
- [MCP 集成指南](docs/MCP_GUIDE.md)
- [架构设计](ARCHITECTURE.md)
- [发布检查清单](docs/RELEASE_CHECKLIST.md)

## 许可证

MIT
