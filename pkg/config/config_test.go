package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waitForFileChange(t *testing.T, path string, oldModTime time.Time) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil && info.ModTime().After(oldModTime) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	// 如果超时，强制更新时间以触发变更检测
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		t.Logf("Warning: failed to update file mtime: %v", err)
	}
}

func TestConfigLoadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.DefaultShmSizeMB = 128
	cfg.MaxDownloadBytes = 2 * 1024 * 1024 * 1024

	require.NoError(t, cfg.Save())

	// 重新加载
	loaded, err := Load(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 128, loaded.DefaultShmSizeMB)
	assert.Equal(t, int64(2*1024*1024*1024), loaded.MaxDownloadBytes)
}

func TestConfigReloadIfChanged(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.DefaultShmSizeMB = 64
	require.NoError(t, cfg.Save())

	// 初次加载
	loaded, err := Load(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 64, loaded.DefaultShmSizeMB)

	// 修改配置文件
	configFile := filepath.Join(tmpDir, "config.json")
	newData := []byte(`{"log_level":"debug","max_download_bytes":2147483648,"disable_cross_device_check":true,"default_shm_size_mb":128,"download_timeout":"10m","watchdog_timeout":"120s","watchdog_check_interval":"10s","mcp_allowed_uids":[1000],"mcp_allowed_gids":[1000],"seccomp_strategy":"whitelist","overlay_strategy":"overlayfs"}`)
	require.NoError(t, os.WriteFile(configFile, newData, 0600))

	// 等待文件系统时间更新（不同文件系统精度不同）
	waitForFileChange(t, configFile, loaded.modTime)

	// 重载
	require.NoError(t, loaded.ReloadIfChanged())
	assert.Equal(t, 128, loaded.DefaultShmSizeMB)
	assert.Equal(t, int64(2147483648), loaded.MaxDownloadBytes)
	assert.Equal(t, "debug", loaded.LogLevel)
	assert.Equal(t, []uint32{1000}, loaded.MCPAllowedUIDs)
	assert.Equal(t, "whitelist", loaded.SeccompStrategy)
	assert.Equal(t, "overlayfs", loaded.OverlayStrategy)
}

func TestConfigOnReloadCallback(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.DefaultShmSizeMB = 64
	require.NoError(t, cfg.Save())

	called := false
	var newCfg *Config
	cfg.OnReload(func(c *Config) {
		called = true
		newCfg = c
	})

	// 修改配置
	configFile := filepath.Join(tmpDir, "config.json")
	newData := []byte(`{"log_level":"warn","max_download_bytes":1073741824,"disable_cross_device_check":false,"default_shm_size_mb":256,"download_timeout":"3m","watchdog_timeout":"30s","watchdog_check_interval":"3s","mcp_allowed_uids":[],"mcp_allowed_gids":[],"seccomp_strategy":"blacklist","overlay_strategy":"overlayfs"}`)
	require.NoError(t, os.WriteFile(configFile, newData, 0600))

	// 等待文件系统时间更新（不同文件系统精度不同）
	waitForFileChange(t, configFile, cfg.modTime)

	require.NoError(t, cfg.ReloadIfChanged())

	assert.True(t, called)
	assert.NotNil(t, newCfg)
	assert.Equal(t, 256, newCfg.DefaultShmSizeMB)
	assert.Equal(t, "blacklist", newCfg.SeccompStrategy)
	assert.Equal(t, "overlayfs", newCfg.OverlayStrategy)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: func() *Config {
				cfg := DefaultConfig()
				cfg.Home = "/tmp"
				return cfg
			}(),
			wantErr: false,
		},
		{
			name: "empty home",
			cfg: &Config{
				Home:             "",
				DefaultShmSizeMB: 64,
				MaxDownloadBytes: 1024,
			},
			wantErr: true,
		},
		{
			name: "invalid shm size",
			cfg: &Config{
				Home:             "/tmp",
				DefaultShmSizeMB: 0,
				MaxDownloadBytes: 1024,
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			cfg: &Config{
				Home:             "/tmp",
				DefaultShmSizeMB: 64,
				MaxDownloadBytes: 1024,
				LogLevel:         "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigDurationParsing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WatchdogTimeout = "90s"
	cfg.WatchdogCheckInterval = "10s"
	cfg.DownloadTimeout = "15m"

	timeout, err := cfg.WatchdogTimeoutDuration()
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, timeout)

	interval, err := cfg.WatchdogCheckIntervalDuration()
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, interval)

	downloadTimeout, err := cfg.DownloadTimeoutDuration()
	require.NoError(t, err)
	assert.Equal(t, 15*time.Minute, downloadTimeout)
}

