// Package storage 提供 Agent 数据持久化接口
package storage

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	keeperrors "keeper/internal/errors"
)

// Store 定义 Agent 存储操作接口
type Store interface {
	// CreateAgent 创建新 Agent 目录结构
	CreateAgent(ctx context.Context, name string, defaultShmSizeMB int, defaultMaxDownloadBytes int64) (*AgentMeta, error)

	// GetAgent 获取 Agent 元数据
	GetAgent(ctx context.Context, name string) (*AgentMeta, error)

	// UpdateAgent 原子更新 Agent 元数据
	UpdateAgent(ctx context.Context, meta *AgentMeta) error

	// ListAgents 列出所有 Agent
	ListAgents(ctx context.Context) ([]*AgentMeta, error)

	// DeleteAgent 删除 Agent 目录
	DeleteAgent(ctx context.Context, name string) error

	// ForkAgent 复制 Agent 数据（同设备硬链接 / 跨设备物理拷贝）
	ForkAgent(ctx context.Context, source, target string) (*AgentMeta, error)

	// CreateSnapshot 创建快照
	CreateSnapshot(ctx context.Context, name, snapshotID string, compressionLevel int) error

	// RollbackSnapshot 回滚快照
	RollbackSnapshot(ctx context.Context, name, snapshotID string) error

	// ListSnapshots 列出快照
	ListSnapshots(ctx context.Context, name string) ([]SnapshotMeta, error)

	// PruneCache 清理未引用的 rootfs 缓存
	PruneCache(ctx context.Context, dryRun bool) ([]string, error)

	// PruneSnapshots 清理旧快照（保留最近 N 个）
	PruneSnapshots(ctx context.Context, name string, keepCount int) ([]string, error)
}

// AgentMeta Agent 元数据
type AgentMeta struct {
	Name             string        `json:"name"`
	State            string        `json:"state"`
	CreatedAt        string        `json:"created_at"`
	StartedAt        string        `json:"started_at,omitempty"`
	StoppedAt        string        `json:"stopped_at,omitempty"`
	ShmSizeMB        int           `json:"shm_size_mb"`
	Ports            []PortMapping `json:"ports"`
	MaxDownloadBytes int64         `json:"max_download_bytes"`
	PID              int           `json:"pid,omitempty"`
	PGID             string        `json:"pgid,omitempty"`
	MCPPID           int           `json:"mcp_pid,omitempty"`
	WatchdogPID      int           `json:"watchdog_pid,omitempty"`
	DroppedLogs      int           `json:"dropped_logs"`
	CacheURL         string        `json:"cache_url,omitempty"`
	CacheKey         string        `json:"cache_key,omitempty"`
	Error            string        `json:"error,omitempty"`
	RootfsDir        string        `json:"-"`
	UpperDir         string        `json:"-"`
	WorkDir          string        `json:"-"`
	Workspace        string        `json:"-"`
	BackupsDir       string        `json:"-"`
	LogsDir          string        `json:"-"`
	DownloadsDir     string        `json:"-"`
}

// PortMapping 端口映射
type PortMapping struct {
	Host      int `json:"host"`
	Container int `json:"container"`
}

