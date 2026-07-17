package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type HTTPDownloader struct {
	client *http.Client
	// limiter 下载速率限制器，nil=不限速。
	// 用 golang.org/x/time/rate 令牌桶限制读取速率，
	// io.CopyBuffer 读慢后写自然放慢，从而实现全局限速。
	limiter *rate.Limiter
}

type Result struct {
	Size       int64
	SHA256     string
	Resumed    bool  // 是否续传
	TotalSize  int64 // 完整文件大小（续传时有用）
}

// ProgressFunc 进度回调：已下载字节数（含续传 offset）、总字节数（未知为 0）
type ProgressFunc func(downloaded int64, total int64)

// Downloader 资产下载器抽象。HTTPDownloader 为默认实现，Aria2Downloader 为 aria2 RPC 实现。
// asset.Service 持该接口（非具体类型），据 config.Download.Aria2RPC 是否空在 HTTP/Aria2 间选用。
// 方法覆盖现 asset 调用面：DownloadWithProgress 为主路径，续传族供未来/测试。
type Downloader interface {
	// DownloadWithProgress 下载到 writer 并回调进度，返回含 SHA256 的结果
	DownloadWithProgress(ctx context.Context, url string, token string, writer io.Writer, onProgress ProgressFunc) (*Result, error)
	// DownloadWithResumeAndProgress 支持断点续传 + 进度回调的下载
	DownloadWithResumeAndProgress(ctx context.Context, url string, token string, writer io.Writer, offset int64, onProgress ProgressFunc) (*Result, error)
	// DownloadWithResume 支持断点续传的下载（无进度回调）
	DownloadWithResume(ctx context.Context, url string, token string, writer io.Writer, offset int64) (*Result, error)
	// Download 基础下载（无进度回调，向后兼容）
	Download(ctx context.Context, url string, token string, writer io.Writer) (*Result, error)
}

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
	// 限速包装：在 progressReader 之外加令牌桶限速，读慢则 copy 自然放慢
	limitedReader := d.applyRate(ctx, progressReader)
	written, err := io.CopyBuffer(io.MultiWriter(writer, hasher), limitedReader, make([]byte, 64*1024))
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
	// 限速包装：在 progressReader 之外加令牌桶限速，读慢则 copy 自然放慢
	limitedReader := d.applyRate(ctx, progressReader)
	written, err := io.CopyBuffer(io.MultiWriter(writer, hasher), limitedReader, make([]byte, 64*1024))
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

// NewHTTPDownloaderWithLimit 创建带速率限制的 HTTP 下载器。
// maxSpeed<=0 时不限速（向后兼容），>0 时按字节/秒限速。
func NewHTTPDownloaderWithLimit(maxSpeed int64) *HTTPDownloader {
	return &HTTPDownloader{
		client:  &http.Client{Timeout: 30 * time.Minute},
		limiter: newDownloadLimiter(maxSpeed),
	}
}

// NewHTTPDownloaderWithTransportAndLimit 创建带 Transport 与速率限制的 HTTP 下载器
func NewHTTPDownloaderWithTransportAndLimit(transport *http.Transport, maxSpeed int64) *HTTPDownloader {
	return &HTTPDownloader{
		client:  &http.Client{Timeout: 30 * time.Minute, Transport: transport},
		limiter: newDownloadLimiter(maxSpeed),
	}
}

// newDownloadLimiter 根据限速值构造令牌桶，maxSpeed<=0 返回 nil（不限速）
func newDownloadLimiter(maxSpeed int64) *rate.Limiter {
	if maxSpeed <= 0 {
		return nil
	}
	// burst 取 maxSpeed 与 4MB 的较小值：既保证 64KB chunk 不会被分次等待，
	// 又避免大限速值下突发过大；token 是浮点计数不预分配内存。
	burst := maxSpeed
	if burst > 4*1024*1024 {
		burst = 4 * 1024 * 1024
	}
	return rate.NewLimiter(rate.Limit(maxSpeed), int(burst))
}

// applyRate 包装 reader 加限速；limiter 为 nil 时原样返回（不限速）。
// golang.org/x/time v0.15.0 的 rate 包尚未提供 NewReader，故用标准库
// rate.Limiter（令牌桶）配合 rateReader 把 WaitN 接入 io.Reader，
// 等价于官方更高版本的 rate.NewReader。
func (d *HTTPDownloader) applyRate(ctx context.Context, r io.Reader) io.Reader {
	if d.limiter == nil {
		return r
	}
	return &rateReader{r: r, limiter: d.limiter, ctx: ctx}
}

// rateReader 在每次 Read 后按读取字节数消耗令牌桶令牌：令牌不足时 WaitN
// 阻塞至有令牌或 ctx 取消，从而限制读取速率；io.CopyBuffer 读慢则写自然放慢。
// 单次读取量截断到 limiter.Burst()，保证 WaitN(n) 不会因 n>burst 报错。
type rateReader struct {
	r       io.Reader
	limiter *rate.Limiter
	ctx     context.Context
}

func (lr *rateReader) Read(p []byte) (int, error) {
	if b := lr.limiter.Burst(); b > 0 && len(p) > b {
		p = p[:b]
	}
	n, err := lr.r.Read(p)
	if n > 0 {
		// 消耗 n 个令牌，不足时阻塞等待；ctx 取消时立即返回错误终止下载
		if werr := lr.limiter.WaitN(lr.ctx, n); werr != nil {
			return n, werr
		}
	}
	return n, err
}
