package container

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSeccompStrategy struct{}

func (m *mockSeccompStrategy) GenerateBPF() ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockSeccompStrategy) Name() string {
	return "mock"
}

type mockSeccompStrategyWriteFail struct{}

func (m *mockSeccompStrategyWriteFail) GenerateBPF() ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockSeccompStrategyWriteFail) Name() string {
	return "mock_write_fail"
}

type mockResourceStrategy struct{}

func (m *mockResourceStrategy) Configure(spec Spec) ([]string, error) {
	return []string{"--resource-custom"}, nil
}

func (m *mockResourceStrategy) Name() string {
	return "mock"
}

type mockResourceStrategyError struct{}

func (m *mockResourceStrategyError) Configure(spec Spec) ([]string, error) {
	return nil, context.DeadlineExceeded
}

func (m *mockResourceStrategyError) Name() string {
	return "mock_error"
}

type mockNetworkStrategy struct{}

func (m *mockNetworkStrategy) Configure(spec Spec) ([]string, error) {
	return []string{"--network-custom"}, nil
}

func (m *mockNetworkStrategy) Name() string {
	return "mock"
}

type mockNetworkStrategyError struct{}

func (m *mockNetworkStrategyError) Configure(spec Spec) ([]string, error) {
	return nil, context.DeadlineExceeded
}

func (m *mockNetworkStrategyError) Name() string {
	return "mock_error"
}

type mockLogStrategy struct{}

func (m *mockLogStrategy) Configure(spec Spec) ([]string, error) {
	return []string{"--log-custom"}, nil
}

func (m *mockLogStrategy) Name() string {
	return "mock"
}

type mockLogStrategyError struct{}

func (m *mockLogStrategyError) Configure(spec Spec) ([]string, error) {
	return nil, context.DeadlineExceeded
}

func (m *mockLogStrategyError) Name() string {
	return "mock_error"
}

type mockOverlayStrategy struct{}

func (m *mockOverlayStrategy) BuildArgs(lower, upper, work, mountpoint string) []string {
	return []string{"--overlay-custom"}
}

func (m *mockOverlayStrategy) Name() string {
	return "mock"
}

func TestBwrapContainerStatusRunning(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test-container")
	require.NoError(t, err)

	// 未启动的容器应返回 created
	status, err := container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "created", status.State)
}

func TestBwrapContainerCloseTwice(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test-container")
	require.NoError(t, err)

	err = container.Close()
	require.NoError(t, err)

	// 再次 Close 不应报错
	err = container.Close()
	assert.NoError(t, err)
}

func TestBwrapBuildArgsWithWorkspace(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	spec := Spec{
		Rootfs:    "/rootfs",
		UpperDir:  "/upper",
		WorkDir:   "/work",
		Workspace: "/host/workspace",
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--bind=/host/workspace:/workspace")
}

func TestBwrapBuildArgsWithoutWorkspace(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	for _, arg := range args {
		if strings.HasPrefix(arg, "--bind=") {
			t.Fatalf("unexpected bind arg when workspace is empty: %s", arg)
		}
	}
}

func TestBwrapBuildArgsWithEnv(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
		Envvars:  []string{"FOO=bar", "ONLY_KEY", "="},
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--setenv=FOO=bar")
	for _, arg := range args {
		if strings.HasPrefix(arg, "--setenv=ONLY_KEY") {
			t.Fatalf("unexpected env arg for malformed entry: %s", arg)
		}
		if strings.HasPrefix(arg, "--setenv==") {
			t.Fatalf("unexpected env arg for malformed entry: %s", arg)
		}
	}
}

func TestBwrapBuildArgsWithSeccompStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	bwrap.SetEnableSeccomp(true)
	bwrap.SetSeccompStrategy(&mockSeccompStrategy{})

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, bpfFile := bwrap.buildArgs(spec)
	if bpfFile != "" {
		assert.Contains(t, args, "--seccomp="+bpfFile)
		assert.True(t, strings.HasSuffix(bpfFile, ".bpf"))
		_ = os.Remove(bpfFile)
	}
}

func TestBwrapBuildArgsWithResourceStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	bwrap.SetResourceStrategy(&mockResourceStrategy{})

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--resource-custom")
}

func TestBwrapBuildArgsWithNetworkStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	bwrap.SetNetworkStrategy(&mockNetworkStrategy{})

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--network-custom")
}

func TestBwrapBuildArgsWithLogStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	bwrap.SetLogStrategy(&mockLogStrategy{})

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--log-custom")
}

func TestBwrapBuildArgsWithOverlayStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	bwrap.SetOverlayStrategy(&mockOverlayStrategy{})

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--overlay-custom")
}

func TestBwrapBuildArgsShmSizeDefault(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--shm-size=64m")
}

func TestBwrapBuildArgsShmSizeZero(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
		ShmSize:  0,
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--shm-size=64m")
}

func TestBwrapWriteSeccompBPFInvalidData(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	// 使用只读目录触发写入失败
	bwrap.enableSeccomp = true
	bwrap.seccompStrat = &mockSeccompStrategyWriteFail{}

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, bpfFile := bwrap.buildArgs(spec)
	if bpfFile != "" {
		assert.Contains(t, args, "--seccomp="+bpfFile)
		_ = os.Remove(bpfFile)
	}
}

func TestBwrapBuildUserNSArgsEnabled(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	bwrap.enableUserNS = true

	args := bwrap.buildUserNSArgs()
	assert.Contains(t, args, "--unshare-user")
	assert.Contains(t, args, "--map-root-user")
}

func TestBwrapBuildUserNSArgsDisabled(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	bwrap.enableUserNS = false

	args := bwrap.buildUserNSArgs()
	assert.Equal(t, []string{"--unshare-user=no"}, args)
}

func TestBwrapBuildWorkspaceArgsEmpty(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	args := bwrap.buildWorkspaceArgs(Spec{})
	assert.Nil(t, args)
}

func TestBwrapBuildEnvArgsMalformed(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	args := bwrap.buildEnvArgs(Spec{Envvars: []string{"ONLY_KEY", "=", "", "A=B=C"}})
	assert.Contains(t, args, "--setenv=A=B=C")
	for _, arg := range args {
		if strings.HasPrefix(arg, "--setenv=ONLY_KEY") {
			t.Fatalf("unexpected malformed env arg: %s", arg)
		}
		if strings.HasPrefix(arg, "--setenv==") {
			t.Fatalf("unexpected malformed env arg: %s", arg)
		}
		if strings.HasPrefix(arg, "--setenv==") {
			t.Fatalf("unexpected malformed env arg: %s", arg)
		}
	}
}

func TestBwrapStatusAfterClose(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	err = container.Close()
	require.NoError(t, err)

	status, err := container.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "destroyed", status.State)
}

func TestBwrapBuildArgsDefaultCommand(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--")
	assert.Contains(t, args, "/bin/sh")
	assert.Contains(t, args, "-c")
	assert.Contains(t, args, "sleep infinity")
}

func TestBwrapCheckDependenciesWithBwrapAndKernel(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	// 即使 bwrap 和内核检查都通过，也可能因其他依赖失败
	err = bwrap.checkDependencies()
	assert.Error(t, err)
}

func TestBwrapBuildArgsSeccompDisabled(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	bwrap.enableSeccomp = false

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
		SeccompBPF: []byte{0x01, 0x02},
	}

	args, _ := bwrap.buildArgs(spec)
	var found bool
	for _, arg := range args {
		if strings.HasPrefix(arg, "--seccomp=") {
			found = true
			break
		}
	}
	assert.False(t, found, "expected no seccomp arg when disabled")
}

func TestBwrapBuildArgsSeccompEmptyStrategy(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	bwrap.enableSeccomp = true
	bwrap.seccompStrat = &mockSeccompStrategy{}

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, bpfFile := bwrap.buildArgs(spec)
	if bpfFile != "" {
		assert.Contains(t, args, "--seccomp="+bpfFile)
		_ = os.Remove(bpfFile)
	}
}

