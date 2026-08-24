package container

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"keeper/internal/log"
)

// NetworkStrategy 网络策略接口
type NetworkStrategy interface {
	// Name 返回策略名称
	Name() string

	// Configure 配置容器网络
	Configure(spec Spec) ([]string, error)
}

// DefaultNetworkStrategy 默认网络策略
type DefaultNetworkStrategy struct {
	logger log.Logger
}

// NewDefaultNetworkStrategy 创建默认网络策略
func NewDefaultNetworkStrategy(logger log.Logger) *DefaultNetworkStrategy {
	if logger == nil {
		logger = log.Global()
	}
	return &DefaultNetworkStrategy{logger: logger}
}

// Name 返回策略名称
func (s *DefaultNetworkStrategy) Name() string {
	return defaultStrategyName
}

// Configure 配置容器网络
func (s *DefaultNetworkStrategy) Configure(spec Spec) ([]string, error) {
	args := []string{}

	// 配置 DNS
	if len(spec.Envvars) > 0 {
		// 设置 DNS 配置
		args = append(args, "--setenv=NAMESERVER=8.8.8.8")
		args = append(args, "--setenv=NAMESERVER=8.8.4.4")
	}

	return args, nil
}

// SanitizedResolvConf 生成不含回环地址的 resolv.conf，并返回可 bind-mount 的临时文件路径。
// 若宿主机 /etc/resolv.conf 为空或仅含 127.0.0.x，则依次尝试默认网关 / 1.1.1.1。
func SanitizedResolvConf() (string, error) {
	nameservers := extractNameservers("/etc/resolv.conf")
	if len(nameservers) == 0 {
		gw := defaultGateway()
		if gw != "" {
			nameservers = []string{gw}
		}
	}
	if len(nameservers) == 0 {
		nameservers = []string{"1.1.1.1"}
	}

	tmpFile, err := os.CreateTemp("", "keeper-resolv-*.conf")
	if err != nil {
		return "", fmt.Errorf("create temp resolv.conf: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmpFile.Name())
		}
	}()

	var sb strings.Builder
	sb.WriteString("nameserver ")
	sb.WriteString(nameservers[0])
	sb.WriteString("\n")
	for _, ns := range nameservers[1:] {
		sb.WriteString("nameserver ")
		sb.WriteString(ns)
		sb.WriteString("\n")
	}

	if _, err := tmpFile.WriteString(sb.String()); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("write resolv.conf: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("close resolv.conf: %w", err)
	}
	return tmpFile.Name(), nil
}

func extractNameservers(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	var out []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "nameserver ") {
			ns := strings.TrimSpace(line[len("nameserver "):])
			if ns != "" && !strings.HasPrefix(ns, "127.") {
				out = append(out, ns)
			}
		}
	}
	return out
}

func defaultGateway() string {
	cmd := exec.Command("ip", "route", "show", "default") // #nosec G204
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[0] == "default" && fields[1] == "via" {
			return strings.TrimSpace(fields[2])
		}
	}
	return ""
}

// NewNetworkStrategy 创建网络策略
func NewNetworkStrategy(name string, logger log.Logger) (NetworkStrategy, error) {
	switch name {
	case "", defaultStrategyName:
		return NewDefaultNetworkStrategy(logger), nil
	default:
		return nil, fmt.Errorf("unknown network strategy: %s", name)
	}
}
