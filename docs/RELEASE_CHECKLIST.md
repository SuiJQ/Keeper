# v0.1.0 Release Checklist

## Code Quality

- [x] `go test ./...` green
- [x] `golangci-lint run` issues reviewed and categorized
- [x] Godoc added for core public APIs
- [x] Repeated strings extracted to constants where practical
- [x] No accidental `sed` artifacts or broken builds

## Functionality

- [x] SOCKS5 proxy server implemented
- [x] Port forwarding server implemented
- [x] MCP SO_PEERCRED auth with UID/GID allowlists
- [x] Docker start/stop lifecycle improvements
- [x] bwrap remains compatible but is no longer the default runtime
- [x] Config hot-reload validated by tests

## Testing

- [x] Unit tests cover storage, bootstrap, agent state, config
- [x] Network integration tests added
- [x] E2E tests pass when `bin/keeper` exists

## Documentation

- [x] README updated with current feature set
- [x] API reference drafted
- [x] Example scripts added
- [ ] User-facing tutorial for first-time setup
- [ ] MCP client integration guide

## Performance & Compatibility

- [x] Binary size within target (≤20MB)
- [x] Benchmark results documented (see benchmarks/README.md)
- [x] Startup latency benchmark added (benchmarks/startup_bench_test.go)
- [x] CI matrix validated on Ubuntu/Alpine
- [x] Container runtime strategy documented: Docker is the default backend; bwrap remains a compatible alternative for environments without Docker

## Release Process

- [x] Update CHANGELOG.md
- [ ] Tag `v0.1.0`
- [ ] Build multi-arch binaries (optional: not required for this release)
- [ ] Publish GitHub Release
- [ ] Announce in project channels
