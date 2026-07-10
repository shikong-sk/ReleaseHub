package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPDownloader struct {
	client *http.Client
}

type Result struct {
	Size       int64
	SHA256     string
	Resumed    bool  // 是否续传
	TotalSize  int64 // 完整文件大小（续传时有用）
}

// ProgressFunc 进度回调：已下载字节数（含续传 offset）、总字节数（未知为 0）
type ProgressFunc func(downloaded int64, total int64)

// DownloadWithResumeAndProgress 支持断点续传 + 进度回调的下载
// offset > 0 时发 Range 请求；onProgress 在 io.Copy 每 64KB 触发
func (d *HTTPDownloader) DownloadWithResumeAndProgress(ctx context.Context, url string, token string, writer io.Writer, offset int64, onProgress ProgressFunc) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ReleaseHub")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 处理响应：206 Partial Content 或 200 OK
	var bodyReader io.Reader = resp.Body
	var totalSize int64
	resumed := false

	if resp.StatusCode == http.StatusPartialContent {
		resumed = true
		if resp.ContentLength > 0 {
			totalSize = offset + resp.ContentLength
		}
		bodyReader = resp.Body
	} else if resp.StatusCode == http.StatusOK {
		totalSize = resp.ContentLength
		if offset > 0 {
			if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, offset)); err != nil {
				return nil, fmt.Errorf("跳过已下载部分失败: %w", err)
			}
			if onProgress != nil {
				onProgress(offset, totalSize)
			}
		}
	} else {
		return nil, fmt.Errorf("下载响应异常: HTTP %d", resp.StatusCode)
	}

	hasher := sha256.New()
	// 用 TeeReader 在每次读后触发进度回调，避免 io.CopyBuffer 的硬编码 32KB
	progressReader := &progressReader{r: bodyReader, downloaded: offset, total: totalSize, onProgress: onProgress}
	written, err := io.CopyBuffer(io.MultiWriter(writer, hasher), progressReader, make([]byte, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("下载写入失败: %w", err)
	}

	return &Result{
		Size:      offset + written,
		SHA256:    hex.EncodeToString(hasher.Sum(nil)),
		Resumed:   resumed,
		TotalSize: totalSize,
	}, nil
}

// progressReader 包装 Reader，在每次 Read 后触发进度回调
type progressReader struct {
	r          io.Reader
	downloaded int64
	total      int64
	onProgress ProgressFunc
}

func (p *progressReader) Read(buf []byte) (int, error) {
	n, err := p.r.Read(buf)
	if n > 0 {
		p.downloaded += int64(n)
		if p.onProgress != nil {
			p.onProgress(p.downloaded, p.total)
		}
	}
	return n, err
}

// DownloadWithResume 支持断点续传的下载（无进度回调）
func (d *HTTPDownloader) DownloadWithResume(ctx context.Context, url string, token string, writer io.Writer, offset int64) (*Result, error) {
	return d.DownloadWithResumeAndProgress(ctx, url, token, writer, offset, nil)
}

// DownloadWithProgress 基础下载 + 进度回调（无续传）
func (d *HTTPDownloader) DownloadWithProgress(ctx context.Context, url string, token string, writer io.Writer, onProgress ProgressFunc) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ReleaseHub")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载响应异常: HTTP %d", resp.StatusCode)
	}

	hasher := sha256.New()
	progressReader := &progressReader{r: resp.Body, total: resp.ContentLength, onProgress: onProgress}
	written, err := io.CopyBuffer(io.MultiWriter(writer, hasher), progressReader, make([]byte, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("下载写入失败: %w", err)
	}

	return &Result{
		Size:   written,
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

// NewHTTPDownloader 创建默认的 HTTP 下载器
func NewHTTPDownloader() *HTTPDownloader {
	return &HTTPDownloader{
		client: &http.Client{
			Timeout: 30 * time.Minute,
		},
	}
}

// NewHTTPDownloaderWithTransport 创建使用指定 Transport 的 HTTP 下载器
func NewHTTPDownloaderWithTransport(transport *http.Transport) *HTTPDownloader {
	return &HTTPDownloader{
		client: &http.Client{
			Timeout:   30 * time.Minute,
			Transport: transport,
		},
	}
}

// Download 基础下载（无进度回调，向后兼容）
func (d *HTTPDownloader) Download(ctx context.Context, url string, token string, writer io.Writer) (*Result, error) {
	return d.DownloadWithProgress(ctx, url, token, writer, nil)
}
