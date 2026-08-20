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
)

// BwrapContainer bwrap 容器运行时实现
type BwrapContainer struct {
	name   string
	pid    int
	cmd    *exec.Cmd
	status ContainerStatus
	logger log.Logger
}

// BwrapFactory bwrap 容器运行时工厂
type BwrapFactory struct{}

// NewBwrapFactory 创建 bwrap 工厂
func NewBwrapFactory() *BwrapFactory {
	return &BwrapFactory{}
}

// Create 创建容器运行时实例
func (f *BwrapFactory) Create(name string) (Container, error) {
	logger := log.Global().WithFields(log.Field{Key: "container", Value: name})
	logger.Info("creating bwrap container")
	return &BwrapContainer{
		name:   name,
		logger: logger,
		status: ContainerStatus{
			State: "created",
		},
	}, nil
}

// Type 返回运行时类型
func (f *BwrapFactory) Type() string {
	return "bwrap"
}

// Start 启动容器
func (c *BwrapContainer) Start(ctx context.Context, spec ContainerSpec) (int, error) {
	c.logger.Info("starting container")

	// 检查环境依赖
	if err := c.checkDependencies(); err != nil {
		return 0, err
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

	// 创建命令
	cmd := exec.CommandContext(ctx, "bwrap", args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	// 启动进程
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start bwrap: %w", err)
	}

	c.cmd = cmd
	c.status = ContainerStatus{
		State:  "running",
		PID:    cmd.Process.Pid,
		PGID:   cmd.Process.Pid,
		Uptime: 0,
		Ports:  spec.Ports,
	}

	c.logger.Info("container started", log.Field{Key: "pid", Value: cmd.Process.Pid})
	return cmd.Process.Pid, nil
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
			return fmt.Errorf("kill container: %w", err)
		}
		<-done // 等待进程完全退出
	case err := <-done:
		if err != nil {
			c.logger.Error("error waiting for container", log.Field{Key: "error", Value: err.Error()})
		}
	}

	c.status.State = "stopped"
	c.status.PID = 0
	c.status.PGID = 0

	c.logger.Info("container stopped successfully")
	return nil
}

// Exec 在容器内执行命令
func (c *BwrapContainer) Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error) {
	if c.cmd == nil || c.cmd.Process == nil {
		return nil, errors.NewKeeperError(errors.ErrCodeContainer, "container not running", nil)
	}

	c.logger.Info("executing command in container",
		log.Field{Key: "command", Value: req.Command},
		log.Field{Key: "pid", Value: c.cmd.Process.Pid},
		log.Field{Key: "timeout", Value: req.Timeout})

	// 检查 nsenter 是否可用
	if _, err := exec.LookPath("nsenter"); err != nil {
		return nil, errors.NewKeeperError(errors.ErrCodeProcess, "nsenter not found, cannot exec into container", err)
	}

	// 使用 nsenter 进入容器命名空间执行命令
	args := []string{
		"-t", strconv.Itoa(c.cmd.Process.Pid),
		"-m", "-u", "-i", "-n", "-p",
		"--", "sh", "-c", req.Command,
	}

	cmd := exec.CommandContext(ctx, "nsenter", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 设置超时
	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
		cmd = exec.CommandContext(ctx, "nsenter", args...)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	// 设置环境变量：继承基础环境，但清除可能用于逃逸的变量
	env := make([]string, 0, len(os.Environ())+len(req.Env))
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
	env = append(env, req.Env...)
	cmd.Env = env

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}

	c.logger.Info("command executed",
		log.Field{Key: "exit_code", Value: exitCode},
		log.Field{Key: "stdout_len", Value: stdout.Len()},
		log.Field{Key: "stderr_len", Value: stderr.Len()})

	// 构建响应
	resp := &ExecResponse{
		ExitCode: exitCode,
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
	}

	// 如果有错误，附加到响应中
	if err != nil {
		resp.Error = err.Error()
	}

	return resp, nil
}

// Status 查询容器状态
func (c *BwrapContainer) Status(ctx context.Context) (*ContainerStatus, error) {
	if c.cmd == nil || c.cmd.Process == nil {
		// 未启动的容器返回当前状态（可能是 created 或 destroyed）
		return &c.status, nil
	}

	// 检查进程是否还在运行
	err := c.cmd.Process.Signal(syscall.Signal(0))
	if err != nil {
		c.status.State = "stopped"
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
	_, err := exec.LookPath("bwrap")
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
func (c *BwrapContainer) buildArgs(spec ContainerSpec) ([]string, string) {
	var args []string
	var bpfFile string

	// 基本参数
	args = append(args, "--unshare-all")
	args = append(args, "--die-with-parent")
	args = append(args, "--new-session")

	// OverlayFS 设置
	// lower 层（只读根文件系统）
	args = append(args, "--ro-bind", spec.Rootfs, "/lower")
	// upper 层（可写层）
	args = append(args, "--bind", spec.UpperDir, "/upper")
	// work 目录（必须与 upper 同设备）
	args = append(args, "--bind", spec.WorkDir, "/work")
	// overlay 挂载点
	args = append(args, "--overlay", "/", "/lower:/upper:/work")

	// 共享内存
	if spec.ShmSize > 0 {
		args = append(args, fmt.Sprintf("--shm-size=%dm", spec.ShmSize))
	}

	// 工作区绑定
	if spec.Workspace != "" {
		args = append(args, fmt.Sprintf("--bind=%s:/workspace", spec.Workspace))
	}

	// 环境变量
	for _, env := range spec.Envvars {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			args = append(args, fmt.Sprintf("--setenv=%s=%s", parts[0], parts[1]))
		}
	}

	// 端口映射（通过 socat 或 iptables 实现，bwrap 本身不直接支持）
	// 这里仅记录配置，实际转发由外部网络模块处理
	for range spec.Ports {
		// bwrap 不支持原生端口映射，需要宿主机 socat/iptables 配合
	}

	// Seccomp BPF（简化实现：写入临时文件后通过 --seccomp 加载）
	if len(spec.SeccompBPF) > 0 {
		bpfFile = filepath.Join(os.TempDir(), fmt.Sprintf("seccomp-%d.bpf", os.Getpid()))
		if err := writeSeccompBPF(bpfFile, spec.SeccompBPF); err == nil {
			args = append(args, "--seccomp="+bpfFile)
		} else {
			// 写入失败，清空路径避免清理时误删
			bpfFile = ""
		}
	}

	// 默认 shell
	args = append(args, "--", "/bin/sh", "-c", "sleep infinity")

	return args, bpfFile
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
	// 简化实现：直接写入文件
	// 实际应该生成 BPF 程序
	return os.WriteFile(filename, bpf, 0644)
}
