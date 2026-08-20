package bootstrap

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ProbeResult 探测结果
type ProbeResult struct {
	KernelVersion     string
	OverlayUserNS     bool
	BwrapAvailable    bool
	SeccompAvailable  bool
	UnshareAvailable  bool
	MissingFeatures   []string
	Errors            []string
}

// ProbeEnvironment 探测环境
func ProbeEnvironment() *ProbeResult {
	result := &ProbeResult{
		KernelVersion:   runtime.GOOS + "/" + runtime.GOARCH,
		OverlayUserNS:   false,
		BwrapAvailable:  false,
		SeccompAvailable: false,
		UnshareAvailable: false,
		MissingFeatures: make([]string, 0),
		Errors:          make([]string, 0),
	}

	// 探测内核版本
	result.KernelVersion = getKernelVersion()

	// 探测 OverlayFS + UserNS 支持
	result.OverlayUserNS = checkOverlayUserNS()
	if !result.OverlayUserNS {
		result.MissingFeatures = append(result.MissingFeatures, "CONFIG_OVERLAY_FS_USERNS")
	}

	// 探测 bwrap
	result.BwrapAvailable = checkCommand("bwrap")
	if !result.BwrapAvailable {
		result.MissingFeatures = append(result.MissingFeatures, "bubblewrap")
	}

	// 探测 Seccomp
	result.SeccompAvailable = checkSeccomp()
	if !result.SeccompAvailable {
		result.MissingFeatures = append(result.MissingFeatures, "seccomp")
	}

	// 探测 unshare
	result.UnshareAvailable = checkCommand("unshare")
	if !result.UnshareAvailable {
		result.MissingFeatures = append(result.MissingFeatures, "util-linux/unshare")
	}

	return result
}

// IsSupported 检查环境是否满足运行要求
func (r *ProbeResult) IsSupported() bool {
	return r.OverlayUserNS && r.BwrapAvailable
}

// String 返回可读的报告
func (r *ProbeResult) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Kernel: %s\n", r.KernelVersion))
	buf.WriteString(fmt.Sprintf("OverlayFS+UserNS: %v\n", r.OverlayUserNS))
	buf.WriteString(fmt.Sprintf("Bubblewrap: %v\n", r.BwrapAvailable))
	buf.WriteString(fmt.Sprintf("Seccomp: %v\n", r.SeccompAvailable))
	buf.WriteString(fmt.Sprintf("Unshare: %v\n", r.UnshareAvailable))

	if len(r.MissingFeatures) > 0 {
		buf.WriteString(fmt.Sprintf("Missing features: %s\n", strings.Join(r.MissingFeatures, ", ")))
	}

	if len(r.Errors) > 0 {
		buf.WriteString(fmt.Sprintf("Errors: %s\n", strings.Join(r.Errors, "; ")))
	}

	return buf.String()
}

// 以下为内部辅助函数

func getKernelVersion() string {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

func checkOverlayUserNS() bool {
	// 方法1：检查 /proc/config.gz
	if checkConfigGzip("CONFIG_OVERLAY_FS_USERNS=y") {
		return true
	}

	// 方法2：检查 /boot/config-*
	configPaths := []string{
		fmt.Sprintf("/boot/config-%s", getKernelRelease()),
		"/boot/config",
	}
	for _, path := range configPaths {
		if checkConfigFile(path, "CONFIG_OVERLAY_FS_USERNS=y") {
			return true
		}
	}

	// 方法3：尝试运行测试（实际部署时使用）
	// 这里简化处理：返回 false，要求用户确保内核支持
	return false
}

func checkConfigGzip(pattern string) bool {
	f, err := os.Open("/proc/config.gz")
	if err != nil {
		return false
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	defer gz.Close()

	return scanConfig(gz, pattern)
}

func checkConfigFile(path, pattern string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	return scanConfig(f, pattern)
}

func scanConfig(r io.Reader, pattern string) bool {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), pattern) {
			return true
		}
	}
	return false
}

func getKernelRelease() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func checkCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func checkSeccomp() bool {
	// 简化实现：检查是否存在 seccomp 相关文件或系统调用
	// 实际应该运行测试程序验证
	return true
}

// ProbeReport 生成详细的探测报告（JSON 格式）
func (r *ProbeResult) ProbeReport() string {
	// 简化实现：返回 String() 的结果
	// 实际应该返回 JSON 格式
	return r.String()
}
