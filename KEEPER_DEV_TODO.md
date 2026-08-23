# Keeper P0-P3 执行计划

## P0 — 阻塞 CI 与核心可用性（进行中）
- [x] 构建 bin/keeper 二进制
- [x] 修复 E2E 测试依赖二进制存在
- [x] 创建 .golangci.yml 配置
- [x] 确保 go test ./... 全绿
- [x] CI 传递 KEEPER_BIN 给测试
- [ ] 验证 golangci-lint / staticcheck 在 CI 中通过
- [ ] 清理 gofmt diff，保证 CI lint 绿色

## P1 — 核心功能补全（进行中）
- [x] 实现 SOCKS5 代理服务端
- [x] 实现端口转发服务端
- [x] 完善 bwrap Start 生命周期与错误态
- [x] MCP SO_PEERCRED 鉴权增强（配置化 UID/GID 白名单）
- [ ] 网络模块单测覆盖新增路径

## P2 — 质量与可维护性（进行中）
- [x] 消除 state.go 重复的状态转换映射
- [x] 统一错误包装与错误码定义
- [ ] 为公共函数添加 godoc 注释
- [x] 提取魔法数字为常量（stateRunning/stateStopped/stateFatalBwrap/defaultStrategyName/customStrategyName 等）
- [ ] 审查并简化 buildArgs 逻辑
- [x] 补充 agent 状态机测试
- [x] 补充 config 热重载测试
- [ ] 添加 network 集成测试
- [ ] 提高整体覆盖率至 80%+

## P3 — 可观测性与边缘特性（待开始）
- [x] 添加 Prometheus 更多指标
- [ ] 文档：API 参考、示例脚本
- [ ] 性能基准测试与优化（二进制大小 ≤20MB、启动耗时 ≤0.5s）
- [ ] 准备 v0.1.0 release checklist
