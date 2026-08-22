package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"keeper/internal/bootstrap"
	"keeper/internal/log"
)

// isBwrapSupported 检查当前环境是否支持 bwrap
func isBwrapSupported() bool {
	result := bootstrap.ProbeEnvironment()
	return result.OverlayUserNS && result.BwrapAvailable
}

// TestBwrapContainerBuildArgs 测试 bwrap 参数构建
func TestBwrapContainerBuildArgs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-bwrap-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// 创建必要的目录结构
	rootfs := filepath.Join(tmpDir, "rootfs")
	upper := filepath.Join(tmpDir, "upper")
	work := filepath.Join(tmpDir, "work")
	workspace := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(rootfs, 0755))
	require.NoError(t, os.MkdirAll(upper, 0755))
	require.NoError(t, os.MkdirAll(work, 0755))
	require.NoError(t, os.MkdirAll(workspace, 0755))

	c := &BwrapContainer{
		name:   "test-container",
		logger: &testLogger{},
	}

	spec := ContainerSpec{
		Name:      "test-container",
		Rootfs:    rootfs,
		UpperDir:  upper,
		WorkDir:   work,
		Workspace: workspace,
		ShmSize:   64,
		Envvars:   []string{"AGENT_NAME=test", "TEST_MODE=1"},
		Ports:     []PortMapping{{Host: 8080, Container: 80}},
	}

	args, bpfFile := c.buildArgs(spec)

	// 验证基本参数
	require.Contains(t, args, "--unshare-all")
	require.Contains(t, args, "--die-with-parent")
	require.Contains(t, args, "--new-session")

	// 验证 OverlayFS 参数
	require.Contains(t, args, "--ro-bind")
	require.Contains(t, args, "--bind")
	require.Contains(t, args, "--overlay")

	// 验证共享内存
	require.Contains(t, args, "--shm-size=64m")

	// 验证工作区绑定
	require.Contains(t, args, "--bind="+workspace+":/workspace")

	// 验证环境变量
	require.Contains(t, args, "--setenv=AGENT_NAME=test")
	require.Contains(t, args, "--setenv=TEST_MODE=1")

	// 验证默认 shell
	require.Contains(t, args, "--")
	require.Contains(t, args, "/bin/sh")
	require.Contains(t, args, "-c")
	require.Contains(t, args, "sleep infinity")

	// 验证 Seccomp BPF 文件
	if bpfFile != "" {
		// 如果生成了 BPF 文件，验证它存在且包含数据
		_, err := os.Stat(bpfFile)
		require.NoError(t, err, "BPF file should exist")
		data, err := os.ReadFile(bpfFile)
		require.NoError(t, err)
		require.Greater(t, len(data), 0, "BPF file should not be empty")
	}
}

// TestBwrapContainerBuildArgsWithCustomBPF 测试自定义 BPF
func TestBwrapContainerBuildArgsWithCustomBPF(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-bwrap-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rootfs := filepath.Join(tmpDir, "rootfs")
	upper := filepath.Join(tmpDir, "upper")
	work := filepath.Join(tmpDir, "work")
	require.NoError(t, os.MkdirAll(rootfs, 0755))
	require.NoError(t, os.MkdirAll(upper, 0755))
	require.NoError(t, os.MkdirAll(work, 0755))

	c := &BwrapContainer{
		name:   "test-container",
		logger: &testLogger{},
	}

	// 自定义 BPF 数据
	customBPF := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00} // 简单的 RET ALLOW

	spec := ContainerSpec{
		Name:       "test-container",
		Rootfs:     rootfs,
		UpperDir:   upper,
		WorkDir:    work,
		SeccompBPF: customBPF,
	}

	args, bpfFile := c.buildArgs(spec)

	// 验证 BPF 文件参数
	require.NotEmpty(t, bpfFile, "BPF file path should not be empty")
	require.Contains(t, args, "--seccomp="+bpfFile)

	// 验证 BPF 文件内容
	data, err := os.ReadFile(bpfFile)
	require.NoError(t, err)
	assert.Equal(t, customBPF, data, "BPF file should contain custom data")
}

// TestBwrapContainerStatus 测试 bwrap Status 方法
func TestBwrapContainerStatus(t *testing.T) {
	// 测试未启动的容器
	c := &BwrapContainer{
		name: "test-container",
		status: ContainerStatus{
			State: "created",
			PID:   0,
		},
		logger: &testLogger{},
	}

	status, err := c.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "created", status.State)
	assert.Equal(t, 0, status.PID)

	// 测试已停止的容器（cmd 为 nil）
	c2 := &BwrapContainer{
		name:   "test-container-2",
		status: ContainerStatus{State: "stopped"},
		logger: &testLogger{},
	}

	status, err = c2.Status(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "stopped", status.State)
}

