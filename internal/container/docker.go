// Package container Docker 容器运行时实现
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"keeper/internal/errors"
	"keeper/internal/log"
)

// DockerContainer Docker 容器运行时实现
type DockerContainer struct {
	name    string
	id      string
	logger  log.Logger
	spec    Spec
	running bool
}

// Ensure DockerContainer implements Container
var _ Container = (*DockerContainer)(nil)

// DockerFactory Docker 容器运行时工厂
type DockerFactory struct{}

// NewDockerFactory 创建 Docker 工厂实例
func NewDockerFactory() *DockerFactory {
	return &DockerFactory{}
}

// Create 创建 Docker 容器运行时实例
func (f *DockerFactory) Create(name string) (Container, error) {
	logger := log.Global().WithFields(log.Field{Key: "container", Value: name})
	logger.Info("creating docker container")
	return &DockerContainer{
		name:    name,
		logger:  logger,
		running: false,
	}, nil
}

// Type 返回运行时类型
func (f *DockerFactory) Type() string {
	return "docker"
}

// Start 启动 Docker 容器
func (c *DockerContainer) Start(ctx context.Context, spec Spec) (int, error) {
	c.logger.Info("starting docker container")
	startTime := time.Now()

	c.spec = spec

	// 构建 docker run 参数
	args := []string{"run", "-d"}

	// 容器名称
	args = append(args, "--name", c.name)

	// Hostname
	args = append(args, "--hostname", c.name)

	// 环境变量
	for _, env := range spec.Envvars {
		args = append(args, "-e", env)
	}

	// 共享内存大小
	if spec.ShmSize > 0 {
		args = append(args, "--shm-size", fmt.Sprintf("%dm", spec.ShmSize))
	}

	// 工作空间挂载
	if spec.Workspace != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/workspace", spec.Workspace))
	}

	// 端口映射
	for _, port := range spec.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", port.Host, port.Container))
	}

	// 使用 Alpine 作为基础镜像（轻量级）
	args = append(args, "alpine:latest")

	// 默认命令：sleep infinity
	args = append(args, "/bin/sh", "-c", "sleep infinity")

	cmd := exec.CommandContext(ctx, "docker", args...) // #nosec G204
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		RecordContainerStart("docker", "error")
		return 0, fmt.Errorf("start docker container: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		RecordContainerStart("docker", "error")
		return 0, fmt.Errorf("docker run failed: %w", err)
	}

	// 获取容器 ID
	containerID, err := c.getContainerID()
	if err != nil {
		RecordContainerStart("docker", "error")
		return 0, fmt.Errorf("get container id: %w", err)
	}

	c.id = containerID
	c.running = true

	// 获取容器 PID
	pid, err := c.getContainerPID()
	if err != nil {
		c.logger.Warn("failed to get container pid", log.Field{Key: "error", Value: err.Error()})
		pid = 0
	}

	RecordContainerStart("docker", "success")
	RecordContainerStartDuration("docker", time.Since(startTime).Seconds())
	SetContainerActive("docker", 1)

	c.logger.Info("docker container started", log.Field{Key: "id", Value: c.id}, log.Field{Key: "pid", Value: pid})
	return pid, nil
}

// Stop 停止 Docker 容器
func (c *DockerContainer) Stop(ctx context.Context, grace time.Duration) error {
	c.logger.Info("stopping docker container",
		log.Field{Key: "id", Value: c.id},
		log.Field{Key: "grace", Value: grace})
	startTime := time.Now()

	if !c.running || c.id == "" {
		c.logger.Warn("container not running")
		return nil
	}

	// 优雅停止（将 grace 转换为秒，最小 1 秒）
	stopGrace := int(grace.Seconds())
	if stopGrace < 1 {
		stopGrace = 1
	}
	cmd := exec.CommandContext(ctx, "docker", "stop", "--time", fmt.Sprintf("%d", stopGrace), c.id) // #nosec G204
	if err := cmd.Run(); err != nil {
		c.logger.Warn("docker stop failed", log.Field{Key: "error", Value: err.Error()})
	}

	c.running = false
	c.id = ""

	RecordContainerStop("docker", "success")
	RecordContainerStopDuration("docker", time.Since(startTime).Seconds())
	SetContainerActive("docker", 0)

	c.logger.Info("docker container stopped")
	return nil
}

