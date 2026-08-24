package container

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"keeper/internal/bootstrap"
	"keeper/internal/errors"
	"keeper/internal/log"
	"keeper/internal/seccomp"
	"keeper/pkg/config"
)

var errOverlayUnsupported = errors.NewKeeperError(errors.ErrCodeFatalKernel, "OverlayFS+UserNS 不可用，请确保宿主机内核支持 CONFIG_OVERLAY_FS_USERNS", nil)

const (
	stateStopped        = "stopped"
	runtimeTypeBwrap    = "bwrap"
	fatalBwrapExecState = "fatal_bwrap_exec"
)

// BwrapContainer bwrap 容器运行时实现
type BwrapContainer struct {
	name          string
	cmd           *exec.Cmd
	status        Status
	logger        log.Logger
	seccompStrat  SeccompStrategy
	overlayStrat  OverlayStrategy
	networkStrat  NetworkStrategy
	resourceStrat ResourceStrategy
	logStrat      LogStrategy

	// enableUserNS 是否启用 UserNS
	enableUserNS bool
	// enableSeccomp 是否启用 Seccomp
	enableSeccomp bool
}

// SetSeccompStrategy 设置 Seccomp 策略
func (c *BwrapContainer) SetSeccompStrategy(strategy SeccompStrategy) {
	c.seccompStrat = strategy
}

// SetOverlayStrategy 设置 Overlay 策略
func (c *BwrapContainer) SetOverlayStrategy(strategy OverlayStrategy) {
	c.overlayStrat = strategy
}

// SetNetworkStrategy 设置网络策略
func (c *BwrapContainer) SetNetworkStrategy(strategy NetworkStrategy) {
	c.networkStrat = strategy
}

// SetResourceStrategy 设置资源限制策略
func (c *BwrapContainer) SetResourceStrategy(strategy ResourceStrategy) {
	c.resourceStrat = strategy
}

// SetLogStrategy 设置日志策略
func (c *BwrapContainer) SetLogStrategy(strategy LogStrategy) {
	c.logStrat = strategy
}

// SetEnableUserNS 设置是否启用 UserNS
func (c *BwrapContainer) SetEnableUserNS(enable bool) {
	c.enableUserNS = enable
}

// SetEnableSeccomp 设置是否启用 Seccomp
func (c *BwrapContainer) SetEnableSeccomp(enable bool) {
	c.enableSeccomp = enable
}

// BwrapFactory implements the container.Factory interface using bubblewrap.
type BwrapFactory struct{}

// NewBwrapFactory creates a new BwrapFactory instance.
func NewBwrapFactory() *BwrapFactory {
	return &BwrapFactory{}
}

// Create creates a new bwrap-backed container runtime instance.
func (f *BwrapFactory) Create(name string) (Container, error) {
	logger := log.Global().WithFields(log.Field{Key: "container", Value: name})
	logger.Info("creating bwrap container")

	// 默认启用 UserNS 和 Seccomp（安全默认值）
	enableUserNS := true
	enableSeccomp := true

	// 尝试从全局配置读取 bwrap 配置
	if cfg, err := config.LoadDefaultIfExists(); err == nil && cfg != nil {
		enableUserNS = cfg.BwrapEnableUserNS
		enableSeccomp = cfg.BwrapEnableSeccomp
	}

	return &BwrapContainer{
		name:          name,
		logger:        logger,
		seccompStrat:  NewBPFGenerator(logger),
		overlayStrat:  NewOverlayBuilder(logger),
		networkStrat:  NewDefaultNetworkStrategy(logger),
		resourceStrat: NewDefaultResourceStrategy(logger),
		logStrat:      NewDefaultLogStrategy(logger),
		status: Status{
			State: "created",
		},
		enableUserNS:  enableUserNS,
		enableSeccomp: enableSeccomp,
	}, nil
}

// Type 返回运行时类型
func (f *BwrapFactory) Type() string {
	return runtimeTypeBwrap
}