func TestConfigReloadNoChange(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.DefaultShmSizeMB = 64
	require.NoError(t, cfg.Save())

	// 初次加载
	loaded, err := Load(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 64, loaded.DefaultShmSizeMB)

	// 未修改配置文件，重载不应触发回调
	called := false
	loaded.OnReload(func(c *Config) {
		called = true
	})

	require.NoError(t, loaded.ReloadIfChanged())
	assert.False(t, called, "callback should not be called when config unchanged")
}

func TestConfigReloadMultipleTimes(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.DefaultShmSizeMB = 64
	require.NoError(t, cfg.Save())

	// 初次加载
	loaded, err := Load(tmpDir)
	require.NoError(t, err)

	callbackCount := 0
	loaded.OnReload(func(c *Config) {
		callbackCount++
	})

	// 第一次修改
	configFile := filepath.Join(tmpDir, "config.json")
	newData1 := []byte(`{"default_shm_size_mb":128}`)
	require.NoError(t, os.WriteFile(configFile, newData1, 0600))
	waitForFileChange(t, configFile, loaded.modTime)
	require.NoError(t, loaded.ReloadIfChanged())
	assert.Equal(t, 1, callbackCount)
	assert.Equal(t, 128, loaded.DefaultShmSizeMB)

	// 第二次修改
	newData2 := []byte(`{"default_shm_size_mb":256}`)
	require.NoError(t, os.WriteFile(configFile, newData2, 0600))
	waitForFileChange(t, configFile, loaded.modTime)
	require.NoError(t, loaded.ReloadIfChanged())
	assert.Equal(t, 2, callbackCount)
	assert.Equal(t, 256, loaded.DefaultShmSizeMB)
}

func TestConfigReloadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.DefaultShmSizeMB = 64
	require.NoError(t, cfg.Save())

	// 初次加载
	loaded, err := Load(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 64, loaded.DefaultShmSizeMB)

	// 写入无效 JSON
	configFile := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.WriteFile(configFile, []byte(`{invalid json}`), 0600))
	waitForFileChange(t, configFile, loaded.modTime)

	// 重载应失败，但不应改变当前配置
	require.Error(t, loaded.ReloadIfChanged())
	assert.Equal(t, 64, loaded.DefaultShmSizeMB, "config should not be corrupted on reload error")
}

func TestConfigReloadPartialUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.DefaultShmSizeMB = 64
	cfg.LogLevel = "info"
	require.NoError(t, cfg.Save())

	// 初次加载
	loaded, err := Load(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 64, loaded.DefaultShmSizeMB)
	assert.Equal(t, "info", loaded.LogLevel)

	// 只修改部分字段
	configFile := filepath.Join(tmpDir, "config.json")
	newData := []byte(`{"log_level":"debug"}`)
	require.NoError(t, os.WriteFile(configFile, newData, 0600))
	waitForFileChange(t, configFile, loaded.modTime)
	require.NoError(t, loaded.ReloadIfChanged())

	// 验证：修改的字段已更新，未修改的字段保持默认
	assert.Equal(t, "debug", loaded.LogLevel)
	assert.Equal(t, 64, loaded.DefaultShmSizeMB, "unmodified field should retain default value")
}

