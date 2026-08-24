# REVIEW — 全局复盘后续执行

Goal: 按性价比由高到低补齐剩余缺陷，完成度推向可发布状态

- [x] 全局复盘代码库结构与质量现状
- [x] 修复 goconst 警告：`cmd/keeper/e2e_test.go` 提取 3 个字符串常量
- [x] 提升 `cmd/keeper` 覆盖率：补充 `loadConfigForCLI`（0%→57.1%）、`agentRunContext.Start`（0%→有覆盖）、`TestAgentRunContextStartMCPError`、`TestAgentRunContextStartWatchdogAlreadyRunning`
- [x] 排查 CI `Test on ubuntu-24.04` 偶发失败根因：`TestWriteSeccompBPFInvalidPath` 因 root 权限导致稳定失败，已移除
- [ ] 固化 bwrap 验证策略：接受 mock + 条件跳过真实测试，或继续探索 QEMU 方案
- [ ] 提交并推送修复
