# MCP_SOPEERCRED — SO_PEERCRED auth enhancement for MCP server
- [x] Audit current SO_PEERCRED implementation in internal/mcp/server.go
- [x] Fix groupMember to check client supplementary groups instead of server's
- [x] Add linux build tag for SO_PEERCRED syscall
- [x] Add comprehensive auth tests in server_test.go
- [x] Run go test ./internal/mcp and verify all pass
