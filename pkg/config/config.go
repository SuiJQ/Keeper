// Package config 管理 Keeper 配置
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config Keeper 全局配置
type Config struct {
	// Home 数据根目录
	Home string `json:"-"`

	// BinDir bwrap 二进制目录
	BinDir string `json:"-"`

	// CacheDir rootfs 缓存目录
	CacheDir string `json:"-"`

	// AgentsDir Agent 数据目录
	AgentsDir string `json:"-"`

	// LogLevel 日志级别
	LogLevel string `json:"log_level"`

	// MaxDownloadBytes 最大下载字节数
	MaxDownloadBytes int64 `json:"max_download_bytes"`

	// DisableCrossDeviceCheck 禁用跨设备检查
	DisableCrossDeviceCheck bool `json:"disable_cross_device_check"`

	// DefaultShmSizeMB 默认共享内存大小（MB）
	DefaultShmSizeMB int `json:"default_shm_size_mb"`

	// DownloadTimeout 下载超时时间
	DownloadTimeout string `json:"download_timeout"`

	// WatchdogTimeout 看门狗超时时间
	WatchdogTimeout string `json:"watchdog_timeout"`

	// WatchdogCheckInterval 看门狗检查间隔
	WatchdogCheckInterval string `json:"watchdog_check_interval"`

	// MCPAllowedUIDs MCP Server 允许的 UID 列表
	MCPAllowedUIDs []uint32 `json:"mcp_allowed_uids"`

	// MCPAllowedGIDs MCP Server 允许的 GID 列表
	MCPAllowedGIDs []uint32 `json:"mcp_allowed_gids"`

	// SeccompStrategy Seccomp BPF 策略（default/whitelist/blacklist/allow_all）
	SeccompStrategy string `json:"seccomp_strategy"`

	// OverlayStrategy OverlayFS 挂载策略（default/overlayfs）
	OverlayStrategy string `json:"overlay_strategy"`

	// file 配置文件路径（不序列化）
	file string

	// modTime 配置文件最后修改时间（用于热加载）
	modTime time.Time

	// mu 配置读写锁
	mu sync.RWMutex

	// onReload 热加载回调
	onReload []func(*Config)
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		LogLevel:                "info",
		MaxDownloadBytes:        1024 * 1024 * 1024, // 1GB
		DisableCrossDeviceCheck: false,
		DefaultShmSizeMB:        64,
		DownloadTimeout:         "5m",
		WatchdogTimeout:         "60s",
		WatchdogCheckInterval:   "5s",
		MCPAllowedUIDs:          []uint32{},
		MCPAllowedGIDs:          []uint32{},
		SeccompStrategy:         "default",
		OverlayStrategy:         "default",
	}
}

// Load 从文件加载配置
func Load(home string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.Home = home
	cfg.BinDir = filepath.Join(home, "bin")
	cfg.CacheDir = filepath.Join(home, "cache")
	cfg.AgentsDir = filepath.Join(home, "agents")

	configFile := filepath.Join(home, "config.json")

	// 如果配置文件存在，加载它
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}

		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}
		cfg.file = configFile

		// 记录文件修改时间
		if info, err := os.Stat(configFile); err == nil {
			cfg.modTime = info.ModTime()
		}
	}

	// 确保目录存在
	if err := cfg.ensureDirs(); err != nil {
		return nil, fmt.Errorf("create directories: %w", err)
	}

	return cfg, nil
}

// Save 保存配置到文件
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.file == "" {
		c.file = filepath.Join(c.Home, "config.json")
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(c.file, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	// 更新修改时间
	if info, err := os.Stat(c.file); err == nil {
		c.modTime = info.ModTime()
	}

	return nil
}

// ReloadIfChanged 检查配置文件是否变更，如有变更则重载
func (c *Config) ReloadIfChanged() error {
	c.mu.RLock()
	file := c.file
	modTime := c.modTime
	c.mu.RUnlock()

	if file == "" {
		return nil
	}

	info, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("stat config file: %w", err)
	}

	// 检查 modTime 是否变化（支持时钟回拨和精度问题）
	if !info.ModTime().After(modTime) {
		return nil // 未变更
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 二次检查，避免并发重载
	if info.ModTime().Equal(c.modTime) || info.ModTime().Before(c.modTime) {
		return nil
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	newCfg := DefaultConfig()
	newCfg.Home = c.Home
	newCfg.BinDir = c.BinDir
	newCfg.CacheDir = c.CacheDir
	newCfg.AgentsDir = c.AgentsDir
	newCfg.file = c.file

	if err := json.Unmarshal(data, newCfg); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}

	// 保留内部字段
	c.LogLevel = newCfg.LogLevel
	c.MaxDownloadBytes = newCfg.MaxDownloadBytes
	c.DisableCrossDeviceCheck = newCfg.DisableCrossDeviceCheck
	c.DefaultShmSizeMB = newCfg.DefaultShmSizeMB
	c.DownloadTimeout = newCfg.DownloadTimeout
	c.WatchdogTimeout = newCfg.WatchdogTimeout
	c.WatchdogCheckInterval = newCfg.WatchdogCheckInterval
	c.MCPAllowedUIDs = newCfg.MCPAllowedUIDs
	c.MCPAllowedGIDs = newCfg.MCPAllowedGIDs
	c.SeccompStrategy = newCfg.SeccompStrategy
	c.OverlayStrategy = newCfg.OverlayStrategy
	c.modTime = info.ModTime()

	// 触发回调
	for _, fn := range c.onReload {
		fn(c)
	}

	return nil
}

// OnReload 注册配置变更回调
func (c *Config) OnReload(fn func(*Config)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onReload = append(c.onReload, fn)
}

// ensureDirs 确保必要目录存在
func (c *Config) ensureDirs() error {
	dirs := []string{
		c.Home,
		c.BinDir,
		c.CacheDir,
		c.AgentsDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}

	return nil
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	if c.Home == "" {
		return fmt.Errorf("home directory is required")
	}

	if c.DefaultShmSizeMB <= 0 {
		return fmt.Errorf("default_shm_size_mb must be positive")
	}

	if c.MaxDownloadBytes <= 0 {
		return fmt.Errorf("max_download_bytes must be positive")
	}

	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
		"fatal": true,
	}

	if !validLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level: %s", c.LogLevel)
	}

	return nil
}

// WatchdogTimeoutDuration 返回看门狗超时时间
func (c *Config) WatchdogTimeoutDuration() (time.Duration, error) {
	return time.ParseDuration(c.WatchdogTimeout)
}

// WatchdogCheckIntervalDuration 返回看门狗检查间隔
func (c *Config) WatchdogCheckIntervalDuration() (time.Duration, error) {
	return time.ParseDuration(c.WatchdogCheckInterval)
}

// DownloadTimeoutDuration 返回下载超时时间
func (c *Config) DownloadTimeoutDuration() (time.Duration, error) {
	return time.ParseDuration(c.DownloadTimeout)
}
