package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDownloadSingleThread(t *testing.T) {
	// 创建测试服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 100; i++ {
			w.Write([]byte{byte(i)})
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
		_ = r.Header.Get("Range")
		ranges = append(ranges, r.Header.Get("Range"))
		rangeHeader := r.Header.Get("Range")

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
		} else {
			// 没有 Range 头，返回全部内容
			start = 0
			end = 999
		}

		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(http.StatusPartialContent)
		for i := start; i <= end; i++ {
			w.Write([]byte{byte(i % 256)})
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
			w.Write([]byte{byte(i)})
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
			w.Write([]byte{byte(i)})
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
