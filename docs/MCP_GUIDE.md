# MCP 集成指南

## 概述

Keeper 内置 MCP (Model Context Protocol) Server，允许 AI Agent 通过 JSON-RPC 2.0 over Unix Socket 控制容器生命周期。

## 架构

```
┌─────────────┐       Unix Socket        ┌─────────────┐
│   AI Agent  │ ←───────────────────────→ │ Keeper MCP  │
│ (Client)    │   SO_PEERCRED 鉴权        │   Server    │
└─────────────┘                           └──────┬──────┘
                                                  │
                                            ┌─────▼─────┐
                                            │  Keeper   │
                                            │   CLI     │
                                            └───────────┘
```

## 快速开始

### 1. 启动 Keeper MCP Server

```bash
# 创建 Agent
./bin/keeper create my-agent

# 启动 Agent（会自动启动 MCP Server）
./bin/keeper start my-agent
```

MCP Server 会自动创建 Unix Socket：
```
~/.local/share/keeper/agents/<agent-name>/mcp.sock
```

### 2. 配置 AI Agent 连接

以 Claude Desktop 为例，在 `claude_desktop_config.json` 中添加：

```json
{
  "mcpServers": {
    "keeper": {
      "command": "/path/to/keeper",
      "args": ["mcp", "agent", "my-agent"],
      "env": {
        "KEEPER_HOME": "/home/user/.local/share/keeper"
      }
    }
  }
}
```

### 3. 可用工具

| 工具 | 参数 | 说明 |
|------|------|------|
| `create_agent` | `name` | 创建新 Agent |
| `start_agent` | `name` | 启动 Agent |
| `stop_agent` | `name` | 停止 Agent |
| `destroy_agent` | `name` | 销毁 Agent |
| `recover_agent` | `name` | 恢复 Agent |
| `list_agents` | - | 列出所有 Agent |
| `inspect_agent` | `name` | 查看 Agent 详情 |
| `fork_agent` | `source`, `target` | 克隆 Agent |
| `copy_file` | `src`, `dst` | 复制文件 |
| `snapshot` | `name` | 创建快照 |
| `rollback` | `name` | 回滚快照 |

## 鉴权

MCP Server 使用 SO_PEERCRED 进行 Unix Socket 客户端身份验证：

- 默认只接受同一 UID 的客户端连接
- 可通过 `mcp_allowed_uids` 和 `mcp_allowed_gids` 配置白名单
- Socket 文件权限：`srwxr-xr-x` (owner only)

## 示例：Python 客户端

```python
import json
import socket

SOCKET_PATH = "/home/user/.local/share/keeper/agents/my-agent/mcp.sock"

def call_mcp(method: str, params: dict) -> dict:
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as sock:
        sock.connect(SOCKET_PATH)
        request = {
            "jsonrpc": "2.0",
            "method": method,
            "params": params,
            "id": 1
        }
        sock.sendall((json.dumps(request) + "\n").encode())
        response = sock.recv(4096).decode()
        return json.loads(response)

# 列出所有 Agent
result = call_mcp("list_agents", {})
print(result)
```

## 故障排查

### 连接被拒绝

- 确认 Agent 已启动：`./bin/keeper status my-agent`
- 确认 Socket 文件存在：`ls -la ~/.local/share/keeper/agents/my-agent/mcp.sock`
- 检查 Socket 权限：应为 `srwxr-xr-x`，owner 为当前用户

### 鉴权失败

- 检查 `mcp_allowed_uids` / `mcp_allowed_gids` 配置
- 确认客户端进程 UID/GID 在白名单中

### 工具调用失败

- 查看 Keeper 日志：`~/.local/share/keeper/logs/keeper.log`
- 确认 Agent 处于正确状态（如 `start_agent` 需要 Agent 为 `stopped` 状态）

## 安全注意事项

- MCP Socket 仅接受本地 Unix 连接，不接受网络访问
- SO_PEERCRED 防止未授权进程伪造身份
- 建议将 `mcp_allowed_uids` 和 `mcp_allowed_gids` 设为最小权限集合
- Socket 文件位于 Agent 私有目录，不受其他用户访问
