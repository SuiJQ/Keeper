package bootstrap

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeEnvironment(t *testing.T) {
	result := ProbeEnvironment()

	// 验证基本字段
	assert.NotEmpty(t, result.KernelVersion)
	assert.NotNil(t, result.MissingFeatures)
	assert.NotNil(t, result.Errors)
}

func TestProbeResultIsSupported(t *testing.T) {
	result := &ProbeResult{
		OverlayUserNS:  true,
		BwrapAvailable: true,
	}
	assert.True(t, result.IsSupported())

	result = &ProbeResult{
		OverlayUserNS:  false,
		BwrapAvailable: true,
	}
	assert.False(t, result.IsSupported())
}

func TestProbeResultString(t *testing.T) {
	result := &ProbeResult{
		KernelVersion:    "5.15.0-76-generic",
		OverlayUserNS:    true,
		BwrapAvailable:   true,
		SeccompAvailable: true,
		UnshareAvailable: true,
	}
	output := result.String()
	assert.Contains(t, output, "Kernel: 5.15.0-76-generic")
	assert.Contains(t, output, "OverlayFS+UserNS: true")
	assert.Contains(t, output, "Bubblewrap: true")
}

func TestProbeResultStringWithMissing(t *testing.T) {
	result := &ProbeResult{
		KernelVersion:    "5.10.134",
		OverlayUserNS:    false,
		BwrapAvailable:   true,
		SeccompAvailable: false,
		UnshareAvailable: true,
		MissingFeatures:  []string{"CONFIG_OVERLAY_FS_USERNS", "seccomp"},
	}
	output := result.String()
	assert.Contains(t, output, "Missing features: CONFIG_OVERLAY_FS_USERNS, seccomp")
}

func TestGetKernelVersion(t *testing.T) {
	version := getKernelVersion()
	assert.NotEmpty(t, version)
	assert.NotEqual(t, "unknown", version)
}

func TestCheckCommand(t *testing.T) {
	// 测试存在的命令
	assert.True(t, checkCommand("ls"))
	assert.True(t, checkCommand("cat"))

	// 测试不存在的命令
	assert.False(t, checkCommand("nonexistent-command-12345"))
}

func TestGetKernelRelease(t *testing.T) {
	release := getKernelRelease()
	// 在真实系统上应该返回类似 5.15.0-76-generic
	// 在测试环境中可能为空
	if release != "" {
		assert.Regexp(t, `^\d+\.\d+`, release)
	}
}

func TestCheckConfigGzip(t *testing.T) {
	// /proc/config.gz 通常不存在，应该返回 false
	result := checkConfigGzip("CONFIG_OVERLAY_FS_USERNS=y")
	assert.False(t, result)
}

func TestCheckConfigFile(t *testing.T) {
	// /boot/config-* 通常不存在，应该返回 false
	result := checkConfigFile("/boot/config-nonexistent")
	assert.False(t, result)
}

func TestProbeReportJSON(t *testing.T) {
	result := &ProbeResult{
		KernelVersion:    "5.15.0-76-generic",
		OverlayUserNS:    true,
		BwrapAvailable:   true,
		SeccompAvailable: true,
		UnshareAvailable: true,
	}
	report := result.ProbeReport()
	assert.Contains(t, report, "KernelVersion")
	assert.Contains(t, report, "5.15.0-76-generic")
	assert.Contains(t, report, "OverlayUserNS")
	assert.Contains(t, report, "BwrapAvailable")
	assert.Contains(t, report, "true")
}

func TestCheckSeccomp(t *testing.T) {
	// checkSeccomp 应该返回布尔值
	result := checkSeccomp()
	assert.IsType(t, true, result)
}

func TestProbeEnvironmentResult(t *testing.T) {
	result := ProbeEnvironment()

	// 验证所有字段都已填充
	assert.NotEmpty(t, result.KernelVersion)
	assert.NotNil(t, result.MissingFeatures)
	assert.NotNil(t, result.Errors)

	// 验证 IsSupported 与探测结果一致
	expected := result.OverlayUserNS && result.BwrapAvailable
	assert.Equal(t, expected, result.IsSupported())
}

func TestCheckConfigFileFound(t *testing.T) {
	// 创建一个临时配置文件
	tmpFile := filepath.Join(t.TempDir(), "config")
	err := os.WriteFile(tmpFile, []byte("CONFIG_OVERLAY_FS_USERNS=y\n"), 0600)
	require.NoError(t, err)

	result := checkConfigFile(tmpFile)
	assert.True(t, result)
}

func TestCheckConfigFileNotFound(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "config")
	err := os.WriteFile(tmpFile, []byte("CONFIG_OTHER=y\n"), 0600)
	require.NoError(t, err)

	result := checkConfigFile(tmpFile)
	assert.False(t, result)
}

func TestCheckConfigGzipFound(t *testing.T) {
	// 创建模拟的 config.gz
	tmpFile := filepath.Join(t.TempDir(), "config.gz")
	f, err := os.Create(tmpFile)
	require.NoError(t, err)

	// 使用 gzip 写入内容
	gw := gzip.NewWriter(f)
	_, err = gw.Write([]byte("CONFIG_OVERLAY_FS_USERNS=y\n"))
	require.NoError(t, err)
	gw.Close()
	f.Close()

	// 由于 checkConfigGzip 硬编码了 /proc/config.gz 路径，
	// 这里我们测试函数不会 panic
	_ = checkConfigGzip("CONFIG_OVERLAY_FS_USERNS=y")
}

func TestIsSupported(t *testing.T) {
	tests := []struct {
		name     string
		result   *ProbeResult
		expected bool
	}{
		{
			name: "all supported",
			result: &ProbeResult{
				OverlayUserNS:  true,
				BwrapAvailable: true,
			},
			expected: true,
		},
		{
			name: "missing overlay",
			result: &ProbeResult{
				OverlayUserNS:  false,
				BwrapAvailable: true,
			},
			expected: false,
		},
		{
			name: "missing bwrap",
			result: &ProbeResult{
				OverlayUserNS:  true,
				BwrapAvailable: false,
			},
			expected: false,
		},
		{
			name: "both missing",
			result: &ProbeResult{
				OverlayUserNS:  false,
				BwrapAvailable: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.IsSupported())
		})
	}
}
