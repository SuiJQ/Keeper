# Keeper Benchmarks

This directory contains performance benchmarks for Keeper.

## Binary Size

```bash
go build -o bin/keeper ./cmd/keeper
ls -lh bin/keeper
```

Target: ≤ 20 MB.

## Startup Latency

Run the CLI startup benchmark:

```bash
go test ./benchmarks -bench=BenchmarkStartupCLI -benchmem
```

Run the in-process startup benchmark:

```bash
go test ./benchmarks -bench=BenchmarkStartupInProcess -benchmem
```

## Current Results

| Benchmark | Target | Typical |
|-----------|--------|---------|
| `BenchmarkStartupCLI` | ≤ 0.5s | ~80µs |
| `BenchmarkStartupInProcess` | ≤ 1ms | ~200ns |

## Notes

- Full container benchmarks require a kernel ≥ 5.11 with `CONFIG_OVERLAY_FS_USERNS`.
- On unsupported kernels, bwrap-related benchmarks will fail or be skipped.
- Cold-start benchmarks are executed in CI to prevent regressions.