// Start 启动容器
func (c *BwrapContainer) Start(ctx context.Context, spec Spec) (int, error) {
	c.logger.Info("starting container")
	startTime := time.Now()

	// 检查环境依赖
	if err := c.checkDependencies(); err != nil {
		c.status.State = fatalBwrapExecState
		RecordContainerStart(runtimeTypeBwrap, "error")
		return 0, err
	}

	// 执行 OverlayFS + UserNS 实际挂载探测（Dry-Run）
	if err := bootstrap.ProbeOverlayDryRun(); err != nil {
		c.status.State = "fatal_unsupported_kernel"
		RecordContainerStart(runtimeTypeBwrap, "error")
		return 0, errOverlayUnsupported
	}

	// 生成 sanitized resolv.conf（过滤 127.0.0.x + 网关 fallback）
	resolvPath, err := SanitizedResolvConf()
	if err != nil {
		c.logger.Warn("failed to create sanitized resolv.conf", log.Field{Key: "error", Value: err.Error()})
	} else {
		defer func() {
			_ = os.Remove(resolvPath)
		}()
	}

	// 构建 bwrap 参数，并收集 Seccomp BPF 临时文件路径
	args, bpfFile := c.buildArgs(spec)

	// 清理 Seccomp BPF 临时文件
	if bpfFile != "" {
		defer func() {
			if err := os.Remove(bpfFile); err != nil && !os.IsNotExist(err) {
				c.logger.Warn("failed to remove seccomp bpf file",
					log.Field{Key: "file", Value: bpfFile},
					log.Field{Key: "error", Value: err.Error()})
			}
		}()
	}

	// 注入 sanitized resolv.conf bind-mount
	if resolvPath != "" {
		args = append(args, "--bind", resolvPath, "/etc/resolv.conf")
	}

	// 创建命令
	cmd := buildBwrapCommand(ctx, args)

	// 启动进程
	if err := cmd.Start(); err != nil {
		c.status.State = fatalBwrapExecState
		RecordContainerStart(runtimeTypeBwrap, "error")
		return 0, fmt.Errorf("start bwrap: %w", err)
	}

	c.cmd = cmd
	c.finalizeStart(startTime, cmd, spec)
	return cmd.Process.Pid, nil
}

func (c *BwrapContainer) finalizeStart(startTime time.Time, cmd *exec.Cmd, spec Spec) {
	// 确定性获取 PGID：读取实际进程组 ID，而非假设等于 PID
	pgid := cmd.Process.Pid
	if gid, err := syscall.Getpgid(pgid); err == nil && gid > 0 {
		pgid = gid
	} else if err != nil {
		c.logger.Warn("failed to get pgid, falling back to pid",
			log.Field{Key: "pid", Value: cmd.Process.Pid},
			log.Field{Key: "error", Value: err.Error()})
	}

	c.status = Status{
		State:  "running",
		PID:    cmd.Process.Pid,
		PGID:   pgid,
		Uptime: 0,
		Ports:  spec.Ports,
	}

	RecordContainerStart(runtimeTypeBwrap, "success")
	RecordContainerStartDuration(runtimeTypeBwrap, time.Since(startTime).Seconds())
	SetContainerActive(runtimeTypeBwrap, 1)

	c.logger.Info("container started", log.Field{Key: "pid", Value: cmd.Process.Pid})
}

func buildBwrapCommand(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, runtimeTypeBwrap, args...) // #nosec G204
	cmd.Stdout = nil
	cmd.Stderr = nil
	// 防御性设置：强制 Go 运行时走 clone 而非 clone3
	cmd.Env = append(os.Environ(), "GODEBUG=clone3=0")
	return cmd
}

// Stop 停止容器
func (c *BwrapContainer) Stop(ctx context.Context, grace time.Duration) error {
	c.logger.Info("stopping container",
		log.Field{Key: "pid", Value: func() int {
			if c.cmd != nil && c.cmd.Process != nil {
				return c.cmd.Process.Pid
			}
			return 0
		}()},
		log.Field{Key: "grace", Value: grace})
	startTime := time.Now()

	if c.cmd == nil || c.cmd.Process == nil {
		c.logger.Warn("container not running")
		return nil
	}

	// 优雅关闭：先尝试 SIGTERM
	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		c.logger.Warn("failed to send SIGTERM", log.Field{Key: "error", Value: err.Error()})
	}

	// 等待优雅关闭或超时
	done := make(chan error, 1)
	go func() {
		_, err := c.cmd.Process.Wait()
		done <- err
	}()

	select {
	case <-time.After(grace):
		c.logger.Warn("grace period exceeded, force killing")
		if err := c.cmd.Process.Kill(); err != nil {
			c.logger.Error("error killing container", log.Field{Key: "error", Value: err.Error()})
			RecordContainerStop(runtimeTypeBwrap, "error")
			c.status.State = stateStopped
			c.status.PID = 0
			c.status.PGID = 0
			return fmt.Errorf("kill container: %w", err)
		}
		<-done // 等待进程完全退出
	case err := <-done:
		if err != nil {
			c.logger.Error("error waiting for container", log.Field{Key: "error", Value: err.Error()})
		}
	}

	c.status.State = stateStopped
	c.status.PID = 0
	c.status.PGID = 0

	RecordContainerStop(runtimeTypeBwrap, "success")
	RecordContainerStopDuration(runtimeTypeBwrap, time.Since(startTime).Seconds())
	SetContainerActive(runtimeTypeBwrap, 0)

	c.logger.Info("container stopped successfully")
	return nil
}

