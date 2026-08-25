# Keeper v0.1.0 后续完善计划

## P0 — 发布阻塞项
- [ ] 解决 GitHub push 认证问题，推送当前 commit
- [ ] 创建 Git Tag `v0.1.0`
- [ ] 创建 GitHub Release（amd64 + arm64 二进制）
- [ ] 验证 Release Asset 可被 install.sh 下载

## P1 — 核心功能完善
- [ ] 提升 `internal/container` 测试覆盖率（当前 43.0%）
- [ ] 提升 `internal/storage` 测试覆盖率（当前 73.4%）
- [ ] 提升 `internal/bootstrap` 测试覆盖率（当前 71.4%）
- [ ] 提升 `internal/logring` 测试覆盖率（当前 67.5%）
- [ ] 验证 Docker 后端在 CI 中的完整生命周期测试

## P2 — 质量与可维护性
- [ ] 补充 `cmd/keeper` 边界条件测试（当前 82.3%）
- [ ] 增加集成测试覆盖更多错误路径
- [ ] 优化错误消息的可读性与一致性
- [ ] 统一日志字段命名规范

## P3 — 文档与生态
- [ ] 补充 User-facing tutorial for first-time setup
- [ ] 补充 MCP client integration guide
- [ ] 更新 CHANGELOG.md 至正式 v0.1.0
- [ ] 准备 v0.1.0 Announce in project channels

## 当前状态（2026-08-24）
- 本地测试全绿（15 包）
- 覆盖率：cmd/keeper 82.3%，整体约 79%
- Release workflow 已创建
- README 安装命令已落地到 install.sh
- GitHub push 需认证修复
