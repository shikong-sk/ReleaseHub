package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"releasehub/backend/internal/services/aria2"
)

// aria2 轮询间隔与总超时（不纳入运行时热更新；如需运维调整，重启进程改 env 即可）。
// pollInterval 为包级 var 而非 const，便于测试覆盖为更短值（如 10ms）加速用例；生产默认 1s。
var aria2PollInterval = 1 * time.Second
const aria2DownloadTimeout = 30 * time.Minute

// Aria2Downloader 实现 Downloader 接口，通过 aria2 JSON-RPC 异步离线下载：
//
//	AddURI(带 Bearer 鉴权、可选 dir) 提交任务 → 轮询 TellStatus 到 complete/error/removed
//	→ 从 aria2 返回的完成文件绝对路径读取文件喂给 caller 传入的 writer（asset 端 io.Pipe writer，
//	驱动 storage Put），SHA256 用 MultiWriter 在读时与写入同步计算，与 HTTP 路径 Result 语义一致。
//
// 环境前提（重要）：ReleaseHub 进程必须对 aria2 完成文件的本地路径有读权限。
// 若 aria2 daemon 与 ReleaseHub 运行在不同容器/卷，文件不可读 → 返回明确 error（供 syncer
// 自动重试并暴露配置问题），不静默回退 HTTP。
//
// 本地文件清理：copyLocalToWriter 用 defer os.Remove 删除完成文件依赖 io.Pipe 无缓冲同步特性 ——
// io.Copy 返回时所有数据已被 pipe reader（storage Put）消费，故删本地文件安全；若未来下载链路
// 改为带缓冲 pipe 或异步度更高的写入，此 defer 可能过早删文件，需同步调整清理时机。
type Aria2Downloader struct {
	client   *aria2.Client
	aria2Dir string // 显式下载目录；空=依赖 daemon 自身配置
}

