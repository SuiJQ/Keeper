package downloader

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadSingleThread(t *testing.T) {
	// 创建测试服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 100; i++ {
			if _, err := w.Write([]byte{byte(i)}); err != nil {
				t.Logf("write error: %v", err)
				break
			}
		}
	}))
	defer ts.Close()

	// 下载文件
	tmpFile := "/tmp/test_download_single.bin"
	defer os.Remove(tmpFile)

	config := &Config{
		URL:        ts.URL,
		OutputPath: tmpFile,
		Threads:    1,
		Timeout:    10 * time.Second,
		MaxRetries: 1,
		RetryDelay: 100 * time.Millisecond,
	}
	downloader := NewDownloader(config, nil)
	err := downloader.Download(context.Background())
	assert.NoError(t, err)

	// 验证文件大小
	info, err := os.Stat(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, int64(100), info.Size())
}

func TestDownloadMultiThread(t *testing.T) {
	// 创建测试服务器，支持 Range
	var ranges []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		rangeHeader := r.Header.Get("Range")

		// 设置支持 Range
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", "1000")

		if rangeHeader == "" {
			// HEAD 请求或没有 Range 的 GET 请求
			w.WriteHeader(http.StatusOK)
			return
		}

		var start, end int64
		if strings.HasPrefix(rangeHeader, "bytes=") {
			parts := strings.Split(strings.TrimPrefix(rangeHeader, "bytes="), "-")
			if len(parts) == 2 {
				start = parseInt64(parts[0])
				if parts[1] != "" {
					end = parseInt64(parts[1])
				} else {
					end = 999
				}
			}
		}

		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/1000", start, end))
		w.WriteHeader(http.StatusPartialContent)
		for i := start; i <= end; i++ {
			if _, err := w.Write([]byte{byte(i % 256)}); err != nil {
				t.Logf("write error: %v", err)
				break
			}
		}
	}))
	defer ts.Close()

	// 下载文件
	tmpFile := "/tmp/test_download_multi.bin"
	defer os.Remove(tmpFile)

	config := &Config{
		URL:        ts.URL,
		OutputPath: tmpFile,
		Threads:    4,
		ChunkSize:  250,
		Timeout:    10 * time.Second,
		MaxRetries: 1,
		RetryDelay: 100 * time.Millisecond,
	}
	downloader := NewDownloader(config, nil)
	err := downloader.Download(context.Background())
	assert.NoError(t, err)

	// 验证文件大小
	info, err := os.Stat(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, int64(1000), info.Size())

	// 验证多线程请求
	assert.GreaterOrEqual(t, len(ranges), 2)
}

func TestDownloadWithProgress(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 100; i++ {
			if _, err := w.Write([]byte{byte(i)}); err != nil {
				t.Logf("write error: %v", err)
				break
			}
		}
	}))
	defer ts.Close()

	tmpFile := "/tmp/test_download_progress.bin"
	defer os.Remove(tmpFile)

	var progressCalls int
	var lastProgress int64
	config := &Config{
		URL:        ts.URL,
		OutputPath: tmpFile,
		Threads:    1,
		Timeout:    10 * time.Second,
		ProgressCallback: func(downloaded, total int64) {
			progressCalls++
			lastProgress = downloaded
		},
	}
	downloader := NewDownloader(config, nil)
	err := downloader.Download(context.Background())
	assert.NoError(t, err)
	assert.Greater(t, progressCalls, 0)
	assert.Equal(t, int64(100), lastProgress)
}