// TestBwrapContainerType 测试 Type 方法
func TestBwrapContainerType(t *testing.T) {
	factory := NewBwrapFactory()
	require.NotNil(t, factory)
	assert.Equal(t, "bwrap", factory.Type())
}

// TestContainerSpecValidation 测试容器规格验证
func TestContainerSpecValidation(t *testing.T) {
	tests := []struct {
		name      string
		spec      ContainerSpec
		expectErr bool
	}{
		{
			name: "valid spec",
			spec: ContainerSpec{
				Name:     "test",
				Rootfs:   "/tmp/rootfs",
				UpperDir: "/tmp/upper",
				WorkDir:  "/tmp/work",
			},
			expectErr: false,
		},
		{
			name: "missing rootfs",
			spec: ContainerSpec{
				Name:     "test",
				UpperDir: "/tmp/upper",
				WorkDir:  "/tmp/work",
			},
			expectErr: false, // 当前不验证必填字段
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 构建参数应该不会panic
			c := &BwrapContainer{logger: &testLogger{}}
			args, _ := c.buildArgs(tt.spec)
			assert.NotNil(t, args)
		})
	}
}

// TestBwrapArgsOverlayFormat 测试 Overlay 参数格式
func TestBwrapArgsOverlayFormat(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-bwrap-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rootfs := filepath.Join(tmpDir, "rootfs")
	upper := filepath.Join(tmpDir, "upper")
	work := filepath.Join(tmpDir, "work")
	require.NoError(t, os.MkdirAll(rootfs, 0755))
	require.NoError(t, os.MkdirAll(upper, 0755))
	require.NoError(t, os.MkdirAll(work, 0755))

	c := &BwrapContainer{logger: &testLogger{}}
	spec := ContainerSpec{
		Name:     "test",
		Rootfs:   rootfs,
		UpperDir: upper,
		WorkDir:  work,
	}

	args, _ := c.buildArgs(spec)

	// 查找 overlay 参数
	var overlayMountIdx int
	var overlayValueIdx int
	found := false
	for i, arg := range args {
		if arg == "--overlay" {
			overlayMountIdx = i
			overlayValueIdx = i + 1
			found = true
			break
		}
	}
	require.True(t, found, "should contain --overlay")

	// 验证 overlay 挂载点
	assert.Equal(t, "/", args[overlayMountIdx+1], "overlay mount point should be /")

	// 验证 overlay 参数格式：lower:upper:work
	overlayValue := args[overlayValueIdx+1]
	parts := strings.Split(overlayValue, ":")
	require.Len(t, parts, 3, "overlay should have 3 parts")
	assert.Equal(t, "/lower", parts[0])
	assert.Equal(t, "/upper", parts[1])
	assert.Equal(t, "/work", parts[2])
}

// TestBwrapArgsSeccompDisabled 测试 Seccomp 禁用情况
func TestBwrapArgsSeccompDisabled(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-bwrap-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rootfs := filepath.Join(tmpDir, "rootfs")
	upper := filepath.Join(tmpDir, "upper")
	work := filepath.Join(tmpDir, "work")
	require.NoError(t, os.MkdirAll(rootfs, 0755))
	require.NoError(t, os.MkdirAll(upper, 0755))
	require.NoError(t, os.MkdirAll(work, 0755))

	c := &BwrapContainer{logger: &testLogger{}}
	spec := ContainerSpec{
		Name:     "test",
		Rootfs:   rootfs,
		UpperDir: upper,
		WorkDir:  work,
		// SeccompBPF 为空
	}

	args, bpfFile := c.buildArgs(spec)

	// 不应该包含 --seccomp 参数
	for _, arg := range args {
		assert.False(t, strings.HasPrefix(arg, "--seccomp="), "should not contain --seccomp when BPF is empty")
	}
	assert.Empty(t, bpfFile, "BPF file path should be empty")
}

// TestBwrapContainerStartRequiresBwrap 测试启动需要 bwrap
func TestBwrapContainerStartRequiresBwrap(t *testing.T) {
	// 这个测试验证启动逻辑，但由于内核限制，我们只测试 checkDependencies
	c := &BwrapContainer{
		name:   "test-container",
		logger: &testLogger{},
	}

	// 在当前环境，可能因为内核或 bwrap 缺失而失败
	err := c.checkDependencies()
	if err != nil {
		// 错误应该是关于 bwrap 或内核的
		msg := err.Error()
		assert.True(t,
			strings.Contains(msg, "bwrap") || strings.Contains(msg, "kernel"),
			"error should mention bwrap or kernel support: %s", msg)
	}
}