// TestConfigReloadSnapshotCompressionLevel 测试快照压缩级别热加载
func TestConfigReloadSnapshotCompressionLevel(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.SnapshotCompressionLevel = 6
	require.NoError(t, cfg.Save())

	// 初次加载
	loaded, err := Load(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, 6, loaded.SnapshotCompressionLevel)

	// 修改快照压缩级别
	configFile := filepath.Join(tmpDir, "config.json")
	newData := []byte(`{"snapshot_compression_level": 1}`)
	require.NoError(t, os.WriteFile(configFile, newData, 0600))
	waitForFileChange(t, configFile, loaded.modTime)
	require.NoError(t, loaded.ReloadIfChanged())

	// 验证：快照压缩级别已更新
	assert.Equal(t, 1, loaded.SnapshotCompressionLevel)
}

// TestConfigValidateSnapshotCompressionLevel 测试快照压缩级别验证
func TestConfigValidateSnapshotCompressionLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Home = t.TempDir()

	// 有效值
	cfg.SnapshotCompressionLevel = 1
	assert.NoError(t, cfg.Validate())

	cfg.SnapshotCompressionLevel = 9
	assert.NoError(t, cfg.Validate())

	cfg.SnapshotCompressionLevel = 6
	assert.NoError(t, cfg.Validate())

	// 无效值
	cfg.SnapshotCompressionLevel = 0
	assert.Error(t, cfg.Validate())

	cfg.SnapshotCompressionLevel = 10
	assert.Error(t, cfg.Validate())

	cfg.SnapshotCompressionLevel = -1
	assert.Error(t, cfg.Validate())
}

// TestConfigValidateNetworkFields 测试网络配置字段验证
func TestConfigValidateNetworkFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Home = t.TempDir()

	// 有效值
	cfg.NetworkForwardMaxConnections = 0
	assert.NoError(t, cfg.Validate())

	cfg.NetworkForwardMaxConnections = 100
	assert.NoError(t, cfg.Validate())

	cfg.NetworkForwardConnectTimeout = "5s"
	assert.NoError(t, cfg.Validate())

	cfg.NetworkForwardConnectTimeout = "1m"
	assert.NoError(t, cfg.Validate())

	// 无效值
	cfg.NetworkForwardConnectTimeout = "invalid"
	assert.Error(t, cfg.Validate())
}

// TestConfigValidateStorageFields 测试存储配置字段验证
func TestConfigValidateStorageFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Home = t.TempDir()

	// 有效值
	cfg.StorageMaxSnapshots = 0 // 不限制
	assert.NoError(t, cfg.Validate())

	cfg.StorageMaxSnapshots = 10
	assert.NoError(t, cfg.Validate())

	cfg.StoragePruneInterval = "1h"
	assert.NoError(t, cfg.Validate())

	cfg.StoragePruneInterval = "30m"
	assert.NoError(t, cfg.Validate())

	// 无效值
	cfg.StoragePruneInterval = "invalid"
	assert.Error(t, cfg.Validate())

	cfg.StorageMaxSnapshots = -1
	assert.Error(t, cfg.Validate())
}

// TestConfigValidateDownloaderFields 测试下载器配置字段验证
func TestConfigValidateDownloaderFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Home = t.TempDir()

	// 有效值
	cfg.DownloaderThreads = 1
	assert.NoError(t, cfg.Validate())

	cfg.DownloaderThreads = 8
	assert.NoError(t, cfg.Validate())

	cfg.DownloaderChunkSize = 1024
	assert.NoError(t, cfg.Validate())

	cfg.DownloaderChunkSize = 10 * 1024 * 1024
	assert.NoError(t, cfg.Validate())

	cfg.DownloaderRetryDelay = "100ms"
	assert.NoError(t, cfg.Validate())

	cfg.DownloaderRetryDelay = "5s"
	assert.NoError(t, cfg.Validate())

	// 无效值
	cfg.DownloaderThreads = 0
	assert.Error(t, cfg.Validate())

	cfg.DownloaderThreads = -1
	assert.Error(t, cfg.Validate())

	cfg.DownloaderChunkSize = 512
	assert.Error(t, cfg.Validate())

	cfg.DownloaderRetryDelay = "invalid"
	assert.Error(t, cfg.Validate())
}