func TestDownloadWithRetry(t *testing.T) {
	attempt := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", "50")
			w.WriteHeader(http.StatusOK)
			return
		}
		attempt++
		if attempt < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", "50")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 50; i++ {
			if _, err := w.Write([]byte{byte(i)}); err != nil {
				t.Logf("write error: %v", err)
				break
			}
		}
	}))
	defer ts.Close()

	tmpFile := "/tmp/test_download_retry.bin"
	defer os.Remove(tmpFile)

	config := &Config{
		URL:        ts.URL,
		OutputPath: tmpFile,
		Threads:    1,
		Timeout:    10 * time.Second,
		MaxRetries: 3,
		RetryDelay: 100 * time.Millisecond,
	}
	downloader := NewDownloader(config, nil)
	err := downloader.Download(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 3, attempt)
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func TestDownloadFile(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "200")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 200; i++ {
			if _, err := w.Write([]byte{byte(i % 256)}); err != nil {
				t.Logf("write error: %v", err)
				break
			}
		}
	}))
	defer ts.Close()

	tmpFile := "/tmp/test_download_file.bin"
	defer os.Remove(tmpFile)

	err := DownloadFile(context.Background(), ts.URL, tmpFile)
	assert.NoError(t, err)

	info, err := os.Stat(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, int64(200), info.Size())
}

func TestDownloadFileWithProgress(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "150")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 150; i++ {
			if _, err := w.Write([]byte{byte(i % 256)}); err != nil {
				t.Logf("write error: %v", err)
				break
			}
		}
	}))
	defer ts.Close()

	tmpFile := "/tmp/test_download_progress_file.bin"
	defer os.Remove(tmpFile)

	var totalProgress int64
	var lastProgress int64
	callback := func(downloaded, total int64) {
		totalProgress++
		lastProgress = downloaded
	}

	err := DownloadFileWithProgress(context.Background(), ts.URL, tmpFile, callback)
	assert.NoError(t, err)
	assert.Greater(t, totalProgress, int64(0))
	assert.Equal(t, int64(150), lastProgress)
}

func TestDownloadWithContextCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		// 缓慢发送数据
		for i := 0; i < 1000000; i++ {
			if _, err := w.Write([]byte{byte(i % 256)}); err != nil {
				t.Logf("write error: %v", err)
				break
			}
			if i%10000 == 0 {
				time.Sleep(1 * time.Millisecond)
			}
		}
	}))
	defer ts.Close()

	tmpFile := "/tmp/test_download_cancel.bin"
	defer os.Remove(tmpFile)

	config := &Config{
		URL:        ts.URL,
		OutputPath: tmpFile,
		Threads:    1,
		Timeout:    30 * time.Second,
		MaxRetries: 1,
		RetryDelay: 100 * time.Millisecond,
	}
	downloader := NewDownloader(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := downloader.Download(ctx)
	assert.Error(t, err)
}

func TestDownloadWithInvalidURL(t *testing.T) {
	tmpFile := "/tmp/test_download_invalid.bin"
	defer os.Remove(tmpFile)

	err := DownloadFile(context.Background(), "http://invalid.url.that.does.not.exist:12345/test", tmpFile)
	assert.Error(t, err)
}

func TestDownloadFileConvenience(t *testing.T) {
	// 创建测试服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 100; i++ {
			if _, err := w.Write([]byte{byte(i)}); err != nil {
				break
			}
		}
	}))
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "downloaded.bin")

	err := DownloadFile(context.Background(), ts.URL+"/100bytes", tmpFile)
	assert.NoError(t, err)

	data, err := os.ReadFile(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, 100, len(data))
}

func TestDownloadFileWithProgressCallback(t *testing.T) {
	// 创建测试服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 100; i++ {
			if _, err := w.Write([]byte{byte(i)}); err != nil {
				break
			}
		}
	}))
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "downloaded.bin")
	var progressCalled bool
	var lastDownloaded int64

	err := DownloadFileWithProgress(context.Background(), ts.URL+"/100bytes", tmpFile, func(downloaded, total int64) {
		progressCalled = true
		lastDownloaded = downloaded
	})
	assert.NoError(t, err)
	assert.True(t, progressCalled, "progress callback should be called")
	assert.Equal(t, int64(100), lastDownloaded)
}

