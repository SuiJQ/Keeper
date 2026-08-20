# 贡献指南

感谢你考虑为 Keeper 项目做出贡献！

## 开发环境

### 前置要求

- Go 1.21+
- Make
- git

### 克隆仓库

```bash
git clone https://github.com/your-org/keeper.git
cd keeper
```

### 安装依赖

```bash
# Go 依赖已包含在 go.mod 中，无需额外安装
go mod download
```

### 构建

```bash
make build
```

### 运行测试

```bash
# 运行所有测试
make test

# 查看覆盖率
make test-cover

# 运行短测试（跳过慢速测试）
make test-short
```

### 代码检查

```bash
# 格式化代码
make fmt

# 代码检查
make vet

# 代码检查（golangci-lint）
make lint
```

## 代码规范

### Go 规范

- 遵循 [Effective Go](https://go.dev/doc/effective_go)
- 使用 `gofmt` 格式化代码
- 使用 `go vet` 检查代码

### 注释规范

- 导出函数必须包含文档注释
- 复杂逻辑需要添加行内注释
- 使用中文或英文注释（保持一致性）

### 错误处理

- 所有错误必须显式处理
- 禁止静默失败
- 使用 `errors.NewKeeperError` 包装错误

### 日志规范

- 使用 `keeper/internal/log` 包
- 日志级别：info / warn / error / debug
- 结构化日志，使用 `Field` 添加键值对

## 提交规范

### Commit Message

格式：`<type>(<scope>): <subject>`

**Type**：
- `feat`：新功能
- `fix`：bug 修复
- `docs`：文档更新
- `style`：代码格式（不影响功能）
- `refactor`：代码重构
- `test`：测试相关
- `chore`：构建/工具链

**Scope**：
- `cmd/keeper`：CLI 入口
- `internal/agent`：Agent 管理
- `internal/container`：容器运行时
- `internal/storage`：存储管理
- `internal/mcp`：MCP Server
- `internal/watchdog`：看门狗
- `internal/bootstrap`：环境探测
- `pkg/config`：配置管理

**示例**：
```
feat(container): add bwrap seccomp support
fix(storage): fix overlayfs cleanup race
docs(readme): update installation instructions
test(mcp): add mock server tests
```

### Pull Request

1. Fork 仓库
2. 创建功能分支：`git checkout -b feat/xxx`
3. 提交修改：`git commit -m "feat(xxx): xxx"`
4. 推送到分支：`git push origin feat/xxx`
5. 创建 Pull Request

### PR 要求

- 代码通过所有测试
- 代码通过 `make lint` 检查
- 包含必要的测试用例
- 更新相关文档

## 测试规范

### 单元测试

- 所有导出函数必须包含测试
- 使用 `testify` 库
- Mock 依赖，隔离测试

### 集成测试

- 在 GitHub Actions 中运行
- 需要真实内核环境
- 标记为 `// +build integration`

### 测试覆盖率

- 新代码覆盖率 ≥ 80%
- 核心模块覆盖率 ≥ 90%

## 行为准则

1. **尊重他人**：友好、耐心、专业
2. **建设性反馈**：提供具体、可操作的改进建议
3. **欢迎新手**：鼓励新人参与，耐心解答问题
4. **保持开放**：接受不同观点，理性讨论

## 许可证

本项目采用 MIT 许可证。贡献代码即表示你同意将代码以 MIT 许可证发布。

---

**注意**：本项目当前处于早期开发阶段（v0.1.0-dev），API 可能随时变更。
