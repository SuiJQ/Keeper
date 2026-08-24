# RELEASE_PREP — v0.1.0 发布准备与最后收敛
- [ ] P2：将 cmd/keeper 覆盖率提升至 80%+（当前 79.8%，差 0.2%）
- [ ] P3：在 internal/container/bwrap_integration_test.go 顶部补充验证策略注释
- [ ] P3：更新 CHANGELOG.md（从 0.1.0-dev 更新为正式变更记录）
- [ ] P3：确认版本号（当前 0.1.0-dev → 是否改为 v0.1.0）
- [ ] P3：确认 RELEASE_CHECKLIST.md 全部勾选
- [ ] P3：监控接下来 1–2 次 GitHub Actions 运行，确认 CI 稳定性

## 备注
- 多架构构建支持不需要
- bwrap 验证策略主体已写入 docs/RELEASE_CHECKLIST.md，仅需补充测试文件注释