// TestBwrapContainerCheckKernelSupport 测试内核支持检查
func TestBwrapContainerCheckKernelSupport(t *testing.T) {
	c := &BwrapContainer{
		name:   "test-container",
		logger: &testLogger{},
	}

	supported := c.checkKernelSupport()
	t.Logf("Kernel support: %v", supported)
	// 不强制要求 true，因为取决于当前环境
}

// TestReadProcessStatusSuccess 测试读取进程状态成功
func TestReadProcessStatusSuccess(t *testing.T) {
	// 读取当前进程的状态
	pid := os.Getpid()
	status, err := readProcessStatus(pid)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.GreaterOrEqual(t, status.Uptime, time.Duration(0))
}

// TestReadProcessStatusInvalidPid 测试读取无效 PID
func TestReadProcessStatusInvalidPid(t *testing.T) {
	status, err := readProcessStatus(999999)
	require.Error(t, err)
	assert.Nil(t, status)
}

// TestReadSystemBootTimeSuccess 测试读取系统启动时间成功
func TestReadSystemBootTimeSuccess(t *testing.T) {
	btime, err := readSystemBootTime()
	require.NoError(t, err)
	assert.Greater(t, btime, int64(0))
	assert.Less(t, btime, time.Now().Unix())
}

// TestWriteSeccompBPFPreGenerated 测试写入预生成的 BPF
func TestWriteSeccompBPFPreGenerated(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-bpf-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	bpfFile := filepath.Join(tmpDir, "test.bpf")
	customBPF := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	err = writeSeccompBPF(bpfFile, customBPF)
	require.NoError(t, err)

	data, err := os.ReadFile(bpfFile)
	require.NoError(t, err)
	assert.Equal(t, customBPF, data)
}

// TestWriteSeccompBPFAutoGenerated 测试自动生成 BPF
func TestWriteSeccompBPFAutoGenerated(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-bpf-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	bpfFile := filepath.Join(tmpDir, "test.bpf")

	// 传入空 BPF，应该自动生成
	err = writeSeccompBPF(bpfFile, []byte{})
	require.NoError(t, err)

	data, err := os.ReadFile(bpfFile)
	require.NoError(t, err)
	assert.Greater(t, len(data), 0, "generated BPF should not be empty")
	// BPF 应该是 8 字节对齐
	assert.Equal(t, 0, len(data)%8)
}

// TestWriteSeccompBPFInvalidPath 测试写入无效路径
func TestWriteSeccompBPFInvalidPath(t *testing.T) {
	err := writeSeccompBPF("/nonexistent/dir/test.bpf", []byte{})
	require.Error(t, err)
}

// TestBwrapContainerCloseNoRunner 测试 Close 方法（无 runner）
func TestBwrapContainerCloseNoRunner(t *testing.T) {
	c := &BwrapContainer{
		name:   "test-container",
		logger: &testLogger{},
	}

	// 未启动的容器 Close 应该成功
	err := c.Close()
	require.NoError(t, err)
	assert.Equal(t, "destroyed", c.status.State)
}

// TestBwrapContainerStartCheckDependenciesError 测试启动时依赖检查失败
func TestBwrapContainerStartCheckDependenciesError(t *testing.T) {
	c := &BwrapContainer{
		name:   "test-container",
		logger: &testLogger{},
	}

	// 在当前环境，Start 会因为内核或 bwrap 缺失而失败
	ctx := context.Background()
	spec := ContainerSpec{
		Name:     "test-container",
		Rootfs:   "/tmp/rootfs",
		UpperDir: "/tmp/upper",
		WorkDir:  "/tmp/work",
	}

	_, err := c.Start(ctx, spec)
	// 期望返回错误（当前环境内核 5.10 不支持 CONFIG_OVERLAY_FS_USERNS）
	assert.Error(t, err)
	if err != nil {
		msg := err.Error()
		assert.True(t,
			strings.Contains(msg, "bwrap") || strings.Contains(msg, "kernel"),
			"error should mention bwrap or kernel support: %s", msg)
	}
}

// TestBwrapContainerStopNotRunning 测试停止未运行的容器
func TestBwrapContainerStopNotRunning(t *testing.T) {
	c := &BwrapContainer{
		name:   "test-container",
		status: ContainerStatus{State: "created"},
		logger: &testLogger{},
	}

	// 未运行的容器 Stop 应该成功（静默返回）
	ctx := context.Background()
	err := c.Stop(ctx, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, "created", c.status.State) // 状态不应改变
}

