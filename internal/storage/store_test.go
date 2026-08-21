package storage

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	meta, err := store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)
	assert.Equal(t, "test-agent", meta.Name)
	assert.Equal(t, "created", meta.State)
	assert.NotEmpty(t, meta.CreatedAt)
	assert.DirExists(t, meta.RootfsDir)
	assert.DirExists(t, meta.UpperDir)
	assert.DirExists(t, meta.WorkDir)
	assert.DirExists(t, meta.Workspace)
}

func TestCreateAgentDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 重复创建应失败
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestGetAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	created, err := store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	got, err := store.GetAgent(ctx, "test-agent")
	require.NoError(t, err)
	assert.Equal(t, created.Name, got.Name)
	assert.Equal(t, created.State, got.State)
}

func TestGetAgentNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.GetAgent(ctx, "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestListAgents(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "agent-a", 64, 1024*1024*1024)
	require.NoError(t, err)
	_, err = store.CreateAgent(ctx, "agent-b", 64, 1024*1024*1024)
	require.NoError(t, err)

	agents, err := store.ListAgents(ctx)
	require.NoError(t, err)
	assert.Len(t, agents, 2)
}

func TestDeleteAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	err = store.DeleteAgent(ctx, "test-agent")
	require.NoError(t, err)

	// 确认已删除
	_, err = store.GetAgent(ctx, "test-agent")
	require.Error(t, err)
}

func TestForkAgent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "source", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入一些文件到 upper 目录
	upperFile := filepath.Join(tmpDir, "agents", "source", "upper", "test.txt")
	err = os.WriteFile(upperFile, []byte("hello"), 0644)
	require.NoError(t, err)

	// Fork
	forked, err := store.ForkAgent(ctx, "source", "target")
	require.NoError(t, err)
	assert.Equal(t, "target", forked.Name)
	assert.Equal(t, "created", forked.State)
	assert.NotEmpty(t, forked.CreatedAt)

	// 确认文件已复制
	targetFile := filepath.Join(tmpDir, "agents", "target", "upper", "test.txt")
	content, err := os.ReadFile(targetFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)

	// 确认 work 目录存在但是空的（启动时自动重建，不复制源内容）
	workDir := filepath.Join(tmpDir, "agents", "target", "work")
	entries, err := os.ReadDir(workDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "work directory should be empty")

	// 确认运行时文件已清理
	runtimeFiles := []string{"pgid", ".watchdog", ".api_sock", ".forward_sock"}
	for _, f := range runtimeFiles {
		_, err = os.Stat(filepath.Join(tmpDir, "agents", "target", f))
		assert.True(t, os.IsNotExist(err), "runtime file %s should be removed", f)
	}

	// 确认 ports.json 已重置
	portsPath := filepath.Join(tmpDir, "agents", "target", "ports.json")
	content, err = os.ReadFile(portsPath)
	require.NoError(t, err)
	assert.Equal(t, "[]\n", string(content))
}

func TestForkAgentInvalidState(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	meta, err := store.CreateAgent(ctx, "source", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 模拟 running 状态
	meta.State = "running"
	err = store.UpdateAgent(ctx, meta)
	require.NoError(t, err)

	// Fork 应该失败
	_, err = store.ForkAgent(ctx, "source", "target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot fork agent in state 'running'")
}

func TestForkAgentTargetExists(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "source", 64, 1024*1024*1024)
	require.NoError(t, err)
	_, err = store.CreateAgent(ctx, "target", 64, 1024*1024*1024)
	require.NoError(t, err)

	_, err = store.ForkAgent(ctx, "source", "target")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestCreateSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入一些文件
	upperFile := filepath.Join(tmpDir, "agents", "test-agent", "upper", "data.txt")
	err = os.WriteFile(upperFile, []byte("snapshot data"), 0644)
	require.NoError(t, err)

	// 创建快照
	err = store.CreateSnapshot(ctx, "test-agent", "snap1", gzip.DefaultCompression)
	require.NoError(t, err)

	// 验证快照目录存在
	snapshotDir := filepath.Join(tmpDir, "agents", "test-agent", "backups", "snap1")
	assert.DirExists(t, snapshotDir)
}

func TestRollbackSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入初始文件
	upperFile := filepath.Join(tmpDir, "agents", "test-agent", "upper", "data.txt")
	err = os.WriteFile(upperFile, []byte("original"), 0644)
	require.NoError(t, err)

	// 创建快照
	err = store.CreateSnapshot(ctx, "test-agent", "snap1", gzip.DefaultCompression)
	require.NoError(t, err)

	// 修改文件
	err = os.WriteFile(upperFile, []byte("modified"), 0644)
	require.NoError(t, err)

	// 回滚
	err = store.RollbackSnapshot(ctx, "test-agent", "snap1")
	require.NoError(t, err)

	// 验证文件恢复
	content, err := os.ReadFile(upperFile)
	require.NoError(t, err)
	assert.Equal(t, []byte("original"), content)
}

func TestPruneCache(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "agent-a", 64, 1024*1024*1024)
	require.NoError(t, err)
	_, err = store.CreateAgent(ctx, "agent-b", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 创建缓存目录
	cacheDir := filepath.Join(tmpDir, "cache", "rootfs")
	os.MkdirAll(filepath.Join(cacheDir, "used-cache"), 0700)
	os.MkdirAll(filepath.Join(cacheDir, "unused-cache"), 0700)

	// 设置 agent-a 使用 used-cache
	meta, _ := store.GetAgent(ctx, "agent-a")
	meta.CacheKey = "used-cache"
	store.UpdateAgent(ctx, meta)

	// Prune
	deleted, err := store.PruneCache(ctx, false)
	require.NoError(t, err)
	assert.Contains(t, deleted, "unused-cache")
	assert.NotContains(t, deleted, "used-cache")
}

func TestAgentMetaPaths(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	meta, err := store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 验证路径是否正确填充
	assert.Equal(t, filepath.Join(tmpDir, "agents", "test-agent", "rootfs"), meta.RootfsDir)
	assert.Equal(t, filepath.Join(tmpDir, "agents", "test-agent", "upper"), meta.UpperDir)
	assert.Equal(t, filepath.Join(tmpDir, "agents", "test-agent", "work"), meta.WorkDir)
	assert.Equal(t, filepath.Join(tmpDir, "agents", "test-agent", "workspace"), meta.Workspace)
}

func TestListSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入一些文件
	upperFile := filepath.Join(tmpDir, "agents", "test-agent", "upper", "data.txt")
	err = os.WriteFile(upperFile, []byte("snapshot data"), 0644)
	require.NoError(t, err)

	// 创建多个快照
	err = store.CreateSnapshot(ctx, "test-agent", "snap1", gzip.DefaultCompression)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond) // 确保时间不同

	err = store.CreateSnapshot(ctx, "test-agent", "snap2", gzip.DefaultCompression)
	require.NoError(t, err)

	// 列出快照
	snapshots, err := store.ListSnapshots(ctx, "test-agent")
	require.NoError(t, err)
	assert.Len(t, snapshots, 2)

	// 验证按时间倒序排列
	assert.Equal(t, "snap2", snapshots[0].SnapshotID)
	assert.Equal(t, "snap1", snapshots[1].SnapshotID)

	// 验证 ParentID
	assert.Equal(t, "snap1", snapshots[0].ParentID)
	assert.Empty(t, snapshots[1].ParentID)
}

func TestListSnapshotsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 列出不存在的快照
	snapshots, err := store.ListSnapshots(ctx, "test-agent")
	require.NoError(t, err)
	assert.Empty(t, snapshots)
}

func TestRollbackSnapshotMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入初始文件
	upperFile := filepath.Join(tmpDir, "agents", "test-agent", "upper", "data.txt")
	err = os.WriteFile(upperFile, []byte("v1"), 0644)
	require.NoError(t, err)

	// 创建快照1
	err = store.CreateSnapshot(ctx, "test-agent", "snap1", gzip.DefaultCompression)
	require.NoError(t, err)

	// 修改文件
	err = os.WriteFile(upperFile, []byte("v2"), 0644)
	require.NoError(t, err)

	// 创建快照2
	err = store.CreateSnapshot(ctx, "test-agent", "snap2", gzip.DefaultCompression)
	require.NoError(t, err)

	// 再次修改文件
	err = os.WriteFile(upperFile, []byte("v3"), 0644)
	require.NoError(t, err)

	// 回滚到快照2
	err = store.RollbackSnapshot(ctx, "test-agent", "snap2")
	require.NoError(t, err)

	content, _ := os.ReadFile(upperFile)
	assert.Equal(t, []byte("v2"), content)

	// 回滚到快照1
	err = store.RollbackSnapshot(ctx, "test-agent", "snap1")
	require.NoError(t, err)

	content, _ = os.ReadFile(upperFile)
	assert.Equal(t, []byte("v1"), content)
}

func TestCreateSnapshotWithWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入 upper 和 workspace 文件
	upperFile := filepath.Join(tmpDir, "agents", "test-agent", "upper", "data.txt")
	workspaceFile := filepath.Join(tmpDir, "agents", "test-agent", "workspace", "work.txt")

	err = os.WriteFile(upperFile, []byte("upper data"), 0644)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Dir(workspaceFile), 0755)
	require.NoError(t, err)
	err = os.WriteFile(workspaceFile, []byte("workspace data"), 0644)
	require.NoError(t, err)

	// 创建快照
	err = store.CreateSnapshot(ctx, "test-agent", "snap1", gzip.DefaultCompression)
	require.NoError(t, err)

	// 验证快照包含 upper 和 workspace
	snapshotDir := filepath.Join(tmpDir, "agents", "test-agent", "backups", "snap1")
	assert.FileExists(t, filepath.Join(snapshotDir, "upper.tar.gz"))
	assert.FileExists(t, filepath.Join(snapshotDir, "workspace.tar.gz"))
	assert.FileExists(t, filepath.Join(snapshotDir, "meta.json"))
}

func TestForkAgentWithSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "source", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 写入文件
	upperFile := filepath.Join(tmpDir, "agents", "source", "upper", "data.txt")
	err = os.WriteFile(upperFile, []byte("source data"), 0644)
	require.NoError(t, err)

	// 创建快照
	err = store.CreateSnapshot(ctx, "source", "snap1", gzip.DefaultCompression)
	require.NoError(t, err)

	// Fork
	_, err = store.ForkAgent(ctx, "source", "forked")
	require.NoError(t, err)

	// 验证 fork 的 agent 没有快照
	snapshots, err := store.ListSnapshots(ctx, "forked")
	require.NoError(t, err)
	assert.Empty(t, snapshots)

	// 验证 fork 的 agent 有正确的文件
	forkedFile := filepath.Join(tmpDir, "agents", "forked", "upper", "data.txt")
	assert.FileExists(t, forkedFile)
	content, _ := os.ReadFile(forkedFile)
	assert.Equal(t, []byte("source data"), content)
}

// TestCopyPhysical 测试物理拷贝
func TestCopyPhysical(t *testing.T) {
	// 创建源目录
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// 创建测试文件和子目录
	srcFile := filepath.Join(srcDir, "test.txt")
	err := os.WriteFile(srcFile, []byte("hello world"), 0644)
	require.NoError(t, err)

	srcSubdir := filepath.Join(srcDir, "subdir")
	err = os.MkdirAll(srcSubdir, 0755)
	require.NoError(t, err)

	srcSubFile := filepath.Join(srcSubdir, "nested.txt")
	err = os.WriteFile(srcSubFile, []byte("nested content"), 0644)
	require.NoError(t, err)

	// 执行物理拷贝
	err = copyPhysical(srcDir, dstDir)
	require.NoError(t, err)

	// 验证文件存在
	dstFile := filepath.Join(dstDir, "test.txt")
	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(content))

	// 验证子目录和文件存在
	dstSubFile := filepath.Join(dstDir, "subdir", "nested.txt")
	content, err = os.ReadFile(dstSubFile)
	require.NoError(t, err)
	assert.Equal(t, "nested content", string(content))
}

