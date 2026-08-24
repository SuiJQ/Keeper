package benchmarks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"keeper/internal/storage"
	"keeper/pkg/config"
)

// BenchmarkConfigLoad measures config loading performance.
func BenchmarkConfigLoad(b *testing.B) {
	b.ReportAllocs()
	tmpDir, err := os.MkdirTemp("", "keeper-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"home":"`+tmpDir+`"}`), 0600); err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		_, _ = config.Load(tmpDir)
	}
}

// BenchmarkStorageCreateAgent measures storage agent creation performance.
func BenchmarkStorageCreateAgent(b *testing.B) {
	b.ReportAllocs()
	tmpDir, err := os.MkdirTemp("", "keeper-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		_, _ = store.CreateAgent(context.TODO(), "bench-agent", 64, 100*1024*1024)
	}
}

// BenchmarkStorageGetAgent measures storage agent retrieval performance.
func BenchmarkStorageGetAgent(b *testing.B) {
	b.ReportAllocs()
	tmpDir, err := os.MkdirTemp("", "keeper-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}

	meta, _ := store.CreateAgent(context.TODO(), "bench-agent", 64, 100*1024*1024)
	_ = meta

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.GetAgent(context.TODO(), "bench-agent")
	}
}

// BenchmarkStorageListAgents measures storage agent listing performance.
func BenchmarkStorageListAgents(b *testing.B) {
	b.ReportAllocs()
	tmpDir, err := os.MkdirTemp("", "keeper-bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStore(tmpDir)
	if err != nil {
		b.Fatal(err)
	}

	// Pre-create some agents
	for i := 0; i < 100; i++ {
		_, _ = store.CreateAgent(context.TODO(), "bench-agent-"+string(rune('0'+i%10)), 64, 100*1024*1024)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.ListAgents(context.TODO())
	}
}