// Exec 在 Docker 容器内执行命令
func (c *DockerContainer) Exec(ctx context.Context, req ExecRequest) (*ExecResponse, error) {
	if !c.running || c.id == "" {
		RecordContainerExec("docker", "error")
		return nil, errors.NewKeeperError(errors.ErrCodeContainer, "container not running", nil)
	}

	startTime := time.Now()

	args := []string{"exec", c.id}
	args = append(args, "/bin/sh", "-c", req.Command)

	runCmd := exec.CommandContext(ctx, "docker", args...) // #nosec G204
	var stdout, stderr strings.Builder
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr

	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
		runCmd = exec.CommandContext(ctx, "docker", args...) // #nosec G204
		runCmd.Stdout = &stdout
		runCmd.Stderr = &stderr
	}

	err := runCmd.Run()

	exitCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = -1
		}
	}

	resp := &ExecResponse{
		ExitCode: exitCode,
		Stdout:   []byte(stdout.String()),
		Stderr:   []byte(stderr.String()),
	}

	if exitCode != 0 {
		resp.Error = fmt.Sprintf("command exited with code %d", exitCode)
		RecordContainerExec("docker", "error")
	} else {
		RecordContainerExec("docker", "success")
	}
	RecordContainerExecDuration("docker", time.Since(startTime).Seconds())

	return resp, nil
}

// Status 查询 Docker 容器状态
func (c *DockerContainer) Status(ctx context.Context) (*Status, error) {
	if !c.running || c.id == "" {
		return &Status{
			State: "stopped",
			PID:   0,
			PGID:  0,
		}, nil
	}

	// 使用 docker inspect 获取状态
	cmd := exec.CommandContext(ctx, "docker", "inspect", c.id) // #nosec G204
	output, err := cmd.Output()
	if err != nil {
		c.running = false
		return &Status{
			State: "stopped",
			PID:   0,
			PGID:  0,
		}, nil
	}

	var inspectResults []struct {
		State struct {
			Status string `json:"Status"`
			Pid    int    `json:"Pid"`
		} `json:"State"`
	}

	if err := json.Unmarshal(output, &inspectResults); err != nil {
		return nil, fmt.Errorf("parse docker inspect: %w", err)
	}

	if len(inspectResults) == 0 {
		c.running = false
		return &Status{
			State: "stopped",
			PID:   0,
			PGID:  0,
		}, nil
	}

	state := inspectResults[0].State.Status
	if state != "running" {
		c.running = false
		return &Status{
			State: "stopped",
			PID:   0,
			PGID:  0,
		}, nil
	}

	c.logger.Debug("docker container status checked",
		log.Field{Key: "state", Value: state},
		log.Field{Key: "pid", Value: inspectResults[0].State.Pid})

	return &Status{
		State: "running",
		PID:   inspectResults[0].State.Pid,
		PGID:  inspectResults[0].State.Pid,
	}, nil
}

// Close 清理 Docker 容器资源
func (c *DockerContainer) Close() error {
	c.logger.Info("closing docker container")
	if c.running && c.id != "" {
		// 强制删除容器
		cmd := exec.Command("docker", "rm", "-f", c.id) // #nosec G204
		_ = cmd.Run()
	}
	c.running = false
	c.id = ""
	return nil
}

// getContainerID 获取容器 ID
func (c *DockerContainer) getContainerID() (string, error) {
	// 使用精确名称过滤，避免匹配到名称相似的其他容器
	cmd := exec.Command("docker", "ps", "-q", "--filter", fmt.Sprintf("name=^/%s$", c.name)) // #nosec G204
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(string(output))
	if id == "" {
		return "", fmt.Errorf("container %s not found", c.name)
	}
	return id, nil
}

// getContainerPID 获取容器 PID
func (c *DockerContainer) getContainerPID() (int, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Pid}}", c.id) // #nosec G204
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(output))
	if pidStr == "" || pidStr == "<no value>" {
		return 0, fmt.Errorf("pid not available")
	}
	var pid int
	if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
		return 0, err
	}
	return pid, nil
}
