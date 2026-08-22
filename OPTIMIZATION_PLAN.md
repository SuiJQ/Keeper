# Keeper 项目优化计划 (DAY1-DAY9)

## 当前状态
- 综合测试覆盖率：约 70%
- 目标：85%+
- 代码质量：有 TODO 注释、部分错误处理不规范
- 配置系统：基础框架已就绪，缺少精细化配置
- 观测性：基础 metrics 已实现，缺少健康检查和结构化日志扩展
- 功能：核心功能完成，bwrap 完整功能待验证

## 优化方案

### DAY 1: 代码质量基础修复
- [ ] 运行 golangci-lint 并修复所有问题
- [ ] 运行 gofmt 确保格式统一
- [ ] 运行 errcheck 修复错误处理
- [ ] 清理所有 TODO 注释
- [ ] 修复所有 FIXME 注释

### DAY 2: 配置系统增强
- [ ] 完善 config.json 结构
- [ ] 添加网络转发配置（max_connections, connect_timeout）
- [ ] 添加下载器配置（threads, retry, chunk_size）
- [ ] 添加存储配置（prune_interval, max_snapshots）
- [ ] 实现配置热加载完善

### DAY 3: 观测性增强
- [ ] 完善 metrics 指标（增加细粒度 counter/histogram）
- [ ] 添加健康检查端点（/healthz, /readyz）
- [ ] 扩展结构化日志字段
- [ ] 添加 trace/span 支持
- [ ] 实现 metrics 聚合导出

### DAY 4: 功能完善 - bwrap 后端
- [ ] 完善 bwrap 配置解析
- [ ] 优化 OverlayFS 挂载逻辑
- [ ] 完善 UserNS 配置
- [ ] 添加内核特性检测
- [ ] 实现 bwrap 资源限制

### DAY 5: 功能完善 - 网络与存储
- [ ] 完善网络转发性能（连接池、负载均衡）
- [ ] 实现增量快照存储
- [ ] 优化存储 prune 策略
- [ ] 完善 cp 命令（支持跨设备）
- [ ] 实现 snapshot 压缩

### DAY 6: 测试覆盖率提升 - 核心模块
- [ ] downloader 测试 (42.9% -> 80%+)
- [ ] network 测试 (65.6% -> 80%+)
- [ ] container 测试 (68.1% -> 80%+)
- [ ] cmd/keeper 测试 (55.4% -> 75%+)

### DAY 7: 测试覆盖率提升 - 边缘模块
- [ ] storage 测试 (72.6% -> 85%+)
- [ ] watchdog 测试 (74.3% -> 85%+)
- [ ] metrics 测试 (78.2% -> 85%+)
- [ ] 端到端集成测试

### DAY 8: Bug 扫描与修复
- [ ] 运行 golangci-lint 全量扫描
- [ ] 运行 staticcheck 分析
- [ ] 运行 errcheck 检查
- [ ] 运行 go vet 检查
- [ ] 修复发现的所有问题

### DAY 9: 最终验证与提交
- [ ] 运行完整测试套件
- [ ] 确保覆盖率达标（85%+）
- [ ] 代码审查自检
- [ ] 提交代码并推送
- [ ] 等待 CI 验证
- [ ] 生成最终报告
