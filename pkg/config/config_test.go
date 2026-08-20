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
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		info, err := os.Stat(path)
		if err == nil && info.ModTime().After(oldModTime) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 如果超时，强制更新时间以触发变更检测
	now := time.Now()
	os.Chtimes(path, now, now)
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
	newData := []byte(`{"log_level":"debug","max_download_bytes":2147483648,"disable_cross_device_check":true,"default_shm_size_mb":128,"download_timeout":"10m","watchdog_timeout":"120s","watchdog_check_interval":"10s","mcp_allowed_uids":[1000],"mcp_allowed_gids":[1000]}`)
	require.NoError(t, os.WriteFile(configFile, newData, 0644))

	// 等待文件系统时间更新（不同文件系统精度不同）
	waitForFileChange(t, configFile, loaded.modTime)

	// 重载
	require.NoError(t, loaded.ReloadIfChanged())
	assert.Equal(t, 128, loaded.DefaultShmSizeMB)
	assert.Equal(t, int64(2147483648), loaded.MaxDownloadBytes)
	assert.Equal(t, "debug", loaded.LogLevel)
	assert.Equal(t, []uint32{1000}, loaded.MCPAllowedUIDs)
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
	newData := []byte(`{"log_level":"warn","max_download_bytes":1073741824,"disable_cross_device_check":false,"default_shm_size_mb":256,"download_timeout":"3m","watchdog_timeout":"30s","watchdog_check_interval":"3s","mcp_allowed_uids":[],"mcp_allowed_gids":[]}`)
	require.NoError(t, os.WriteFile(configFile, newData, 0644))

	// 等待文件系统时间更新（不同文件系统精度不同）
	waitForFileChange(t, configFile, cfg.modTime)

	require.NoError(t, cfg.ReloadIfChanged())

	assert.True(t, called)
	assert.NotNil(t, newCfg)
	assert.Equal(t, 256, newCfg.DefaultShmSizeMB)
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
