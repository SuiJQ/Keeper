// Package downloader 提供多线程分块下载能力
package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"keeper/internal/log"
)

// Config 下载配置
type Config struct {
	URL              string
	OutputPath       string
	Threads          int
	ChunkSize        int64
	Timeout          time.Duration
	ProgressCallback func(downloaded, total int64)
	MaxRetries       int
	RetryDelay       time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Threads:    4,
		ChunkSize:  1024 * 1024, // 1MB
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		RetryDelay: 2 * time.Second,
	}
}

// Downloader 多线程下载器
type Downloader struct {
	config *Config
	logger log.Logger
}

// NewDownloader 创建下载器实例
func NewDownloader(config *Config, logger log.Logger) *Downloader {
	if logger == nil {
		logger = log.Global()
	}
	if config == nil {
		config = DefaultConfig()
	}
	if config.Threads <= 0 {
		config.Threads = 4
	}
	if config.ChunkSize <= 0 {
		config.ChunkSize = 1024 * 1024 // 1MB
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.RetryDelay <= 0 {
		config.RetryDelay = 2 * time.Second
	}
	return &Downloader{
		config: config,
		logger: logger,
	}
}

// Download 执行多线程下载
func (d *Downloader) Download(ctx context.Context) error {
	var lastErr error
	for attempt := 0; attempt < d.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.config.RetryDelay):
			}
		}

		// 1. 获取文件大小和 ETag
		totalSize, etag, supportRange, err := d.getFileInfo(ctx)
		if err != nil {
			// 如果无法获取文件信息，回退到单线程下载（带重试）
			d.logger.Warn("failed to get file info, falling back to single thread", log.Field{Key: "error", Value: err.Error()})
			if err := d.downloadUnknownSize(ctx); err != nil {
				lastErr = err
				continue
			}
			return nil
		}

		d.logger.Info("download started",
			log.Field{Key: "url", Value: d.config.URL},
			log.Field{Key: "size", Value: totalSize},
			log.Field{Key: "etag", Value: etag},
			log.Field{Key: "range_support", Value: supportRange})

		// 2. 创建输出文件
		file, err := os.Create(d.config.OutputPath)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer func() { _ = file.Close() }()

		if err := file.Truncate(totalSize); err != nil {
			return fmt.Errorf("truncate file: %w", err)
		}

		// 3. 如果不支持 Range 或文件较小，使用单线程下载
		if !supportRange || totalSize < d.config.ChunkSize {
			if err := d.downloadSingleThread(ctx, file, totalSize); err != nil {
				lastErr = err
				continue
			}
			return nil
		}

		// 4. 多线程分块下载
		if err := d.downloadMultiThread(ctx, file, totalSize); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("download failed after %d retries: %w", d.config.MaxRetries, lastErr)
}

