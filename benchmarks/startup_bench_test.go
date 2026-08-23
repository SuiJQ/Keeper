package benchmarks

import (
	"testing"
	"time"
)

// BenchmarkStartup measures CLI invocation overhead.
func BenchmarkStartup(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		// Replace with actual command invocation or in-process startup path.
		_ = time.Since(start)
	}
}