// Exec 在容器内执行命令
func (c *BwrapContainer) Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error) {
	if err := ensureContainerRunning(c); err != nil {
		return nil, err
	}

	startTime := time.Now()
	if err := ensureNsenterAvailable(); err != nil {
		return nil, err
	}

	args := buildNsenterArgs(c.cmd.Process.Pid, req.Command)
	cmd, stdout, stderr, err := buildExecCommand(ctx, args, req.Timeout)
	if err != nil {
		return nil, err
	}

	env := sanitizeEnv(req.Env)
	cmd.Env = env

	exitCode := runCommand(cmd)
	resp := buildExecResponse(exitCode, stdout, stderr, startTime)

	c.logger.Info("command executed",
		log.Field{Key: "exit_code", Value: exitCode},
		log.Field{Key: "stdout_len", Value: stdout.Len()},
		log.Field{Key: "stderr_len", Value: stderr.Len()})

	return resp, nil
}

func ensureContainerRunning(c *BwrapContainer) error {
	if c.cmd == nil || c.cmd.Process == nil {
		RecordContainerExec(runtimeTypeBwrap, "error")
		return errors.NewKeeperError(errors.ErrCodeContainer, "container not running", nil)
	}
	return nil
}

func ensureNsenterAvailable() error {
	if _, err := exec.LookPath("nsenter"); err != nil {
		RecordContainerExec(runtimeTypeBwrap, "error")
		return errors.NewKeeperError(errors.ErrCodeProcess, "nsenter not found, cannot exec into container", err)
	}
	return nil
}

func buildNsenterArgs(pid int, command string) []string {
	return []string{
		"-t", strconv.Itoa(pid),
		"-m", "-u", "-i", "-n", "-p",
		"--", "sh", "-c", command,
	}
}

func buildExecCommand(ctx context.Context, args []string, timeout time.Duration) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer, error) {
	cmd := exec.CommandContext(ctx, "nsenter", args...) // #nosec G204
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, "nsenter", args...) // #nosec G204
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}
	return cmd, &stdout, &stderr, nil
}

func sanitizeEnv(reqEnv []string) []string {
	env := make([]string, 0, len(os.Environ())+len(reqEnv))
	skipPrefixes := []string{"LD_PRELOAD", "LD_LIBRARY_PATH", "LD_DEBUG", "LD_TRACE", "LD_AUDIT"}
	for _, e := range os.Environ() {
		skip := false
		for _, prefix := range skipPrefixes {
			if strings.HasPrefix(e, prefix+"=") {
				skip = true
				break
			}
		}
		if !skip {
			env = append(env, e)
		}
	}
	return append(env, reqEnv...)
}

func runCommand(cmd *exec.Cmd) int {
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return exitCode
}

func buildExecResponse(exitCode int, stdout, stderr *bytes.Buffer, startTime time.Time) *ExecResponse {
	resp := &ExecResponse{
		ExitCode: exitCode,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
	}

	if exitCode != 0 {
		resp.Error = fmt.Sprintf("command exited with code %d", exitCode)
		RecordContainerExec(runtimeTypeBwrap, "error")
	} else {
		RecordContainerExec(runtimeTypeBwrap, "success")
	}
	RecordContainerExecDuration(runtimeTypeBwrap, time.Since(startTime).Seconds())
	return resp
}

// Status 查询容器状态
func (c *BwrapContainer) Status(ctx context.Context) (*Status, error) {
	if c.cmd == nil || c.cmd.Process == nil {
		// 未启动的容器返回当前状态（可能是 created 或 destroyed）
		return &c.status, nil
	}

	// 检查进程是否还在运行
	err := c.cmd.Process.Signal(syscall.Signal(0))
	if err != nil {
		c.status.State = stateStopped
		c.status.PID = 0
		c.status.PGID = 0
		c.logger.Warn("container process not found", log.Field{Key: "pid", Value: c.cmd.Process.Pid})
		return &c.status, nil
	}

	c.status.State = "running"
	c.status.PID = c.cmd.Process.Pid
	c.status.PGID = c.cmd.Process.Pid

	// 读取 /proc/<pid>/status 获取详细信息
	if status, err := readProcessStatus(c.cmd.Process.Pid); err == nil {
		c.status.Uptime = status.Uptime
	} else {
		c.logger.Debug("failed to read process status", log.Field{Key: "pid", Value: c.cmd.Process.Pid}, log.Field{Key: "error", Value: err.Error()})
	}

	c.logger.Debug("container status checked",
		log.Field{Key: "state", Value: c.status.State},
		log.Field{Key: "pid", Value: c.status.PID},
		log.Field{Key: "uptime", Value: c.status.Uptime})

	return &c.status, nil
}