// TestBwrapContainerExecNotRunning 测试在未运行的容器中执行命令
func TestBwrapContainerExecNotRunning(t *testing.T) {
	c := &BwrapContainer{
		name:   "test-container",
		status: ContainerStatus{State: "created"},
		logger: &testLogger{},
	}

	ctx := context.Background()
	req := ExecRequest{
		Command: "echo hello",
		Timeout: 30 * time.Second,
	}

	resp, err := c.Exec(ctx, req)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "container not running")
}

// TestBwrapContainerStatusRunning 测试运行中容器的状态
func TestBwrapContainerStatusRunning(t *testing.T) {
	// 模拟一个正在运行的容器
	c := &BwrapContainer{
		name: "test-container",
		status: ContainerStatus{
			State:  "running",
			PID:    12345,
			PGID:   12345,
			Uptime: 10 * time.Second,
		},
		logger: &testLogger{},
	}

	ctx := context.Background()
	status, err := c.Status(ctx)
	require.NoError(t, err)
	assert.Equal(t, "running", status.State)
	assert.Equal(t, 12345, status.PID)
}

// TestBwrapContainerLifecycle 测试 bwrap 容器完整生命周期（依赖检查失败场景）
func TestBwrapContainerLifecycle(t *testing.T) {
	c := &BwrapContainer{
		name:   "lifecycle-test",
		logger: &testLogger{},
	}

	ctx := context.Background()
	spec := ContainerSpec{
		Name:     "lifecycle-test",
		Rootfs:   "/tmp/rootfs",
		UpperDir: "/tmp/upper",
		WorkDir:  "/tmp/work",
		ShmSize:  64,
		Envvars:  []string{"TEST=1"},
	}

	if isBwrapSupported() {
		// 环境支持：创建必要目录并测试完整生命周期
		require.NoError(t, os.MkdirAll(spec.Rootfs, 0755))
		require.NoError(t, os.MkdirAll(spec.UpperDir, 0755))
		require.NoError(t, os.MkdirAll(spec.WorkDir, 0755))

		// 1. 启动
		pid, err := c.Start(ctx, spec)
		require.NoError(t, err)
		assert.Greater(t, pid, 0)

		// 2. 状态查询
		status, err := c.Status(ctx)
		require.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, "running", status.State)
		assert.Equal(t, pid, status.PID)

		// 3. 执行命令
		resp, err := c.Exec(ctx, ExecRequest{
			Command: "echo hello-from-bwrap",
			Timeout: 5 * time.Second,
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, 0, resp.ExitCode)
		assert.Contains(t, string(resp.Stdout), "hello-from-bwrap")

		// 4. 停止
		err = c.Stop(ctx, 5*time.Second)
		require.NoError(t, err)

		// 5. 关闭
		err = c.Close()
		require.NoError(t, err)
		assert.Equal(t, "destroyed", c.status.State)
	} else {
		// 环境不支持：验证错误处理路径
		pid, err := c.Start(ctx, spec)
		assert.Error(t, err)
		assert.Equal(t, 0, pid)
		if err != nil {
			msg := err.Error()
			assert.True(t,
				strings.Contains(msg, "bwrap") || strings.Contains(msg, "kernel") || strings.Contains(msg, "CONFIG_OVERLAY_FS_USERNS"),
				"error should mention bwrap or kernel support: %s", msg)
		}

		status, err := c.Status(ctx)
		require.NoError(t, err)
		assert.NotNil(t, status)

		resp, err := c.Exec(ctx, ExecRequest{
			Command: "echo test",
			Timeout: 5 * time.Second,
		})
		assert.Error(t, err)
		assert.Nil(t, resp)

		err = c.Stop(ctx, 5*time.Second)
		require.NoError(t, err)

		err = c.Close()
		require.NoError(t, err)
		assert.Equal(t, "destroyed", c.status.State)
	}
}

