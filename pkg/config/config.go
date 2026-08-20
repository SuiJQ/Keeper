// Package config 管理 Keeper 配置
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	// file 配置文件路径（不序列化）
	file string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		LogLevel:                "info",
		MaxDownloadBytes:        1024 * 1024 * 1024, // 1GB
		DisableCrossDeviceCheck: false,
		DefaultShmSizeMB:        64,
		DownloadTimeout:         "5m",
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
	}

	// 确保目录存在
	if err := cfg.ensureDirs(); err != nil {
		return nil, fmt.Errorf("create directories: %w", err)
	}

	return cfg, nil
}

// Save 保存配置到文件
func (c *Config) Save() error {
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

	return nil
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
