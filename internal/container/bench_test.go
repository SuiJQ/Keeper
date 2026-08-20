package container

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BenchmarkBuildArgs benchmarks buildArgs performance
func BenchmarkBuildArgs(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "keeper-bench-*")
	require.NoError(b, err)
	defer os.RemoveAll(tmpDir)

	rootfs := filepath.Join(tmpDir, "rootfs")
	upper := filepath.Join(tmpDir, "upper")
	work := filepath.Join(tmpDir, "work")
	require.NoError(b, os.MkdirAll(rootfs, 0755))
	require.NoError(b, os.MkdirAll(upper, 0755))
	require.NoError(b, os.MkdirAll(work, 0755))

	c := &BwrapContainer{logger: &testLogger{}}
	spec := ContainerSpec{
		Name:     "bench-container",
		Rootfs:   rootfs,
		UpperDir: upper,
		WorkDir:  work,
		ShmSize:  64,
		Envvars:  []string{"AGENT_NAME=bench", "TEST_MODE=1"},
		Ports:    []PortMapping{{Host: 8080, Container: 80}},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.buildArgs(spec)
	}
}

// BenchmarkMockStartStop benchmarks mock container start/stop
func BenchmarkMockStartStop(b *testing.B) {
	factory := NewMockFactory(&testLogger{})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("bench-%d", i)
		c, err := factory.Create(name)
		require.NoError(b, err)

		_, err = c.Start(context.Background(), ContainerSpec{Name: name})
		require.NoError(b, err)

		err = c.Stop(context.Background(), 5*time.Second)
		require.NoError(b, err)
	}
}

// BenchmarkMockExec benchmarks mock container exec
func BenchmarkMockExec(b *testing.B) {
	factory := NewMockFactory(&testLogger{})
	c, err := factory.Create("bench-exec")
	require.NoError(b, err)

	_, err = c.Start(context.Background(), ContainerSpec{Name: "bench-exec"})
	require.NoError(b, err)
	defer c.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = c.Exec(context.Background(), ExecRequest{
			Command: "echo hello",
			Timeout: 30 * time.Second,
		})
		require.NoError(b, err)
	}
}