// TestBwrapContainerBuildArgsWithWorkspace 测试带工作区的 buildArgs
func TestBwrapContainerBuildArgsWithWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-bwrap-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rootfs := filepath.Join(tmpDir, "rootfs")
	upper := filepath.Join(tmpDir, "upper")
	work := filepath.Join(tmpDir, "work")
	workspace := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(rootfs, 0755))
	require.NoError(t, os.MkdirAll(upper, 0755))
	require.NoError(t, os.MkdirAll(work, 0755))
	require.NoError(t, os.MkdirAll(workspace, 0755))

	c := &BwrapContainer{logger: &testLogger{}}
	spec := ContainerSpec{
		Name:      "test",
		Rootfs:    rootfs,
		UpperDir:  upper,
		WorkDir:   work,
		Workspace: workspace,
		ShmSize:   128,
		Envvars:   []string{"KEY=VALUE"},
	}

	args, bpfFile := c.buildArgs(spec)

	// 验证工作区绑定
	assert.Contains(t, args, "--bind="+workspace+":/workspace")
	// 验证共享内存大小
	assert.Contains(t, args, "--shm-size=128m")
	// 验证环境变量
	assert.Contains(t, args, "--setenv=KEY=VALUE")
	// 验证默认 shell
	assert.Contains(t, args, "/bin/sh")
	assert.Contains(t, args, "-c")
	assert.Contains(t, args, "sleep infinity")
	// BPF 文件应为空（未提供自定义 BPF）
	assert.Empty(t, bpfFile)
}

// TestBwrapContainerBuildArgsWithStrategies 测试 buildArgs 中策略集成
func TestBwrapContainerBuildArgsWithStrategies(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "keeper-bwrap-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	rootfs := filepath.Join(tmpDir, "rootfs")
	upper := filepath.Join(tmpDir, "upper")
	work := filepath.Join(tmpDir, "work")
	workspace := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(rootfs, 0755))
	require.NoError(t, os.MkdirAll(upper, 0755))
	require.NoError(t, os.MkdirAll(work, 0755))
	require.NoError(t, os.MkdirAll(workspace, 0755))

	// 创建带自定义策略的容器
	c := &BwrapContainer{
		name:   "strategy-test",
		logger: &testLogger{},
		networkStrat: &CustomNetworkStrategy{
			nameservers: []string{"1.1.1.1", "8.8.8.8"},
		},
		resourceStrat: &CustomResourceStrategy{
			shmSize: 128,
		},
		logStrat: &CustomLogStrategy{
			level: "debug",
		},
	}

	spec := ContainerSpec{
		Name:      "strategy-test",
		Rootfs:    rootfs,
		UpperDir:  upper,
		WorkDir:   work,
		Workspace: workspace,
		ShmSize:   64, // 自定义策略应覆盖此值
		Envvars:   []string{"KEY=VALUE"},
	}

	args, bpfFile := c.buildArgs(spec)

	// 验证网络策略参数
	assert.Contains(t, args, "--setenv=NAMESERVER=1.1.1.1")
	assert.Contains(t, args, "--setenv=NAMESERVER=8.8.8.8")

	// 验证资源策略参数（自定义策略应覆盖 ShmSize）
	assert.Contains(t, args, "--shm-size=128m")

	// 验证日志策略参数
	assert.Contains(t, args, "--log-level=debug")

	// 验证环境变量
	assert.Contains(t, args, "--setenv=KEY=VALUE")

	// BPF 文件应为空
	assert.Empty(t, bpfFile)
}

// CustomNetworkStrategy 自定义网络策略
type CustomNetworkStrategy struct {
	nameservers []string
}

func (s *CustomNetworkStrategy) Name() string { return "custom" }
func (s *CustomNetworkStrategy) Configure(spec ContainerSpec) ([]string, error) {
	args := []string{}
	for _, ns := range s.nameservers {
		args = append(args, "--setenv=NAMESERVER="+ns)
	}
	return args, nil
}

// CustomResourceStrategy 自定义资源策略
type CustomResourceStrategy struct {
	shmSize uint32
}

func (s *CustomResourceStrategy) Name() string { return "custom" }
func (s *CustomResourceStrategy) Configure(spec ContainerSpec) ([]string, error) {
	return []string{fmt.Sprintf("--shm-size=%dm", s.shmSize)}, nil
}

// CustomLogStrategy 自定义日志策略
type CustomLogStrategy struct {
	level string
}

func (s *CustomLogStrategy) Name() string { return "custom" }
func (s *CustomLogStrategy) Configure(spec ContainerSpec) ([]string, error) {
	return []string{"--log-level=" + s.level}, nil
}

type testLogger struct{}

func (l *testLogger) Debug(msg string, fields ...log.Field)     {}
func (l *testLogger) Info(msg string, fields ...log.Field)      {}
func (l *testLogger) Warn(msg string, fields ...log.Field)      {}
func (l *testLogger) Error(msg string, fields ...log.Field)     {}
func (l *testLogger) Fatal(msg string, fields ...log.Field)     {}
func (l *testLogger) WithFields(fields ...log.Field) log.Logger { return l }
func (l *testLogger) WithTrace(traceID, spanID string) log.Logger { return l }
func (l *testLogger) Sync() error                               { return nil }
func (l *testLogger) SetOutput(w io.Writer)                     {}