// NewAria2Downloader 创建 aria2 RPC 下载器。
// rpcURL/secret/httpURL 为 aria2 daemon 连接参数；dir 非空则把 aria2 下载引导到该目录。
func NewAria2Downloader(rpcURL, secret, httpURL, dir string) (*Aria2Downloader, error) {
	c, err := aria2.NewClient(aria2.Config{
		RPCEndpoint:  rpcURL,
		Secret:       secret,
		HTTPEndpoint: httpURL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 aria2 客户端失败: %w", err)
	}
	return &Aria2Downloader{client: c, aria2Dir: dir}, nil
}

// Download 基础下载（无进度回调），委托主方法
func (a *Aria2Downloader) Download(ctx context.Context, url string, token string, writer io.Writer) (*Result, error) {
	return a.DownloadWithProgress(ctx, url, token, writer, nil)
}

// DownloadWithProgress 主下载路径：aria2 AddURI → 轮询 TellStatus → 读完成文件喂 writer
func (a *Aria2Downloader) DownloadWithProgress(ctx context.Context, url string, token string, writer io.Writer, onProgress ProgressFunc) (*Result, error) {
	// 1. AddURI 提交下载任务
	options := map[string]interface{}{}
	if a.aria2Dir != "" {
		options["dir"] = a.aria2Dir
	}
	// 下载源鉴权走 Bearer header（与 HTTPDownloader 语义一致）；aria2 daemon RPC 鉴权走 client.secret，两者别混淆
	if token != "" {
		options["header"] = []string{"Authorization: Bearer " + token}
	}
	gid, err := a.client.AddURI(ctx, []string{url}, options)
	if err != nil {
		return nil, fmt.Errorf("aria2 AddURI 失败: %w", err)
	}

	// 2. 轮询 TellStatus 直到终态；用 detached ctx 在 ctx 取消/超时后仍能 Remove aria2 任务
	ticker := time.NewTicker(aria2PollInterval)
	defer ticker.Stop()
	timeout := time.NewTimer(aria2DownloadTimeout)
	defer timeout.Stop()

	var localPath string
	var totalSize int64
pollLoop:
	for {
		select {
		case <-ctx.Done():
			_, _ = a.client.Remove(context.Background(), gid)
			return nil, ctx.Err()
		case <-timeout.C:
			_, _ = a.client.Remove(context.Background(), gid)
			return nil, fmt.Errorf("aria2 下载超时（%s），gid=%s", aria2DownloadTimeout, gid)
		case <-ticker.C:
			status, stErr := a.client.TellStatus(ctx, gid, "status", "files", "totalLength", "errorCode")
			if stErr != nil {
				// RPC 暂时性错误（如 daemon 抖动），本轮跳过待下次 tick 再试
				continue
			}
			st, _ := status["status"].(string)
			switch st {
			case "complete":
				localPath, totalSize, err = a.extractFileInfo(status)
				if err != nil {
					_, _ = a.client.Remove(context.Background(), gid)
					return nil, err
				}
				break pollLoop
			case "error":
				_, _ = a.client.Remove(context.Background(), gid)
				return nil, a.statusError(status, "aria2 下载失败")
			case "removed":
				return nil, fmt.Errorf("aria2 任务被外部移除，gid=%s", gid)
			}
			// active/waiting/paused：继续轮询
		}
	}

	// 3. 完成后清理 aria2 任务记录（避免 aria2 任务池堆积），随后读本地完成文件喂 writer
	_, _ = a.client.Remove(context.Background(), gid)

	return a.copyLocalToWriter(ctx, localPath, totalSize, writer, onProgress)
}

func (a *Aria2Downloader) DownloadWithResumeAndProgress(ctx context.Context, url string, token string, writer io.Writer, offset int64, onProgress ProgressFunc) (*Result, error) {
	// aria2 为异步全量下载语义：caller 传入的 offset 是 HTTP Range 语义，
	// aria2 模式下忽略 offset 委托主方法 —— aria2 daemon 自身的 .aria2 control file
	// 控制同一 URL 未完成部分的断点续传，与 HTTP Range offset 是正交的两套机制。
	// （asset 主路径只调 DownloadWithProgress，续传族仅满足接口齐备。）
	_ = offset
	return a.DownloadWithProgress(ctx, url, token, writer, onProgress)
}

// DownloadWithResume 续传族（无进度回调），委托 DownloadWithResumeAndProgress
func (a *Aria2Downloader) DownloadWithResume(ctx context.Context, url string, token string, writer io.Writer, offset int64) (*Result, error) {
	return a.DownloadWithResumeAndProgress(ctx, url, token, writer, offset, nil)
}

// extractFileInfo 解析 aria2 complete 任务的完成文件绝对路径与总字节数
func (a *Aria2Downloader) extractFileInfo(status map[string]interface{}) (path string, totalSize int64, err error) {
	files, ok := status["files"].([]interface{})
	if !ok || len(files) == 0 {
		return "", 0, fmt.Errorf("aria2 完成但未返回 files 信息")
	}
	fileMap, ok := files[0].(map[string]interface{})
	if !ok {
		return "", 0, fmt.Errorf("aria2 完成但 files[0] 非对象")
	}
	path, ok = fileMap["path"].(string)
	if !ok || path == "" {
		return "", 0, fmt.Errorf("aria2 完成但完成文件路径为空")
	}
	if total, ok2 := status["totalLength"].(string); ok2 {
		if n, perr := strconv.ParseInt(total, 10, 64); perr == nil {
			totalSize = n
		}
	}
	return path, totalSize, nil
}

// statusError 构造 aria2 错误响应的 error，尽量带 errorCode
func (a *Aria2Downloader) statusError(status map[string]interface{}, prefix string) error {
	if ec, ok := status["errorCode"].(string); ok && ec != "" {
		return fmt.Errorf("%s，errorCode=%s", prefix, ec)
	}
	return fmt.Errorf("%s", prefix)
}

// copyLocalToWriter 打开 aria2 完成的本地文件，用 ctxReader 响应 ctx 取消，progressReader 回调进度，
// MultiWriter 同步算 SHA256；读完后 defer 清理本地文件避免堆积。
func (a *Aria2Downloader) copyLocalToWriter(ctx context.Context, localPath string, totalSize int64, writer io.Writer, onProgress ProgressFunc) (*Result, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("打开 aria2 完成文件失败（路径: %s，可能 ReleaseHub 进程无读权限）: %w", localPath, err)
	}
	defer f.Close()
	defer os.Remove(localPath) // 依赖 io.Pipe 无缓冲同步:io.Copy 返回时数据已被 storage 消费;Put 完成或失败均清理本地文件

	// 复用 HTTPDownloader 同款 progressReader（同包），但底层 reader 包一层 ctxReader 以响应取消
	reader := &progressReader{
		r:          &ctxReader{r: f, ctx: ctx},
		total:      totalSize,
		onProgress: onProgress,
	}
	hasher := sha256.New()
	written, err := io.CopyBuffer(io.MultiWriter(writer, hasher), reader, make([]byte, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("读取 aria2 完成文件喂存储失败: %w", err)
	}

	return &Result{
		Size:   written,
		SHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

// ctxReader 包装 reader，在每次 Read 前检查 ctx；ctx 取消时 Read 返回 ctx.Err()
// 使 io.Copy 能被 ctx 取消中断（与 HTTP 路径通过 HTTP client 响应 ctx 的语义对齐）。
type ctxReader struct {
	r   io.Reader
	ctx context.Context
}

func (cr *ctxReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}
