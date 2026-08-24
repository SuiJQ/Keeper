package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetHomeDir(t *testing.T) {
	// 保存原始环境变量
	origHome := os.Getenv("HOME")
	origKeeperHome := os.Getenv("KEEPER_HOME")
	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("KEEPER_HOME", origKeeperHome)
	}()

	tests := []struct {
		name       string
		home       string
		keeperHome string
		want       string
	}{
		{
			name:       "default uses HOME",
			home:       "/tmp/test-home",
			keeperHome: "",
			want:       filepath.Join("/tmp/test-home", defaultHome),
		},
		{
			name:       "KEEPER_HOME overrides HOME",
			home:       "/tmp/test-home",
			keeperHome: "/custom/keeper/home",
			want:       "/custom/keeper/home",
		},
		{
			name:       "empty HOME with KEEPER_HOME",
			home:       "",
			keeperHome: "/another/path",
			want:       "/another/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("HOME", tt.home)
			os.Setenv("KEEPER_HOME", tt.keeperHome)

			got := getHomeDir()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsValidName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"valid-name", true},
		{"valid_name", true},
		{"ValidName123", true},
		{"a", true},
		{"a1", true},
		{"1a", true},
		{"", false},
		{"-invalid", false},
		{"_invalid", false},
		{"invalid!", false},
		{"invalid@name", false},
		{"invalid name", false},
		{"this-name-is-way-too-long-to-be-valid-because-it-exceeds-the-maximum-length", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidName(tt.name)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStatDevice(t *testing.T) {
	// 使用一个确定不存在的路径
	tmpDir := t.TempDir()
	dev := statDevice(filepath.Join(tmpDir, "this-definitely-does-not-exist-12345"))
	assert.Equal(t, uint64(0), dev)

	// 测试当前目录（某些环境下可能是设备，只需保证不崩溃）
	dev = statDevice(".")
	assert.NotNil(t, dev)
}

func TestKillProcess(t *testing.T) {
	// 测试杀死不存在进程
	err := killProcess(999999)
	assert.Error(t, err)
	// 错误信息可能是 "process not found" 或 "os: process already finished"
	// 取决于操作系统如何报告不存在的进程
	assert.Contains(t, err.Error(), "process")
}

func TestParseAgentPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantPath string
		wantOK   bool
	}{
		{"simple file", "simple:/file.txt", "simple", "/file.txt", true},
		{"nested path", "nested:/dir/subdir/file.txt", "nested", "/dir/subdir/file.txt", true},
		{"root path", "agent:/", "agent", "/", true},
		{"no colon", "agent", "", "", false},
		{"empty agent", ":/file.txt", "", "", false},
		{"empty path", "agent:", "agent", ".", true},
		{"local file", "/local/path", "", "", false},
		{"windows path", "agent:C:\\path", "agent", "C:\\path", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, path, ok := parseAgentPath(tt.input)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantPath, path)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestCopyLocalToLocal(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")

	// 写入源文件
	require.NoError(t, os.WriteFile(src, []byte("hello world"), 0644))

	// 复制文件
	err := copyLocalToLocal(src, dst)
	require.NoError(t, err)

	// 验证目标文件
	content, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), content)
}

func TestCopyLocalToLocalMissingSource(t *testing.T) {
	tmpDir := t.TempDir()

	err := copyLocalToLocal(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dst"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source not found")
}

func TestCopyLocalToLocalDirectorySource(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src")
	dst := filepath.Join(tmpDir, "dst")

	// 创建源目录
	require.NoError(t, os.MkdirAll(src, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "file.txt"), []byte("hello"), 0644))

	// 复制目录
	err := copyLocalToLocal(src, dst)
	require.NoError(t, err)

	// 验证目录已复制
	assert.FileExists(t, filepath.Join(dst, "file.txt"))
}

func TestCopyDirRecursiveMissingSource(t *testing.T) {
	tmpDir := t.TempDir()

	err := copyDirRecursive(filepath.Join(tmpDir, "nonexistent"), filepath.Join(tmpDir, "dst"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "lstat")
}

func TestCopyDirRecursiveFileSource(t *testing.T) {
	tmpDir := t.TempDir()

	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst")

	// 创建源文件
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0644))

	// copyDirRecursive 对文件源会返回 nil（filepath.Walk 可以处理文件）
	err := copyDirRecursive(src, dst)
	assert.NoError(t, err)

	// 验证文件已复制（作为文件而不是目录）
	assert.FileExists(t, dst)
}
