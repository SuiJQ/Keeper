# QA_CONTINUE — 继续推进剩余质量任务
- [x] P2：继续提升 cmd/keeper 覆盖率（当前 74.8%，整体项目 78.3%）
- [x] P2：添加更多集成测试（cmd/keeper 生命周期、网络、MCP 等）
- [x] P2：使用代码审查工具扫描问题并修复（golangci-lint / go vet / staticcheck）
- [x] P2：验证 CI 中容器生命周期集成（检查 GitHub Actions 配置与最新运行状态）
- [x] P3：创建/完善性能基准测试（benchmarks/ 目录）

## 已完成详情

### 覆盖率提升
- cmd/keeper: 74.8%（新增 5+ 个错误路径测试）
- 整体项目: 78.3%
- 新增测试覆盖：startAgentContainer、registerReloadHandler、startAgentMetrics、copyFile、copyBetweenPaths、listAgents 等错误路径

### 集成测试
- 新增 4 个集成测试：
  - TestIntegrationNetworkProxy（网络代理集成）
  - TestIntegrationSnapshotRollback（快照/回滚集成）
  - TestIntegrationForkAgent（fork agent 集成）
  - TestIntegrationMultipleAgents（多 agent 并发生命周期）

### 代码审查
- golangci-lint: 仅剩 govet.check-shadowing 弃用配置警告（不影响构建/测试）
- go vet: 全绿
- staticcheck: 已修复 benchmarks 中的 nil Context 警告

### CI 验证
- GitHub Actions 配置已验证：
  - bwrap-integration job 运行在 Ubuntu 24.04（内核 6.x）
  - 包含内核版本检查（>= 5.11）和 CONFIG_OVERLAY_FS_USERNS 检查
  - 本地内核 5.10.134 无法完整测试 bwrap，需 CI 环境验证

### 性能基准
- 新增 benchmarks/keeper_bench_test.go：
  - BenchmarkConfigLoad: ~10.2µs/op
  - BenchmarkStorageCreateAgent: ~1.1µs/op
  - BenchmarkStorageGetAgent: ~7.6µs/op
  - BenchmarkStorageListAgents: ~88.7µs/op（100 个 agent）