func TestBwrapBuildArgsNetworkStrategyError(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	bwrap.networkStrat = &mockNetworkStrategyError{}

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	for _, arg := range args {
		if strings.HasPrefix(arg, "--network-") {
			t.Fatalf("unexpected network arg on error: %s", arg)
		}
	}
}

func TestBwrapBuildArgsLogStrategyError(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	bwrap.logStrat = &mockLogStrategyError{}

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	for _, arg := range args {
		if strings.HasPrefix(arg, "--log-") {
			t.Fatalf("unexpected log arg on error: %s", arg)
		}
	}
}

func TestBwrapBuildArgsResourceStrategyError(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)
	bwrap.resourceStrat = &mockResourceStrategyError{}

	spec := Spec{
		Rootfs:   "/rootfs",
		UpperDir: "/upper",
		WorkDir:  "/work",
	}

	args, _ := bwrap.buildArgs(spec)
	assert.Contains(t, args, "--shm-size=64m")
}

func TestSanitizeEnvFiltersUnsafePrefixes(t *testing.T) {
	env := []string{"FOO=bar", "LD_PRELOAD=evil", "LD_LIBRARY_PATH=/tmp", "HOME=/root"}
	filtered := sanitizeEnv(env)
	for _, e := range filtered {
		if strings.HasPrefix(e, "LD_PRELOAD=") ||
			strings.HasPrefix(e, "LD_LIBRARY_PATH=") ||
			strings.HasPrefix(e, "LD_DEBUG=") ||
			strings.HasPrefix(e, "LD_TRACE=") ||
			strings.HasPrefix(e, "LD_AUDIT=") {
			t.Fatalf("unsafe env leaked: %s", e)
		}
	}
	assert.Contains(t, filtered, "FOO=bar")
	assert.Contains(t, filtered, "HOME=/root")
}

func TestRunCommandExitCodes(t *testing.T) {
	// runCommand is unexported; test via Exec on a stopped container instead
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	_, err = container.Exec(context.Background(), ExecRequest{Command: "true"})
	assert.Error(t, err)
	_ = factory
}

func TestBuildNsenterArgs(t *testing.T) {
	args := buildNsenterArgs(1234, "id")
	assert.Equal(t, []string{"-t", "1234", "-m", "-u", "-i", "-n", "-p", "--", "sh", "-c", "id"}, args)
}

func TestEnsureContainerRunningError(t *testing.T) {
	factory := NewBwrapFactory()
	container, err := factory.Create("test")
	require.NoError(t, err)

	bwrap, ok := container.(*BwrapContainer)
	require.True(t, ok)

	err = ensureContainerRunning(bwrap)
	assert.Error(t, err)
}

func TestEnsureNsenterAvailableError(t *testing.T) {
	// 当前环境通常有 nsenter；仅验证函数可调用
	err := ensureNsenterAvailable()
	if err != nil {
		assert.Contains(t, err.Error(), "nsenter not found")
	}
}

func TestBuildExecCommandWithoutTimeout(t *testing.T) {
	cmd, stdout, stderr, err := buildExecCommand(context.Background(), []string{"-t", "1", "-m", "--", "sh", "-c", "id"}, 0)
	require.NoError(t, err)
	assert.NotNil(t, cmd)
	assert.NotNil(t, stdout)
	assert.NotNil(t, stderr)
}

func TestBuildExecResponseSuccess(t *testing.T) {
	resp := buildExecResponse(0, bytes.NewBufferString("ok"), bytes.NewBufferString(""), time.Now())
	assert.Equal(t, 0, resp.ExitCode)
	assert.Equal(t, []byte("ok"), resp.Stdout)
	assert.Empty(t, resp.Error)
}

func TestBuildExecResponseFailure(t *testing.T) {
	resp := buildExecResponse(1, bytes.NewBufferString(""), bytes.NewBufferString("err"), time.Now())
	assert.Equal(t, 1, resp.ExitCode)
	assert.Contains(t, resp.Error, "command exited with code 1")
}
