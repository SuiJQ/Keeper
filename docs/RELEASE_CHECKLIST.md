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
- [x] bwrap start/stop lifecycle improvements
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
- [ ] Benchmark results documented
- [ ] Startup latency benchmark added
- [ ] CI matrix validated on Ubuntu/Alpine
- [x] bwrap validation strategy documented: happy-path lifecycle is validated via `MockContainer`; real bwrap end-to-end is gated by kernel config (`CONFIG_OVERLAY_FS_USERNS`) and skipped in environments without it

## Release Process

- [ ] Update CHANGELOG.md
- [ ] Tag `v0.1.0`
- [ ] Build multi-arch binaries
- [ ] Publish GitHub Release
- [ ] Announce in project channels
