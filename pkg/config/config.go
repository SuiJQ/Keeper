// Package config manages Keeper configuration.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// SnapshotCompressionLevel 快照压缩级别（1-9，1 最快，9 最佳压缩）
	SnapshotCompressionLevel int `json:"snapshot_compression_level"`

	// NetworkForwardMaxConnections 每个端口转发的最大并发连接数（0=不限制）
	NetworkForwardMaxConnections int `json:"network_forward_max_connections"`

	// NetworkForwardConnectTimeout 端口转发连接超时时间
	NetworkForwardConnectTimeout string `json:"network_forward_connect_timeout"`

	// DownloaderThreads 下载器默认线程数
	DownloaderThreads int `json:"downloader_threads"`

	// DownloaderChunkSize 下载器分块大小（字节）
	DownloaderChunkSize int64 `json:"downloader_chunk_size"`

	// DownloaderRetryDelay 下载器重试间隔
	DownloaderRetryDelay string `json:"downloader_retry_delay"`

	// StorageMaxSnapshots 每个 Agent 保留的最大快照数（0=不限制）
	StorageMaxSnapshots int `json:"storage_max_snapshots"`

	// StoragePruneInterval 存储清理间隔
	StoragePruneInterval string `json:"storage_prune_interval"`

	// BwrapEnableUserNS 是否启用 UserNS
	BwrapEnableUserNS bool `json:"bwrap_enable_userns"`

	// BwrapEnableSeccomp 是否启用 Seccomp
	BwrapEnableSeccomp bool `json:"bwrap_enable_seccomp"`

	// MetricsEnabled 是否启用 metrics server
	MetricsEnabled bool `json:"metrics_enabled"`

	// MetricsListenAddr metrics server 监听地址
	MetricsListenAddr string `json:"metrics_listen_addr"`

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
		LogLevel:                     "info",
		MaxDownloadBytes:             1024 * 1024 * 1024, // 1GB
		DisableCrossDeviceCheck:      false,
		DefaultShmSizeMB:             64,
		DownloadTimeout:              "5m",
		WatchdogTimeout:              "60s",
		WatchdogCheckInterval:        "5s",
		MCPAllowedUIDs:               []uint32{},
		MCPAllowedGIDs:               []uint32{},
		SeccompStrategy:              "default",
		OverlayStrategy:              "default",
		SnapshotCompressionLevel:     6, // 平衡速度和压缩率
		NetworkForwardMaxConnections: 0, // 不限制
		NetworkForwardConnectTimeout: "5s",
		DownloaderThreads:            4,
		DownloaderChunkSize:          1024 * 1024, // 1MB
		DownloaderRetryDelay:         "100ms",
		StorageMaxSnapshots:          0, // 不限制
		StoragePruneInterval:         "1h",
		BwrapEnableUserNS:            true,
		BwrapEnableSeccomp:           true,
		MetricsEnabled:               true,
		MetricsListenAddr:            ":9090",
	}
}

// resolveConfigFile 解析配置文件路径并校验其在 home 目录内
func resolveConfigFile(home, configFile string) (string, error) {
	absHome, err := filepath.Abs(home)
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	absConfig, err := filepath.Abs(configFile)
	if err != nil {
		return "", fmt.Errorf("resolve config file: %w", err)
	}
	if !strings.HasPrefix(absConfig, absHome+string(filepath.Separator)) && absConfig != absHome {
		return "", fmt.Errorf("config file %q is outside home %q", absConfig, absHome)
	}
	return absConfig, nil
}

// Load loads Keeper configuration from the specified home directory.
func Load(home string) (*Config, error) {
	cfg := DefaultConfig()
	cfg.Home = home
	cfg.BinDir = filepath.Join(home, "bin")
	cfg.CacheDir = filepath.Join(home, "cache")
	cfg.AgentsDir = filepath.Join(home, "agents")

	configFile := filepath.Join(home, "config.json")
	configFile, err := resolveConfigFile(home, configFile)
	if err != nil {
		return nil, err
	}

	// 如果配置文件存在，加载它
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile) // #nosec G304
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