func TestDownloadSingleThreadFallback(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "downloaded.bin")

	// 创建一个不支持 Range 的服务器
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "none")
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 100; i++ {
			_, _ = w.Write([]byte{byte(i)})
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	downloader := NewDownloader(&Config{
		URL:        server.URL,
		OutputPath: tmpFile,
		Threads:    4,
	}, nil)

	err := downloader.Download(context.Background())
	assert.NoError(t, err)

	data, err := os.ReadFile(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, 100, len(data))
}

func TestDownloadChunkRetry(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "downloaded.bin")

	requestCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		// 支持 Range 请求
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			// HEAD 或 GET 请求（获取文件信息）
			if requestCount <= 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Length", "100")
			w.Header().Set("Accept-Ranges", "bytes")
			w.WriteHeader(http.StatusOK)
			for i := 0; i < 100; i++ {
				_, _ = w.Write([]byte{byte(i)})
			}
			return
		}

		// Range 请求
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %s/100", rangeHeader[6:]))
		w.WriteHeader(http.StatusPartialContent)
		for i := 0; i < 100; i++ {
			_, _ = w.Write([]byte{byte(i)})
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	downloader := NewDownloader(&Config{
		URL:        server.URL,
		OutputPath: tmpFile,
		Threads:    1,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
	}, nil)

	err := downloader.Download(context.Background())
	assert.NoError(t, err)
	// 验证请求次数（前2次失败，第3次成功）
	assert.GreaterOrEqual(t, requestCount, 3, "should have at least 3 requests")
}

func TestDownloadContextCancel(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "downloaded.bin")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	downloader := NewDownloader(&Config{
		URL:        server.URL,
		OutputPath: tmpFile,
		Threads:    1,
	}, nil)

	err := downloader.Download(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestDownloadConfigDefaults(t *testing.T) {
	config := DefaultConfig()
	assert.Equal(t, 4, config.Threads)
	assert.Equal(t, int64(1024*1024), config.ChunkSize)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 2*time.Second, config.RetryDelay)
}

func TestNewDownloaderWithNilConfig(t *testing.T) {
	downloader := NewDownloader(nil, nil)
	assert.NotNil(t, downloader)
	assert.NotNil(t, downloader.config)
	assert.Equal(t, 4, downloader.config.Threads)
}

func TestGetFileInfoFromGET(t *testing.T) {
	// 创建测试服务器，返回 Content-Range
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			for i := 0; i < 100; i++ {
				_, _ = w.Write([]byte{byte(i)})
			}
			return
		}

		// 返回 1 字节，但 Content-Range 表示总大小
		w.Header().Set("Content-Length", "1")
		w.Header().Set("Content-Range", "bytes 0-0/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer ts.Close()

	downloader := NewDownloader(&Config{
		URL:        ts.URL,
		OutputPath: filepath.Join(t.TempDir(), "test.bin"),
		Threads:    1,
	}, nil)

	size, etag, supportRange, err := downloader.getFileInfoFromGET(context.Background())
	assert.NoError(t, err)
	// getFileInfoFromGET 返回 Content-Length（1），不会解析 Content-Range
	assert.Equal(t, int64(1), size)
	assert.Empty(t, etag)
	assert.False(t, supportRange)
}

func TestGetFileInfoFromGETWithContentLength(t *testing.T) {
	// 创建测试服务器，返回 Content-Length
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "50")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 50; i++ {
			_, _ = w.Write([]byte{byte(i)})
		}
	}))
	defer ts.Close()

	downloader := NewDownloader(&Config{
		URL:        ts.URL,
		OutputPath: filepath.Join(t.TempDir(), "test.bin"),
		Threads:    1,
	}, nil)

	size, _, _, err := downloader.getFileInfoFromGET(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, int64(50), size)
}

