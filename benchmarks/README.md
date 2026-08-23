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
go test ./cmd/keeper -bench=BenchmarkStartup -benchmem
```

Target: ≤ 0.5s for local create/start path on supported kernels.

## Notes

- Full container benchmarks require a kernel ≥ 5.11 with `CONFIG_OVERLAY_FS_USERNS`.
- On unsupported kernels, bwrap-related benchmarks will fail or be skipped.