// LoadDefaultIfExists attempts to load configuration from the default Keeper directory.
func LoadDefaultIfExists() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get user home: %w", err)
	}

	defaultHome := filepath.Join(home, ".local", "share", "keeper")
	configFile := filepath.Join(defaultHome, "config.json")

	// 检查默认配置是否存在
	if _, err := os.Stat(configFile); err != nil {
		return nil, fmt.Errorf("default config not found: %w", err)
	}

	return Load(defaultHome)
}

// Save persists the configuration to disk.
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

	if err := os.WriteFile(c.file, data, 0600); err != nil {
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

	return c.applyConfig(file, info)
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

// applyConfig 将重载后的配置应用到当前实例，并触发回调
func (c *Config) applyConfig(file string, info os.FileInfo) error {
	data, err := os.ReadFile(file) // #nosec G304
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
	c.SnapshotCompressionLevel = newCfg.SnapshotCompressionLevel
	c.NetworkForwardMaxConnections = newCfg.NetworkForwardMaxConnections
	c.NetworkForwardConnectTimeout = newCfg.NetworkForwardConnectTimeout
	c.DownloaderThreads = newCfg.DownloaderThreads
	c.DownloaderChunkSize = newCfg.DownloaderChunkSize
	c.DownloaderRetryDelay = newCfg.DownloaderRetryDelay
	c.StorageMaxSnapshots = newCfg.StorageMaxSnapshots
	c.StoragePruneInterval = newCfg.StoragePruneInterval
	c.BwrapEnableUserNS = newCfg.BwrapEnableUserNS
	c.BwrapEnableSeccomp = newCfg.BwrapEnableSeccomp
	c.MetricsEnabled = newCfg.MetricsEnabled
	c.MetricsListenAddr = newCfg.MetricsListenAddr
	c.modTime = info.ModTime()

	for _, fn := range c.onReload {
		fn(c)
	}

	return nil
}

// Validate checks whether the current configuration is valid.
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

	if c.SnapshotCompressionLevel < 1 || c.SnapshotCompressionLevel > 9 {
		return fmt.Errorf("snapshot_compression_level must be between 1 and 9")
	}

	if c.DownloaderThreads < 1 {
		return fmt.Errorf("downloader_threads must be at least 1")
	}

	if c.DownloaderChunkSize < 1024 {
		return fmt.Errorf("downloader_chunk_size must be at least 1024 bytes")
	}

	validSeccomp := map[string]bool{
		"default":   true,
		"whitelist": true,
		"blacklist": true,
		"allow_all": true,
	}
	if !validSeccomp[c.SeccompStrategy] {
		return fmt.Errorf("invalid seccomp_strategy: %s", c.SeccompStrategy)
	}

	validOverlay := map[string]bool{
		"default":   true,
		"overlayfs": true,
	}
	if !validOverlay[c.OverlayStrategy] {
		return fmt.Errorf("invalid overlay_strategy: %s", c.OverlayStrategy)
	}

	if c.NetworkForwardMaxConnections < 0 {
		return fmt.Errorf("network_forward_max_connections must be non-negative")
	}

	if _, err := time.ParseDuration(c.NetworkForwardConnectTimeout); err != nil {
		return fmt.Errorf("invalid network_forward_connect_timeout: %s", c.NetworkForwardConnectTimeout)
	}

	if c.StorageMaxSnapshots < 0 {
		return fmt.Errorf("storage_max_snapshots must be non-negative")
	}

	if _, err := time.ParseDuration(c.StoragePruneInterval); err != nil {
		return fmt.Errorf("invalid storage_prune_interval: %s", c.StoragePruneInterval)
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

// NetworkForwardConnectTimeoutDuration 返回端口转发连接超时时间
func (c *Config) NetworkForwardConnectTimeoutDuration() (time.Duration, error) {
	return time.ParseDuration(c.NetworkForwardConnectTimeout)
}

// DownloaderRetryDelayDuration 返回下载器重试间隔
func (c *Config) DownloaderRetryDelayDuration() (time.Duration, error) {
	return time.ParseDuration(c.DownloaderRetryDelay)
}

// StoragePruneIntervalDuration 返回存储清理间隔
func (c *Config) StoragePruneIntervalDuration() (time.Duration, error) {
	return time.ParseDuration(c.StoragePruneInterval)
}