func TestDownloadUnknownSize(t *testing.T) {
	// 创建测试服务器，返回流式数据
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 100; i++ {
			_, _ = w.Write([]byte{byte(i)})
		}
	}))
	defer ts.Close()

	downloader := NewDownloader(&Config{
		URL:        ts.URL,
		OutputPath: filepath.Join(t.TempDir(), "unknown_size.bin"),
		Threads:    1,
	}, nil)

	err := downloader.Download(context.Background())
	assert.NoError(t, err)

	data, err := os.ReadFile(downloader.config.OutputPath)
	assert.NoError(t, err)
	assert.Equal(t, 100, len(data))
}

func TestDownloadChunk(t *testing.T) {
	// 创建测试服务器，支持 Range
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		if rangeHeader == "" {
			w.Header().Set("Content-Length", "100")
			w.WriteHeader(http.StatusOK)
			for i := 0; i < 100; i++ {
				_, _ = w.Write([]byte{byte(i)})
			}
			return
		}

		// 解析 Range
		var start, end int64
		if strings.HasPrefix(rangeHeader, "bytes=") {
			parts := strings.Split(strings.TrimPrefix(rangeHeader, "bytes="), "-")
			if len(parts) == 2 {
				start = parseInt64(parts[0])
				if parts[1] != "" {
					end = parseInt64(parts[1])
				} else {
					end = 99
				}
			}
		}

		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/100", start, end))
		w.WriteHeader(http.StatusPartialContent)
		for i := start; i <= end; i++ {
			_, _ = w.Write([]byte{byte(i)})
		}
	}))
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "chunked.bin")
	file, err := os.Create(tmpFile)
	require.NoError(t, err)
	defer file.Close()

	// 写 100 字节占位
	_ = file.Truncate(100)

	downloader := NewDownloader(&Config{
		URL:        ts.URL,
		OutputPath: tmpFile,
		Threads:    1,
	}, nil)

	ctx := context.Background()
	err = downloader.downloadChunk(ctx, file, Chunk{Start: 0, End: 49})
	assert.NoError(t, err)

	data, err := os.ReadFile(tmpFile)
	assert.NoError(t, err)
	assert.Equal(t, 100, len(data))
}

// TestDownloadChunkHTTPError 测试 downloadChunk HTTP 错误
func TestDownloadChunkHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "error_chunked.bin")
	file, err := os.Create(tmpFile)
	require.NoError(t, err)
	defer file.Close()

	_ = file.Truncate(100)

	downloader := NewDownloader(&Config{
		URL:        ts.URL,
		OutputPath: tmpFile,
		Threads:    1,
		MaxRetries: 1,
		RetryDelay: 0,
	}, nil)

	ctx := context.Background()
	err = downloader.downloadChunk(ctx, file, Chunk{Start: 0, End: 49})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

// TestDownloadChunkContextCancel 测试 downloadChunk 上下文取消
func TestDownloadChunkContextCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 慢速响应
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Range", "bytes 0-49/100")
		w.WriteHeader(http.StatusPartialContent)
		for i := 0; i < 50; i++ {
			_, _ = w.Write([]byte{byte(i)})
		}
	}))
	defer ts.Close()

	tmpFile := filepath.Join(t.TempDir(), "cancel_chunked.bin")
	file, err := os.Create(tmpFile)
	require.NoError(t, err)
	defer file.Close()

	_ = file.Truncate(100)

	downloader := NewDownloader(&Config{
		URL:        ts.URL,
		OutputPath: tmpFile,
		Threads:    1,
		Timeout:    5 * time.Second,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	// 立即取消
	cancel()

	err = downloader.downloadChunk(ctx, file, Chunk{Start: 0, End: 49})
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

// TestDownloadChunkWriteError 测试 downloadChunk 写入错误
func TestDownloadChunkWriteError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes 0-49/100")
		w.WriteHeader(http.StatusPartialContent)
		for i := 0; i < 50; i++ {
			_, _ = w.Write([]byte{byte(i)})
		}
	}))
	defer ts.Close()

}