// Close 清理资源
func (c *BwrapContainer) Close() error {
	c.logger.Info("closing container")
	c.status.State = "destroyed"
	return nil
}

// checkDependencies 检查环境依赖
func (c *BwrapContainer) checkDependencies() error {
	// 检查 bwrap 是否可用
	_, err := exec.LookPath(runtimeTypeBwrap)
	if err != nil {
		return errors.NewKeeperError(errors.ErrCodeFatalBwrap, "bwrap not found, please install bubblewrap", err)
	}

	// 检查内核支持
	if !c.checkKernelSupport() {
		return errors.NewKeeperError(errors.ErrCodeFatalKernel, "kernel does not support required features (CONFIG_OVERLAY_FS_USERNS)", nil)
	}

	return nil
}

// checkKernelSupport 检查内核支持
func (c *BwrapContainer) checkKernelSupport() bool {
	// 使用 bootstrap 包的环境探测功能
	// 注意：这里导入 bootstrap 包是为了复用探测逻辑
	// 实际部署时应该缓存探测结果，避免重复检测
	result := bootstrap.ProbeEnvironment()
	return result.OverlayUserNS && result.BwrapAvailable
}

// buildArgs 构建 bwrap 参数，返回参数列表和 Seccomp BPF 临时文件路径（如果有）
func (c *BwrapContainer) buildArgs(spec Spec) ([]string, string) {
	args := buildBasicArgs()
	args = append(args, c.buildUserNSArgs()...)
	args = append(args, c.buildOverlayArgs(spec)...)
	args = append(args, c.buildResourceArgs(spec)...)
	args = append(args, c.buildNetworkArgs()...)
	args = append(args, c.buildLogArgs()...)
	args = append(args, c.buildWorkspaceArgs(spec)...)
	args = append(args, c.buildEnvArgs(spec)...)
	bpfFile := c.buildSeccompArgs(spec)
	if bpfFile != "" {
		args = append(args, "--seccomp="+bpfFile)
	}

	// 默认 shell
	args = append(args, "--", "/bin/sh", "-c", "sleep infinity")
	return args, bpfFile
}

func buildBasicArgs() []string {
	return []string{
		"--unshare-all",
		"--die-with-parent",
		"--new-session",
	}
}

func (c *BwrapContainer) buildUserNSArgs() []string {
	if !c.enableUserNS {
		c.logger.Warn("UserNS disabled, container will run in host user namespace")
		return []string{"--unshare-user=no"}
	}

	c.logger.Debug("UserNS enabled with root mapping")
	return []string{"--unshare-user", "--map-root-user"}
}

func (c *BwrapContainer) buildOverlayArgs(spec Spec) []string {
	if c.overlayStrat != nil {
		return c.overlayStrat.BuildArgs(spec.Rootfs, spec.UpperDir, spec.WorkDir, "/")
	}

	return []string{
		"--ro-bind", spec.Rootfs, "/lower",
		"--bind", spec.UpperDir, "/upper",
		"--bind", spec.WorkDir, "/work",
		"--overlay", "/", "/lower:/upper:/work",
	}
}

func (c *BwrapContainer) buildResourceArgs(spec Spec) []string {
	if c.resourceStrat != nil {
		resourceArgs, err := c.resourceStrat.Configure(spec)
		if err == nil {
			return resourceArgs
		}
	}

	shmSize := spec.ShmSize
	if shmSize <= 0 {
		shmSize = 64
	}
	return []string{fmt.Sprintf("--shm-size=%dm", shmSize)}
}

func (c *BwrapContainer) buildNetworkArgs() []string {
	if c.networkStrat == nil {
		return nil
	}

	networkArgs, err := c.networkStrat.Configure(Spec{})
	if err == nil {
		return networkArgs
	}
	return nil
}

