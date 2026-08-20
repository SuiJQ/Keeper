package bootstrap

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	result := checkConfigFile("/boot/config-nonexistent", "CONFIG_OVERLAY_FS_USERNS=y")
	assert.False(t, result)
}