// TestCopyPhysicalEmptyDir 测试空目录拷贝
func TestCopyPhysicalEmptyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	err := copyPhysical(srcDir, dstDir)
	require.NoError(t, err)

	// 验证目标目录存在
	_, err = os.Stat(dstDir)
	assert.NoError(t, err)
}

// TestCheckSameDevice 测试设备检查
func TestCheckSameDevice(t *testing.T) {
	// 测试相同设备
	err := checkSameDevice("/tmp", "/tmp")
	assert.NoError(t, err)

	// 测试少于2个路径
	err = checkSameDevice("/tmp")
	assert.NoError(t, err)

	err = checkSameDevice()
	assert.NoError(t, err)
}

// TestCheckSameDeviceDifferentDevices 测试不同设备
func TestCheckSameDeviceDifferentDevices(t *testing.T) {
	// 获取 /tmp 和当前目录的设备
	tmpDir := t.TempDir()

	// 创建子目录确保在同一设备上
	subDir := filepath.Join(tmpDir, "sub")
	err := os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	// 测试同一设备上的路径
	err = checkSameDevice(tmpDir, subDir)
	assert.NoError(t, err)
}

// TestAtomicExchange 测试原子交换
func TestAtomicExchange(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	// 创建源文件和目标文件
	source := filepath.Join(tmpDir, "source.txt")
	target := filepath.Join(tmpDir, "target.txt")

	err := os.WriteFile(source, []byte("source content"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(target, []byte("target content"), 0644)
	require.NoError(t, err)

	// 执行原子交换
	err = atomicExchange(source, target)
	require.NoError(t, err)

	// 验证内容已交换
	content, err := os.ReadFile(source)
	require.NoError(t, err)
	assert.Equal(t, "target content", string(content))

	content, err = os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "source content", string(content))
}

// TestAtomicExchangeNonExistent 测试不存在的文件
func TestAtomicExchangeNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	source := filepath.Join(tmpDir, "nonexistent.txt")
	target := filepath.Join(tmpDir, "target.txt")

	err := os.WriteFile(target, []byte("target content"), 0644)
	require.NoError(t, err)

	// 交换不存在的文件应该返回错误
	err = atomicExchange(source, target)
	assert.Error(t, err)
}

// TestListAgentsEmpty 测试空目录
func TestListAgentsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	agents, err := store.ListAgents(ctx)
	require.NoError(t, err)
	assert.Empty(t, agents)
}

// TestListAgentsInvalidDir 测试无效目录
func TestListAgentsInvalidDir(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	// 创建无效的 agent 目录（没有 meta.json）
	invalidDir := filepath.Join(tmpDir, "agents", "invalid-agent")
	os.MkdirAll(invalidDir, 0755)

	ctx := context.Background()
	agents, err := store.ListAgents(ctx)
	require.NoError(t, err)
	assert.Empty(t, agents) // 应该跳过无效目录
}

// TestAtomicWriteFile 测试原子写文件
func TestAtomicWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "test.txt")

	// 写入数据
	err := atomicWriteFile(filename, []byte("hello world"))
	require.NoError(t, err)

	// 验证内容
	content, err := os.ReadFile(filename)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(content))
}

// TestAtomicWriteFileDirNotExist 测试目录不存在
func TestAtomicWriteFileDirNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	filename := filepath.Join(tmpDir, "subdir", "test.txt")

	// 目录不存在，应该返回错误
	err := atomicWriteFile(filename, []byte("hello world"))
	assert.Error(t, err)
}