func (c *BwrapContainer) buildLogArgs() []string {
	if c.logStrat == nil {
		return nil
	}

	logArgs, err := c.logStrat.Configure(Spec{})
	if err == nil {
		return logArgs
	}
	return nil
}

func (c *BwrapContainer) buildWorkspaceArgs(spec Spec) []string {
	if spec.Workspace == "" {
		return nil
	}
	return []string{fmt.Sprintf("--bind=%s:/workspace", spec.Workspace)}
}

func (c *BwrapContainer) buildEnvArgs(spec Spec) []string {
	var args []string
	for _, env := range spec.Envvars {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			args = append(args, fmt.Sprintf("--setenv=%s=%s", parts[0], parts[1]))
		}
	}
	return args
}

func (c *BwrapContainer) buildSeccompArgs(spec Spec) string {
	if !c.enableSeccomp {
		c.logger.Info("seccomp disabled by configuration")
		return ""
	}

	if len(spec.SeccompBPF) > 0 {
		return c.writeSeccompBPFFile(spec.SeccompBPF)
	}

	if c.seccompStrat != nil {
		return c.writeSeccompBPFFileFromStrategy(c.seccompStrat)
	}

	return ""
}

func (c *BwrapContainer) writeSeccompBPFFile(bpfData []byte) string {
	bpfFile := filepath.Join(os.TempDir(), fmt.Sprintf("seccomp-%d.bpf", os.Getpid()))
	writeErr := os.WriteFile(bpfFile, bpfData, 0600)
	if writeErr == nil {
		return bpfFile
	}

	c.logger.Warn("failed to write seccomp bpf file", log.Field{Key: "error", Value: writeErr.Error()})
	return ""
}

func (c *BwrapContainer) writeSeccompBPFFileFromStrategy(strategy SeccompStrategy) string {
	bpfData, err := strategy.GenerateBPF()
	if err != nil || len(bpfData) == 0 {
		return ""
	}

	bpfFile := filepath.Join(os.TempDir(), fmt.Sprintf("seccomp-%d.bpf", os.Getpid()))
	writeErr := os.WriteFile(bpfFile, bpfData, 0600)
	if writeErr == nil {
		return bpfFile
	}

	c.logger.Warn("failed to write seccomp bpf file", log.Field{Key: "error", Value: writeErr.Error()})
	return ""
}

// 辅助函数

type processStatus struct {
	Uptime time.Duration
}

func readProcessStatus(pid int) (*processStatus, error) {
	// 从 /proc/<pid>/stat 读取进程启动时间
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return nil, err
	}

	// 解析 stat 文件，提取第22个字段（starttime）
	// 格式: pid (comm) state ppid pgrp session tty_nr tpgid flags minflt cminflt majflt cmajflt utime stimecutime cstime priority nice num_threads itrealvalue starttime vsize rss
	parts := strings.Fields(string(data))
	if len(parts) < 22 {
		return nil, fmt.Errorf("invalid stat format")
	}

	starttime, err := strconv.ParseInt(parts[21], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse starttime: %w", err)
	}

	// 读取系统启动时间（/proc/stat 中的 btime）
	btime, err := readSystemBootTime()
	if err != nil {
		return nil, fmt.Errorf("read boot time: %w", err)
	}

	// 读取时钟频率（默认 100 Hz）
	clkTick := int64(100)

	// 计算进程启动时间（秒）
	processStart := btime + starttime/clkTick

	// 计算 uptime
	now := time.Now().Unix()
	uptime := now - processStart
	if uptime < 0 {
		uptime = 0
	}

	return &processStatus{Uptime: time.Duration(uptime) * time.Second}, nil
}

// readSystemBootTime 读取系统启动时间
func readSystemBootTime() (int64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "btime ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				btime, err := strconv.ParseInt(parts[1], 10, 64)
				if err != nil {
					return 0, err
				}
				return btime, nil
			}
		}
	}
	return 0, fmt.Errorf("btime not found in /proc/stat")
}

func writeSeccompBPF(filename string, bpf []byte) error {
	// 如果传入的是预生成的 BPF 数据，直接写入
	if len(bpf) > 0 {
		return os.WriteFile(filename, bpf, 0600)
	}

	// 否则使用默认过滤器生成 BPF 程序
	filter := seccomp.NewDefaultFilter()
	generatedBPF, err := seccomp.GenerateBPF(filter)
	if err != nil {
		return fmt.Errorf("generate seccomp bpf: %w", err)
	}

	return os.WriteFile(filename, generatedBPF, 0600)
}