// TestConfigValidateMetricsFields 测试指标配置字段验证
func TestConfigValidateMetricsFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Home = t.TempDir()

	// 有效值
	cfg.MetricsEnabled = true
	assert.NoError(t, cfg.Validate())

	cfg.MetricsEnabled = false
	assert.NoError(t, cfg.Validate())

	cfg.MetricsListenAddr = ":9090"
	assert.NoError(t, cfg.Validate())

	cfg.MetricsListenAddr = "127.0.0.1:9090"
	assert.NoError(t, cfg.Validate())

	// MetricsListenAddr 为空字符串在 MetricsEnabled=false 时是允许的
	cfg.MetricsEnabled = false
	cfg.MetricsListenAddr = ""
	assert.NoError(t, cfg.Validate())
}

// TestConfigAllFieldsRoundTrip 测试所有字段的序列化/反序列化
func TestConfigAllFieldsRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Home = tmpDir
	cfg.LogLevel = "debug"
	cfg.MaxDownloadBytes = 2 * 1024 * 1024 * 1024
	cfg.DisableCrossDeviceCheck = true
	cfg.DefaultShmSizeMB = 128
	cfg.DownloadTimeout = "10m"
	cfg.WatchdogTimeout = "120s"
	cfg.WatchdogCheckInterval = "10s"
	cfg.MCPAllowedUIDs = []uint32{1000, 1001}
	cfg.MCPAllowedGIDs = []uint32{1000}
	cfg.SeccompStrategy = "whitelist"
	cfg.OverlayStrategy = "overlayfs"
	cfg.SnapshotCompressionLevel = 3
	cfg.NetworkForwardMaxConnections = 50
	cfg.NetworkForwardConnectTimeout = "10s"
	cfg.DownloaderThreads = 8
	cfg.DownloaderChunkSize = 2 * 1024 * 1024
	cfg.DownloaderRetryDelay = "200ms"
	cfg.StorageMaxSnapshots = 10
	cfg.StoragePruneInterval = "2h"
	cfg.BwrapEnableUserNS = false
	cfg.BwrapEnableSeccomp = true
	cfg.MetricsEnabled = false
	cfg.MetricsListenAddr = "127.0.0.1:9090"

	require.NoError(t, cfg.Save())

	loaded, err := Load(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "debug", loaded.LogLevel)
	assert.Equal(t, int64(2*1024*1024*1024), loaded.MaxDownloadBytes)
	assert.Equal(t, true, loaded.DisableCrossDeviceCheck)
	assert.Equal(t, 128, loaded.DefaultShmSizeMB)
	assert.Equal(t, "10m", loaded.DownloadTimeout)
	assert.Equal(t, "120s", loaded.WatchdogTimeout)
	assert.Equal(t, "10s", loaded.WatchdogCheckInterval)
	assert.Equal(t, []uint32{1000, 1001}, loaded.MCPAllowedUIDs)
	assert.Equal(t, []uint32{1000}, loaded.MCPAllowedGIDs)
	assert.Equal(t, "whitelist", loaded.SeccompStrategy)
	assert.Equal(t, "overlayfs", loaded.OverlayStrategy)
	assert.Equal(t, 3, loaded.SnapshotCompressionLevel)
	assert.Equal(t, 50, loaded.NetworkForwardMaxConnections)
	assert.Equal(t, "10s", loaded.NetworkForwardConnectTimeout)
	assert.Equal(t, 8, loaded.DownloaderThreads)
	assert.Equal(t, int64(2*1024*1024), loaded.DownloaderChunkSize)
	assert.Equal(t, "200ms", loaded.DownloaderRetryDelay)
	assert.Equal(t, 10, loaded.StorageMaxSnapshots)
	assert.Equal(t, "2h", loaded.StoragePruneInterval)
	assert.Equal(t, false, loaded.BwrapEnableUserNS)
	assert.Equal(t, true, loaded.BwrapEnableSeccomp)
	assert.Equal(t, false, loaded.MetricsEnabled)
	assert.Equal(t, "127.0.0.1:9090", loaded.MetricsListenAddr)
}
