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
	Size   int64
	SHA256 string
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