// TestRecreateWorkDir 测试重建 work 目录
func TestRecreateWorkDir(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, "work")

	// 创建 work 目录
	os.MkdirAll(workDir, 0755)
	os.WriteFile(filepath.Join(workDir, "test.txt"), []byte("test"), 0644)

	// 重建 work 目录
	err := recreateWorkDir(workDir)
	require.NoError(t, err)

	// 验证 work 目录存在
	_, err = os.Stat(workDir)
	assert.NoError(t, err)

	// 验证旧文件已删除
	_, err = os.Stat(filepath.Join(workDir, "test.txt"))
	assert.True(t, os.IsNotExist(err))
}

// TestRecreateWorkDirExistingPurge 测试已存在的 purge 目录
func TestRecreateWorkDirExistingPurge(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, "work")
	purgeDir := workDir + ".purge"

	// 创建 work 目录和 purge 目录
	os.MkdirAll(workDir, 0755)
	os.MkdirAll(purgeDir, 0755)
	os.WriteFile(filepath.Join(workDir, "test.txt"), []byte("test"), 0644)

	// 重建 work 目录（purge 目录已存在）
	err := recreateWorkDir(workDir)
	require.NoError(t, err)

	// 验证 work 目录存在
	_, err = os.Stat(workDir)
	assert.NoError(t, err)
}

// TestRollbackSnapshotNotFound 测试快照不存在
func TestRollbackSnapshotNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "test-agent", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 回滚不存在的快照
	err = store.RollbackSnapshot(ctx, "test-agent", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestDecompressCopy 测试解压复制
func TestDecompressCopy(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 tar.gz 文件
	srcFile := filepath.Join(tmpDir, "test.tar.gz")
	f, err := os.Create(srcFile)
	require.NoError(t, err)

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// 添加一个文件
	err = tw.WriteHeader(&tar.Header{
		Name: "test.txt",
		Mode: 0644,
		Size: 11,
	})
	require.NoError(t, err)
	_, err = tw.Write([]byte("hello world"))
	require.NoError(t, err)

	tw.Close()
	gw.Close()
	f.Close()

	// 解压到目标目录
	dstDir := filepath.Join(tmpDir, "dst")
	err = decompressCopy(srcFile, dstDir)
	require.NoError(t, err)

	// 验证文件存在
	dstFile := filepath.Join(dstDir, "test.txt")
	content, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(content))
}

// TestDecompressCopyInvalidFile 测试无效文件
func TestDecompressCopyInvalidFile(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建无效的 tar.gz 文件
	srcFile := filepath.Join(tmpDir, "invalid.tar.gz")
	os.WriteFile(srcFile, []byte("not a valid tar.gz"), 0644)

	dstDir := filepath.Join(tmpDir, "dst")
	err := decompressCopy(srcFile, dstDir)
	assert.Error(t, err)
}

// TestPruneCacheDryRun 测试 dry run 模式
func TestPruneCacheDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "agent-a", 64, 1024*1024*1024)
	require.NoError(t, err)

	// 创建缓存目录
	cacheDir := filepath.Join(tmpDir, "cache", "rootfs")
	os.MkdirAll(filepath.Join(cacheDir, "unused-cache"), 0700)

	// Prune dry run
	deleted, err := store.PruneCache(ctx, true)
	require.NoError(t, err)
	assert.Contains(t, deleted, "unused-cache")

	// 验证缓存目录仍然存在
	_, err = os.Stat(filepath.Join(cacheDir, "unused-cache"))
	assert.NoError(t, err)
}

// TestPruneCacheEmpty 测试空缓存
func TestPruneCacheEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStore(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = store.CreateAgent(ctx, "agent-a", 64, 1024*1024*1024)
	require.NoError(t, err)

	// Prune 空缓存
	deleted, err := store.PruneCache(ctx, false)
	require.NoError(t, err)
	assert.Empty(t, deleted)
}
