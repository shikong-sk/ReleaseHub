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

// DownloadWithResume 支持断点续传的下载，offset > 0 时发 Range 请求
func (d *HTTPDownloader) DownloadWithResume(ctx context.Context, url string, token string, writer io.Writer, offset int64) (*Result, error) {
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
		// 续传成功
		resumed = true
		if resp.ContentLength > 0 {
			totalSize = offset + resp.ContentLength
		}
		bodyReader = resp.Body
	} else if resp.StatusCode == http.StatusOK {
		// 服务端不支持 Range，返回完整内容
		totalSize = resp.ContentLength
		if offset > 0 {
			// 期望续传但服务端返回 200，需跳过已下载部分
			// ponytail: 简单跳过 offset 字节，大文件时浪费带宽但保证正确
			if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, offset)); err != nil {
				return nil, fmt.Errorf("跳过已下载部分失败: %w", err)
			}
		}
	} else {
		return nil, fmt.Errorf("下载响应异常: HTTP %d", resp.StatusCode)
	}

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(writer, hasher), bodyReader)
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

func (d *HTTPDownloader) Download(ctx context.Context, url string, token string, writer io.Writer) (*Result, error) {
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
	written, err := io.Copy(io.MultiWriter(writer, hasher), resp.Body)
	if err != nil {
		return nil, fmt.Errorf("下载写入失败: %w", err)
	}

	return &Result{
		Size:   written,
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}