// SnapshotMeta 快照元数据
type SnapshotMeta struct {
	SnapshotID  string    `json:"snapshot_id"`
	ParentID    string    `json:"parent_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Compressed  bool      `json:"compressed"`
	UpperSize   int64     `json:"upper_size_bytes"`
	WorkSize    int64     `json:"workspace_size_bytes"`
	Files       int       `json:"files_count"`
	Incremental bool      `json:"incremental,omitempty"`
}

// fileStore 基于文件系统的存储实现
type fileStore struct {
	home           string
	agentsDir      string
	cacheDir       string
	globalLockPath string
}

// NewStore 创建存储实例
func NewStore(home string) (Store, error) {
	agentsDir := filepath.Join(home, "agents")
	cacheDir := filepath.Join(home, "cache", "rootfs")

	if err := os.MkdirAll(agentsDir, 0700); err != nil {
		return nil, fmt.Errorf("create agents dir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	return &fileStore{
		home:           home,
		agentsDir:      agentsDir,
		cacheDir:       cacheDir,
		globalLockPath: filepath.Join(home, ".global_lock"),
	}, nil
}

// agentDir 返回 Agent 目录路径
func (s *fileStore) agentDir(name string) string {
	return filepath.Join(s.agentsDir, name)
}

// CreateAgent 创建 Agent 目录结构
func (s *fileStore) CreateAgent(ctx context.Context, name string, defaultShmSizeMB int, defaultMaxDownloadBytes int64) (*AgentMeta, error) {
	agentPath := s.agentDir(name)

	// 检查是否已存在
	if _, err := os.Stat(agentPath); err == nil {
		return nil, fmt.Errorf("agent '%s' already exists", name)
	}

	// 创建目录结构
	dirs := []string{
		agentPath,
		filepath.Join(agentPath, "rootfs"),
		filepath.Join(agentPath, "upper"),
		filepath.Join(agentPath, "work"),
		filepath.Join(agentPath, "workspace"),
		filepath.Join(agentPath, "backups"),
		filepath.Join(agentPath, "logs"),
		filepath.Join(agentPath, "downloads"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", dir, err)
		}
	}

	shmSize := defaultShmSizeMB
	if shmSize <= 0 {
		shmSize = 64
	}
	maxDownload := defaultMaxDownloadBytes
	if maxDownload <= 0 {
		maxDownload = 1024 * 1024 * 1024
	}

	meta := &AgentMeta{
		Name:             name,
		State:            "created",
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		ShmSizeMB:        shmSize,
		MaxDownloadBytes: maxDownload,
		RootfsDir:        filepath.Join(agentPath, "rootfs"),
		UpperDir:         filepath.Join(agentPath, "upper"),
		WorkDir:          filepath.Join(agentPath, "work"),
		Workspace:        filepath.Join(agentPath, "workspace"),
		BackupsDir:       filepath.Join(agentPath, "backups"),
		LogsDir:          filepath.Join(agentPath, "logs"),
		DownloadsDir:     filepath.Join(agentPath, "downloads"),
	}

	// 原子写入 meta.json
	if err := s.writeMeta(agentPath, meta); err != nil {
		_ = os.RemoveAll(agentPath)
		return nil, fmt.Errorf("write meta: %w", err)
	}

	// 初始化空 ports.json
	portsPath := filepath.Join(agentPath, "ports.json")
	if err := os.WriteFile(portsPath, []byte("[]\n"), 0600); err != nil {
		_ = os.RemoveAll(agentPath)
		return nil, fmt.Errorf("write ports: %w", err)
	}

	return meta, nil
}

// GetAgent 获取 Agent 元数据
func (s *fileStore) GetAgent(ctx context.Context, name string) (*AgentMeta, error) {
	agentPath := s.agentDir(name)
	meta, err := s.readMeta(agentPath)
	if err != nil {
		return nil, fmt.Errorf("agent '%s' not found: %w", name, err)
	}
	return meta, nil
}

// UpdateAgent 原子更新 Agent 元数据
func (s *fileStore) UpdateAgent(ctx context.Context, meta *AgentMeta) error {
	agentPath := s.agentDir(meta.Name)
	if err := s.writeMeta(agentPath, meta); err != nil {
		return fmt.Errorf("update meta: %w", err)
	}
	return nil
}

// ListAgents 列出所有 Agent
func (s *fileStore) ListAgents(ctx context.Context) ([]*AgentMeta, error) {
	entries, err := os.ReadDir(s.agentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	var agents []*AgentMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.readMeta(filepath.Join(s.agentsDir, entry.Name()))
		if err != nil {
			continue // 跳过无效目录
		}
		agents = append(agents, meta)
	}
	return agents, nil
}

// DeleteAgent 删除 Agent 目录
func (s *fileStore) DeleteAgent(ctx context.Context, name string) error {
	agentPath := s.agentDir(name)
	if err := os.RemoveAll(agentPath); err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}

// ForkAgent 复制 Agent（复用 rootfs 创建逻辑）
func (s *fileStore) ForkAgent(ctx context.Context, source, target string) (*AgentMeta, error) {
	startTime := time.Now()
	sourcePath := s.agentDir(source)
	targetPath := s.agentDir(target)

	sourceMeta, err := s.readMeta(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("source agent '%s' not found: %w", source, err)
	}
	if sourceMeta.State != "stopped" && sourceMeta.State != "created" {
		return nil, fmt.Errorf("cannot fork agent in state '%s', stop it first", sourceMeta.State)
	}
	if _, err := os.Stat(targetPath); err == nil {
		return nil, fmt.Errorf("target agent '%s' already exists", target)
	}

	if err := os.MkdirAll(targetPath, 0700); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}
	if err := s.forkCopyDirs(sourcePath, targetPath); err != nil {
		_ = os.RemoveAll(targetPath)
		return nil, fmt.Errorf("copy source dirs: %w", err)
	}
	s.cleanForkRuntimeFiles(targetPath)

	portsPath := filepath.Join(targetPath, "ports.json")
	if err := os.WriteFile(portsPath, []byte("[]\n"), 0600); err != nil {
		_ = os.RemoveAll(targetPath)
		return nil, fmt.Errorf("reset ports.json: %w", err)
	}

	newMeta := newForkMeta(sourceMeta, targetPath, target)
	if err := s.writeMeta(targetPath, newMeta); err != nil {
		_ = os.RemoveAll(targetPath)
		return nil, fmt.Errorf("write target meta: %w", err)
	}

	RecordFork("success")
	RecordForkDuration(time.Since(startTime).Seconds())
	return newMeta, nil
}

func (s *fileStore) forkCopyDirs(sourcePath, targetPath string) error {
	copyDirs := []struct {
		src  string
		dst  string
		skip bool
	}{
		{filepath.Join(sourcePath, "rootfs"), filepath.Join(targetPath, "rootfs"), false},
		{filepath.Join(sourcePath, "upper"), filepath.Join(targetPath, "upper"), false},
		{filepath.Join(sourcePath, "workspace"), filepath.Join(targetPath, "workspace"), false},
		{filepath.Join(sourcePath, "backups"), filepath.Join(targetPath, "backups"), true},
		{filepath.Join(sourcePath, "logs"), filepath.Join(targetPath, "logs"), true},
		{filepath.Join(sourcePath, "downloads"), filepath.Join(targetPath, "downloads"), true},
		{filepath.Join(sourcePath, "work"), filepath.Join(targetPath, "work"), true},
	}

	for _, cd := range copyDirs {
		if err := s.copyDir(cd.src, cd.dst, cd.skip); err != nil {
			return fmt.Errorf("copy %s: %w", cd.src, err)
		}
	}
	return nil
}

func (s *fileStore) cleanForkRuntimeFiles(targetPath string) {
	runtimeFiles := []string{"pgid", ".watchdog", ".api_sock", ".forward_sock"}
	for _, f := range runtimeFiles {
		_ = os.Remove(filepath.Join(targetPath, f))
	}
}

func newForkMeta(sourceMeta *AgentMeta, targetPath, target string) *AgentMeta {
	return &AgentMeta{
		Name:             target,
		State:            "created",
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		ShmSizeMB:        sourceMeta.ShmSizeMB,
		MaxDownloadBytes: sourceMeta.MaxDownloadBytes,
		CacheURL:         sourceMeta.CacheURL,
		CacheKey:         sourceMeta.CacheKey,
		RootfsDir:        filepath.Join(targetPath, "rootfs"),
		UpperDir:         filepath.Join(targetPath, "upper"),
		WorkDir:          filepath.Join(targetPath, "work"),
		Workspace:        filepath.Join(targetPath, "workspace"),
		BackupsDir:       filepath.Join(targetPath, "backups"),
		LogsDir:          filepath.Join(targetPath, "logs"),
		DownloadsDir:     filepath.Join(targetPath, "downloads"),
	}
}

// CreateSnapshot 创建快照（支持压缩与增量）
func (s *fileStore) CreateSnapshot(ctx context.Context, name, snapshotID string, compressionLevel int) error {
	startTime := time.Now()
	agentPath := s.agentDir(name)
	backupsDir := filepath.Join(agentPath, "backups", snapshotID)

	if err := os.MkdirAll(backupsDir, 0700); err != nil {
		RecordSnapshotCreate("error")
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	// 记录父快照（用于增量）
	parentID := ""
	snapshots, err := s.ListSnapshots(ctx, name)
	if err == nil && len(snapshots) > 0 {
		parentID = snapshots[0].SnapshotID
	}

	// 压缩复制 upper
	upperPath := filepath.Join(agentPath, "upper")
	upperDst := filepath.Join(backupsDir, "upper.tar.gz")
	upperSize, files, err := compressCopy(upperPath, upperDst, compressionLevel)
	if err != nil {
		_ = os.RemoveAll(backupsDir)
		RecordSnapshotCreate("error")
		return fmt.Errorf("snapshot upper: %w", err)
	}

	// 压缩复制 workspace
	workspacePath := filepath.Join(agentPath, "workspace")
	workspaceDst := filepath.Join(backupsDir, "workspace.tar.gz")
	workspaceSize, _, err := compressCopy(workspacePath, workspaceDst, compressionLevel)
	if err != nil {
		_ = os.RemoveAll(backupsDir)
		RecordSnapshotCreate("error")
		return fmt.Errorf("snapshot workspace: %w", err)
	}

	// 写入快照元数据
	meta := SnapshotMeta{
		SnapshotID:  snapshotID,
		ParentID:    parentID,
		CreatedAt:   time.Now().UTC(),
		Compressed:  true,
		UpperSize:   upperSize,
		WorkSize:    workspaceSize,
		Files:       files,
		Incremental: parentID != "", // 有父快照则为增量快照
	}
	metaJSON, _ := json.Marshal(meta)
	if err := os.WriteFile(safeJoin(backupsDir, "meta.json"), metaJSON, 0600); err != nil {
		_ = os.RemoveAll(backupsDir)
		RecordSnapshotCreate("error")
		return fmt.Errorf("write snapshot meta: %w", err)
	}

	RecordSnapshotCreate("success")
	RecordSnapshotCreateDuration(time.Since(startTime).Seconds())
	return nil
}

// RollbackSnapshot 回滚快照
func (s *fileStore) RollbackSnapshot(ctx context.Context, name, snapshotID string) error {
	startTime := time.Now()
	agentPath := s.agentDir(name)
	snapshotDir := filepath.Join(agentPath, "backups", snapshotID)

	if _, err := os.Stat(snapshotDir); err != nil {
		RecordSnapshotRollback("error")
		return fmt.Errorf("snapshot '%s' not found: %w", snapshotID, err)
	}

	upperPath := filepath.Join(agentPath, "upper")
	workspacePath := filepath.Join(agentPath, "workspace")

	upperTmp := upperPath + ".rollback"
	if err := decompressCopy(safeJoin(snapshotDir, "upper.tar.gz"), upperTmp); err != nil {
		RecordSnapshotRollback("error")
		return fmt.Errorf("decompress upper: %w", err)
	}

	workspaceTmp := workspacePath + ".rollback"
	if err := decompressCopy(safeJoin(snapshotDir, "workspace.tar.gz"), workspaceTmp); err != nil {
		_ = os.RemoveAll(upperTmp)
		RecordSnapshotRollback("error")
		return fmt.Errorf("decompress workspace: %w", err)
	}

	if err := atomicExchange(upperPath, upperTmp); err != nil {
		_ = os.RemoveAll(upperTmp)
		_ = os.RemoveAll(workspaceTmp)
		RecordSnapshotRollback("error")
		return fmt.Errorf("atomic exchange upper: %w", err)
	}

	if err := atomicExchange(workspacePath, workspaceTmp); err != nil {
		_ = atomicExchange(upperTmp, upperPath)
		_ = os.RemoveAll(workspaceTmp)
		RecordSnapshotRollback("error")
		return fmt.Errorf("atomic exchange workspace: %w", err)
	}

	if err := recreateWorkDir(filepath.Join(agentPath, "work")); err != nil {
		RecordSnapshotRollback("error")
		return fmt.Errorf("recreate work dir: %w", err)
	}

	RecordSnapshotRollback("success")
	RecordSnapshotRollbackDuration(time.Since(startTime).Seconds())
	return nil
}

// PruneCache 清理未引用的缓存
func (s *fileStore) PruneCache(ctx context.Context, dryRun bool) ([]string, error) {
	startTime := time.Now()
	// 获取所有活跃的 cache_key
	activeKeys := make(map[string]bool)
	agents, err := s.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	for _, meta := range agents {
		if meta.CacheKey != "" {
			activeKeys[meta.CacheKey] = true
		}
	}

	// 扫描缓存目录
	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			RecordCachePrune("success")
			return nil, nil
		}
		RecordCachePrune("error")
		return nil, err
	}

	var deleted []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !activeKeys[entry.Name()] {
			if dryRun {
				deleted = append(deleted, entry.Name())
			} else {
				if err := os.RemoveAll(filepath.Join(s.cacheDir, entry.Name())); err != nil {
					RecordCachePrune("error")
					return nil, fmt.Errorf("delete cache %s: %w", entry.Name(), err)
				}
				deleted = append(deleted, entry.Name())
			}
		}
	}

	result := "success"
	if dryRun {
		result = "dry_run"
	}
	RecordCachePrune(result)
	RecordCachePruneDuration(time.Since(startTime).Seconds())
	return deleted, nil
}

// PruneSnapshots 清理旧快照（保留最近 N 个）
func (s *fileStore) PruneSnapshots(ctx context.Context, name string, keepCount int) ([]string, error) {
	startTime := time.Now()

	// keepCount <= 0 表示不限制
	if keepCount <= 0 {
		return nil, nil
	}

	snapshots, err := s.ListSnapshots(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	// 如果快照数量未超过限制，无需清理
	if len(snapshots) <= keepCount {
		return nil, nil
	}

	// 按创建时间排序（最新的在前）
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	// 需要删除的快照（跳过前 keepCount 个）
	toDelete := snapshots[keepCount:]

	agentPath := s.agentDir(name)
	var deleted []string

	for _, meta := range toDelete {
		snapshotDir := filepath.Join(agentPath, "backups", meta.SnapshotID)
		if err := os.RemoveAll(snapshotDir); err != nil {
			RecordSnapshotPrune("error")
			return deleted, fmt.Errorf("delete snapshot %s: %w", meta.SnapshotID, err)
		}
		deleted = append(deleted, meta.SnapshotID)
	}

	result := "success"
	if len(deleted) == 0 {
		result = "noop"
	}
	RecordSnapshotPrune(result)
	RecordSnapshotPruneDuration(time.Since(startTime).Seconds())
	return deleted, nil
}

// 辅助方法

func (s *fileStore) writeMeta(agentPath string, meta *AgentMeta) error {
	metaPath := safeJoin(agentPath, "meta.json")
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(metaPath, data)
}

func (s *fileStore) readMeta(agentPath string) (*AgentMeta, error) {
	metaPath := safeJoin(agentPath, "meta.json")
	data, err := os.ReadFile(metaPath) // #nosec G304
	if err != nil {
		return nil, err
	}
	var meta AgentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	meta.RootfsDir = safeJoin(agentPath, "rootfs")
	meta.UpperDir = safeJoin(agentPath, "upper")
	meta.WorkDir = safeJoin(agentPath, "work")
	meta.Workspace = safeJoin(agentPath, "workspace")
	meta.BackupsDir = safeJoin(agentPath, "backups")
	meta.LogsDir = safeJoin(agentPath, "logs")
	meta.DownloadsDir = safeJoin(agentPath, "downloads")
	return &meta, nil
}

func safeJoin(dir, name string) string {
	return filepath.Join(dir, filepath.Base(name))
}

func (s *fileStore) copyDir(src, dst string, emptyOnly bool) error {
	if emptyOnly {
		return os.MkdirAll(dst, 0700)
	}

	// 检查源是否存在
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}

	// 检查设备是否相同
	sameDevice := true
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	dstParent := filepath.Dir(dst)
	if err := os.MkdirAll(dstParent, 0700); err != nil {
		return err
	}
	dstInfo, err := os.Stat(dstParent)
	if err != nil {
		return err
	}
	if srcInfo.Sys().(*syscall.Stat_t).Dev != dstInfo.Sys().(*syscall.Stat_t).Dev {
		sameDevice = false
	}

	// 同设备硬链接，跨设备物理拷贝
	if sameDevice {
		return copyHardlink(src, dst)
	}
	return copyPhysical(src, dst)
}

// 以下为底层工具函数实现

func atomicWriteFile(filename string, data []byte) error {
	dir := filepath.Dir(filename)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, filename)
}

func copyHardlink(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		// 硬链接
		return os.Link(path, targetPath)
	})
}

func copyPhysical(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		// 拷贝文件
		srcFile, err := os.Open(path) // #nosec G304
		if err != nil {
			return err
		}
		dstFile, err := os.Create(targetPath) // #nosec G304
		if err != nil {
			_ = srcFile.Close()
			return err
		}
		_, copyErr := io.Copy(dstFile, srcFile)
		// 立即关闭文件，避免在 Walk 中累积文件描述符
		srcFileCloseErr := srcFile.Close()
		dstFileCloseErr := dstFile.Close()

		if copyErr != nil {
			if stderrors.Is(copyErr, syscall.ENOSPC) {
				return fmt.Errorf("%w: %s", keeperrors.ErrNoSpace, copyErr)
			}
			return copyErr
		}
		if srcFileCloseErr != nil {
			return srcFileCloseErr
		}
		if dstFileCloseErr != nil {
			return dstFileCloseErr
		}
		// 保留权限
		return os.Chmod(targetPath, info.Mode())
	})
}

func checkSameDevice(paths ...string) error {
	if len(paths) < 2 {
		return nil
	}
	var dev uint64
	for i, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			return err
		}
		d := fi.Sys().(*syscall.Stat_t).Dev
		if i == 0 {
			dev = d
		} else if d != dev {
			return fmt.Errorf("device mismatch: %s != %s", paths[0], paths[i])
		}
	}
	return nil
}

func atomicExchange(source, target string) error {
	// 使用 renameat2 实现真正的原子交换（Linux 3.15+）
	// 如果内核不支持，fallback 到非原子实现
	sourceDir := filepath.Dir(source)
	targetDir := filepath.Dir(target)

	sourceFD, err := os.Open(sourceDir) // #nosec G304
	if err != nil {
		return fmt.Errorf("open source dir: %w", err)
	}
	defer func() { _ = sourceFD.Close() }()

	targetFD, err := os.Open(targetDir) // #nosec G304
	if err != nil {
		return fmt.Errorf("open target dir: %w", err)
	}
	defer func() { _ = targetFD.Close() }()

	// 尝试使用 renameat2 进行原子交换
	// RENAME_EXCHANGE: 原子交换两个文件
	// RENAME_NOREPLACE: 不覆盖已存在的文件（作为 fallback）
	err = unix.Renameat2(int(sourceFD.Fd()), filepath.Base(source),
		int(targetFD.Fd()), filepath.Base(target), unix.RENAME_EXCHANGE)
	if err == nil {
		return nil
	}

	// Fallback：如果 renameat2 不可用或失败，使用非原子实现
	// 注意：这仅在测试环境或旧内核上使用
	tmpFile := target + ".tmp"
	if err := os.Rename(target, tmpFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rename target to tmp: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(tmpFile, target)
		return fmt.Errorf("rename source to target: %w", err)
	}
	if tmpFile != target {
		if err := os.Rename(tmpFile, source); err != nil {
			return fmt.Errorf("rename tmp to source: %w", err)
		}
	}
	return nil
}

func recreateWorkDir(workDir string) error {
	// 重命名隔离法
	purgeDir := workDir + ".purge"

	// 如果 purge 目录已存在，先删除
	if _, err := os.Stat(purgeDir); err == nil {
		if err := os.RemoveAll(purgeDir); err != nil {
			return fmt.Errorf("remove existing purge dir: %w", err)
		}
	}

	if err := os.Rename(workDir, purgeDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(workDir, 0700); err != nil {
		return err
	}
	// 后台异步回收（5 秒超时，超时则遗留文件由下次启动清理）
	go func() {
		done := make(chan struct{})
		go func() {
			_ = os.RemoveAll(purgeDir)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			// 异步清理超时，遗留文件将由下次启动清理
		}
	}()
	return nil
}

// compressCopy 压缩复制目录到 tar.gz
func compressCopy(src, dst string, compressionLevel int) (int64, int, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return 0, 0, err
	}

	f, err := os.Create(dst) // #nosec G304
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()

	gw, err := gzip.NewWriterLevel(f, compressionLevel)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	var totalSize int64
	var fileCount int
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		return compressCopyWalk(src, path, info, err, tw, gw, &totalSize, &fileCount)
	}); err != nil {
		_ = os.Remove(dst)
		if stderrors.Is(err, syscall.ENOSPC) {
			return 0, 0, fmt.Errorf("%w: %s", keeperrors.ErrNoSpace, err)
		}
		return 0, 0, err
	}

	return totalSize, fileCount, nil
}

func compressCopyWalk(src, path string, info os.FileInfo, err error, tw *tar.Writer, gw *gzip.Writer, totalSize *int64, fileCount *int) error {
	if err != nil {
		return err
	}
	if info == nil {
		return nil
	}
	rel, err2 := filepath.Rel(src, path)
	if err2 != nil {
		return err2
	}
	if rel == "." {
		return nil
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = rel
	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if info.IsDir() {
		return nil
	}

	srcFile, err := os.Open(path) // #nosec G304
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	n, err := io.Copy(gw, srcFile)
	if err != nil {
		if stderrors.Is(err, syscall.ENOSPC) {
			return fmt.Errorf("%w: %s", keeperrors.ErrNoSpace, err)
		}
		return err
	}
	*totalSize += n
	*fileCount++
	return nil
}

// decompressCopy 解压 tar.gz 到目录
func decompressCopy(src, dst string) error {
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}

	f, err := os.Open(src) // #nosec G304
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gr.Close() }()

	const maxDecompressedSize = 10 << 30 // 10GB limit to prevent decompression bombs
	var totalSize int64

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target, err := safeTarTarget(dst, header.Name)
		if err != nil {
			return err
		}

		totalSize += header.Size
		if totalSize > maxDecompressedSize {
			return fmt.Errorf("decompressed size exceeds limit: %d > %d", totalSize, maxDecompressedSize)
		}

		if err := writeTarEntry(target, header, tr); err != nil {
			return err
		}
	}
	return nil
}

func safeTarTarget(dst, entryName string) (string, error) {
	target := filepath.Join(dst, entryName)
	cleanDst := filepath.Clean(dst)
	if !strings.HasPrefix(target, cleanDst+string(os.PathSeparator)) && target != cleanDst {
		return "", fmt.Errorf("invalid tar entry path: %s", entryName)
	}
	return target, nil
}

func writeTarEntry(target string, header *tar.Header, r io.Reader) error {
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, header.FileInfo().Mode())
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode()) // #nosec G304
		if err != nil {
			return err
		}
		defer func() { _ = outFile.Close() }()

		if _, err := io.Copy(outFile, r); err != nil {
			return err
		}
		return nil
	}
	return nil
}

// ListSnapshots 列出快照
func (s *fileStore) ListSnapshots(ctx context.Context, name string) ([]SnapshotMeta, error) {
	agentPath := s.agentDir(name)
	backupsDir := filepath.Join(agentPath, "backups")

	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []SnapshotMeta{}, nil
		}
		return nil, err
	}

	var snapshots []SnapshotMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := safeJoin(safeJoin(backupsDir, entry.Name()), "meta.json")
		data, err := os.ReadFile(metaPath) // #nosec G304
		if err != nil {
			continue
		}
		var meta SnapshotMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		snapshots = append(snapshots, meta)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt)
	})

	return snapshots, nil
}