// downloadUnknownSize 下载未知大小的文件
func (d *Downloader) downloadUnknownSize(ctx context.Context) error {
	file, err := os.Create(d.config.OutputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() { _ = file.Close() }()

	req, err := http.NewRequestWithContext(ctx, "GET", d.config.URL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: d.config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	buf := make([]byte, 32*1024)
	var downloaded int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := file.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if d.config.ProgressCallback != nil {
				d.config.ProgressCallback(downloaded, downloaded)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
func (d *Downloader) getFileInfo(ctx context.Context) (int64, string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", d.config.URL, nil)
	if err != nil {
		return 0, "", false, err
	}

	client := &http.Client{Timeout: d.config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		// HEAD 失败，尝试 GET
		return d.getFileInfoFromGET(ctx)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.ContentLength < 0 {
		return 0, "", false, fmt.Errorf("unknown file size")
	}

	etag := resp.Header.Get("ETag")
	supportRange := resp.Header.Get("Accept-Ranges") == "bytes"

	return resp.ContentLength, etag, supportRange, nil
}

// getFileInfoFromGET 通过 GET 获取文件信息（仅读取前 1 字节）
func (d *Downloader) getFileInfoFromGET(ctx context.Context) (int64, string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", d.config.URL, nil)
	if err != nil {
		return 0, "", false, err
	}
	req.Header.Set("Range", "bytes=0-0")

	client := &http.Client{Timeout: d.config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", false, err
	}
	defer func() { _ = resp.Body.Close() }()

	// 读取响应头获取总大小
	totalSize := resp.ContentLength
	if totalSize < 0 {
		// 尝试从 Content-Range 解析
		contentRange := resp.Header.Get("Content-Range")
		if strings.HasPrefix(contentRange, "bytes 0-0/") {
			if size, err := strconv.ParseInt(strings.Split(contentRange, "/")[1], 10, 64); err == nil {
				totalSize = size
			}
		}
	}

	if totalSize < 0 {
		// 如果仍然无法获取大小，尝试读取全部内容
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return 0, "", false, err
		}
		totalSize = int64(len(body))
	}

	return totalSize, "", false, nil
}

// downloadSingleThread 单线程下载
func (d *Downloader) downloadSingleThread(ctx context.Context, file *os.File, totalSize int64) error {
	req, err := http.NewRequestWithContext(ctx, "GET", d.config.URL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: d.config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	buf := make([]byte, 32*1024)
	var downloaded int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := file.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if d.config.ProgressCallback != nil {
				d.config.ProgressCallback(downloaded, totalSize)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// downloadMultiThread 多线程分块下载
func (d *Downloader) downloadMultiThread(ctx context.Context, file *os.File, totalSize int64) error {
	// 计算分块
	var chunks []Chunk
	offset := int64(0)
	for offset < totalSize {
		end := offset + d.config.ChunkSize - 1
		if end >= totalSize {
			end = totalSize - 1
		}
		chunks = append(chunks, Chunk{
			Start: offset,
			End:   end,
		})
		offset = end + 1
	}

	d.logger.Info("download chunks",
		log.Field{Key: "total_chunks", Value: len(chunks)},
		log.Field{Key: "chunk_size", Value: d.config.ChunkSize})

	// 并发下载
	var wg sync.WaitGroup
	errCh := make(chan error, len(chunks))
	var mu sync.Mutex
	var downloaded int64

	for i, chunk := range chunks {
		wg.Add(1)
		go func(idx int, c Chunk) {
			defer wg.Done()
			if err := d.downloadChunk(ctx, file, c, totalSize); err != nil {
				errCh <- fmt.Errorf("chunk %d: %w", idx, err)
				return
			}
			mu.Lock()
			downloaded += (c.End - c.Start + 1)
			if d.config.ProgressCallback != nil {
				d.config.ProgressCallback(downloaded, totalSize)
			}
			mu.Unlock()
		}(i, chunk)
	}

	// 等待完成
	wg.Wait()
	close(errCh)

	// 检查错误
	for err := range errCh {
		return err
	}

	return nil
}

// Chunk 下载分块
type Chunk struct {
	Start int64
	End   int64
}

// downloadChunk 下载单个分块
func (d *Downloader) downloadChunk(ctx context.Context, file *os.File, chunk Chunk, totalSize int64) error {
	var lastErr error
	for attempt := 0; attempt < d.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.config.RetryDelay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", d.config.URL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", chunk.Start, chunk.End))

		client := &http.Client{Timeout: d.config.Timeout}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}

		buf := make([]byte, 32*1024)
		offset := chunk.Start
		for {
			select {
			case <-ctx.Done():
				_ = resp.Body.Close()
				return ctx.Err()
			default:
			}

			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := file.WriteAt(buf[:n], offset); werr != nil {
					_ = resp.Body.Close()
					return werr
				}
				offset += int64(n)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				lastErr = err
				break
			}
		}
		_ = resp.Body.Close()

		if offset > chunk.End {
			lastErr = nil
			break
		}
	}

	return lastErr
}

// DownloadFile 便捷函数：下载文件到指定路径
func DownloadFile(ctx context.Context, url, outputPath string) error {
	config := DefaultConfig()
	config.URL = url
	config.OutputPath = outputPath
	downloader := NewDownloader(config, nil)
	return downloader.Download(ctx)
}

// DownloadFileWithProgress 带进度的下载
func DownloadFileWithProgress(ctx context.Context, url, outputPath string, callback func(downloaded, total int64)) error {
	config := DefaultConfig()
	config.URL = url
	config.OutputPath = outputPath
	config.ProgressCallback = callback
	downloader := NewDownloader(config, nil)
	return downloader.Download(ctx)
}
