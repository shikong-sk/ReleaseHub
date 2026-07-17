package downloader

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// makeData 生成指定大小的重复模式测试数据
func makeData(n int) []byte {
	return bytes.Repeat([]byte{0xAB}, n)
}

// rangeServer 创建支持 Range 请求的测试服务器
func rangeServer(data []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rng := r.Header.Get("Range")
		if rng != "" {
			var start int64
			fmt.Sscanf(rng, "bytes=%d-", &start)
			w.Header().Set("Content-Length", strconv.Itoa(len(data)-int(start)))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(data[start:])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}))
}

// TestDownloadWithRateLimit 验证设定限速后下载耗时显著增加
func TestDownloadWithRateLimit(t *testing.T) {
	// 512KB at 256KB/s: burst 256KB 后剩余 256KB@256KB/s ≈ 1s
	data := makeData(512 * 1024)
	server := rangeServer(data)
	defer server.Close()

	d := NewHTTPDownloaderWithLimit(256 * 1024)
	var buf bytes.Buffer
	start := time.Now()
	result, err := d.DownloadWithProgress(context.Background(), server.URL, "", &buf, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("限速下载失败: %v", err)
	}
	if result.Size != int64(len(data)) {
		t.Errorf("下载大小不匹配: got %d, want %d", result.Size, len(data))
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("下载内容不一致")
	}
	// burst 消耗后剩余 256KB@256KB/s ≈ 1s，允许浮动
	if elapsed < 700*time.Millisecond {
		t.Errorf("限速未生效: 512KB@256KB/s 耗时 %v, 期望 >=700ms", elapsed)
	}
	t.Logf("限速下载 512KB@256KB/s 耗时 %v", elapsed)
}

// TestDownloadWithoutLimit 验证不限速时下载快速完成
func TestDownloadWithoutLimit(t *testing.T) {
	data := makeData(512 * 1024)
	server := rangeServer(data)
	defer server.Close()

	d := NewHTTPDownloader()
	var buf bytes.Buffer
	start := time.Now()
	_, err := d.DownloadWithProgress(context.Background(), server.URL, "", &buf, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("不限速下载失败: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("不限速下载 512KB 耗时 %v, 期望 <500ms", elapsed)
	}
	t.Logf("不限速下载 512KB 耗时 %v", elapsed)
}

// TestDownloadWithResumeAndRateLimit 验证断点续传下载同样受限速约束
func TestDownloadWithResumeAndRateLimit(t *testing.T) {
	// 512KB total, offset=256KB → 续传下载 256KB
	// 128KB/s: burst 128KB 后剩余 128KB@128KB/s ≈ 1s
	data := makeData(512 * 1024)
	server := rangeServer(data)
	defer server.Close()

	d := NewHTTPDownloaderWithLimit(128 * 1024)
	var buf bytes.Buffer
	start := time.Now()
	result, err := d.DownloadWithResumeAndProgress(context.Background(), server.URL, "", &buf, 256*1024, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("续传限速下载失败: %v", err)
	}
	if !result.Resumed {
		t.Error("期望 resumed=true，服务器应返回 206")
	}
	// result.Size = offset + 本次续传字节数 = 256KB + 256KB = 512KB（完整文件）
	if result.Size != 512*1024 {
		t.Errorf("续传下载大小不匹配: got %d, want %d", result.Size, 512*1024)
	}
	// buf 仅含本次续传写入的 256KB
	if buf.Len() != 256*1024 {
		t.Errorf("续传本批次写入大小不匹配: got %d, want %d", buf.Len(), 256*1024)
	}
	if !bytes.Equal(buf.Bytes(), data[256*1024:]) {
		t.Error("续传下载内容不一致")
	}
	// 续传部分 256KB, burst 128KB → 剩余 128KB@128KB/s ≈ 1s
	if elapsed < 700*time.Millisecond {
		t.Errorf("续传限速未生效: 256KB@128KB/s 耗时 %v, 期望 >=700ms", elapsed)
	}
	t.Logf("续传限速 256KB@128KB/s 耗时 %v", elapsed)
}
