package downloader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockAria2Server 构造一个响应 aria2 JSON-RPC 的测试服务器：
//   addUri → 返回假 gid；tellStatus 第一次 active、之后 complete（指向传入完成文件路径）；
//   remove → 返回空字符串。
// pollCount 用于断言轮询次数，完成后 remove 释放 aria2 任务记录。
func mockAria2Server(t *testing.T, completePath string) (string, *int32) {
	t.Helper()
	var pollCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method  string        `json:"method"`
			ID      string        `json:"id"`
			Params  []interface{} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)

		var result interface{}
		switch req.Method {
		case "aria2.addUri":
			result = "deadbeef"
		case "aria2.tellStatus":
			n := atomic.AddInt32(&pollCount, 1)
			if n == 1 {
				result = map[string]interface{}{"status": "active"}
			} else {
				result = map[string]interface{}{
					"status":      "complete",
					"totalLength": "7",
					"files":       []interface{}{map[string]interface{}{"path": completePath}},
				}
			}
		case "aria2.remove":
			result = "ok"
		default:
			result = nil
		}
		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &pollCount
}

// TestAria2DownloaderComplete 验证 AddURI → 轮询 TellStatus active→complete → 读本地完成文件
// 喂 writer + SHA256 同步计算 + defer 清理本地文件
func TestAria2DownloaderComplete(t *testing.T) {
	orig := aria2PollInterval
	aria2PollInterval = 10 * time.Millisecond
	t.Cleanup(func() { aria2PollInterval = orig })

	// 准备完成文件（aria2 下载到本地的落点）
	dir := t.TempDir()
	data := []byte("hello!")
	wantSHA := sha256Hex(data)
	completePath := filepath.Join(dir, "asset.bin")
	if err := os.WriteFile(completePath, data, 0644); err != nil {
		t.Fatalf("写入完成文件失败: %v", err)
	}

	rpcURL, pollCount := mockAria2Server(t, completePath)

	// NewAria2Downloader：URL 截取到 http://host:port（aria2.NewClient 要 http JSON-RPC 端点）
	// rpcURL 已是 http://127.0.0.1:port，直接用作 RPC 端点
	d, err := NewAria2Downloader(rpcURL, "", "", "")
	if err != nil {
		t.Fatalf("NewAria2Downloader 失败: %v", err)
	}

	var buf bytes.Buffer
	result, err := d.DownloadWithProgress(context.Background(), "http://example.com/asset.bin", "tok", &buf, nil)
	if err != nil {
		t.Fatalf("Aria2 下载失败: %v", err)
	}
	if result.Size != int64(len(data)) {
		t.Errorf("Size 不匹配: got %d want %d", result.Size, len(data))
	}
	if result.SHA256 != wantSHA {
		t.Errorf("SHA256 不匹配: got %s want %s", result.SHA256, wantSHA)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Error("下载内容不一致")
	}
	if atomic.LoadInt32(pollCount) < 2 {
		t.Errorf("轮询次数 %d，期望 >=2（active→complete）", atomic.LoadInt32(pollCount))
	}
	// 本地完成文件已被 defer os.Remove 清理
	if _, statErr := os.Stat(completePath); !os.IsNotExist(statErr) {
		t.Errorf("本地完成文件未清理: %v", statErr)
	}
}

// TestAria2DownloaderHeaderAndDir 验证 AddURI 收到 Bearer header 与 dir option
func TestAria2DownloaderHeaderAndDir(t *testing.T) {
	orig := aria2PollInterval
	aria2PollInterval = 10 * time.Millisecond
	t.Cleanup(func() { aria2PollInterval = orig })

	dir := t.TempDir()
	data := []byte("dir-opt")
	completePath := filepath.Join(dir, "x.bin")
	_ = os.WriteFile(completePath, data, 0644)

	var gotHeader, gotDir interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Method == "aria2.addUri" {
			// params: [token:secret, [uris], options]
			if len(req.Params) >= 3 {
				if opt, ok := req.Params[2].(map[string]interface{}); ok {
					gotHeader = opt["header"]
					gotDir = opt["dir"]
				}
			}
		}
var result interface{} = "gid-hd"
		if req.Method == "aria2.tellStatus" {
			result = map[string]interface{}{
				"status":      "complete",
				"totalLength": "7",
				"files":       []interface{}{map[string]interface{}{"path": completePath}},
			}
		}
		if req.Method == "aria2.remove" {
			result = "ok"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0", "id": "1", "result": result,
		})
	}))
	t.Cleanup(srv.Close)

	d, err := NewAria2Downloader(srv.URL, "rpcSecret", "", dir)
	if err != nil {
		t.Fatalf("NewAria2Downloader 失败: %v", err)
	}
	var buf bytes.Buffer
	if _, err := d.DownloadWithProgress(context.Background(), "http://ex/a", "tok", &buf, nil); err != nil {
		t.Fatalf("下载失败: %v", err)
	}
	// header 应为 ["Authorization: Bearer tok"]
	hdr, _ := gotHeader.([]interface{})
	if len(hdr) != 1 || !strings.Contains(fmt.Sprintf("%v", hdr[0]), "Bearer tok") {
		t.Errorf("AddURI header 期望 [Authorization: Bearer tok], got %#v", gotHeader)
	}
	// dir 应为传入的 dir
	if gotDir != dir {
		t.Errorf("AddURI dir 期望 %s, got %v", dir, gotDir)
	}
}

// TestAria2DownloaderError 验证 aria2 status=error 返回明确 error 且不静默回退
func TestAria2DownloaderError(t *testing.T) {
	orig := aria2PollInterval
	aria2PollInterval = 10 * time.Millisecond
	t.Cleanup(func() { aria2PollInterval = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
var result interface{} = "gid-err"
		if req.Method == "aria2.tellStatus" {
			result = map[string]interface{}{"status": "error", "errorCode": "42"}
		}
		if req.Method == "aria2.remove" {
			result = "ok"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0", "id": "1", "result": result,
		})
	}))
	t.Cleanup(srv.Close)

	d, err := NewAria2Downloader(srv.URL, "", "", "")
	if err != nil {
		t.Fatalf("NewAria2Downloader 失败: %v", err)
	}
	var buf bytes.Buffer
	_, err = d.DownloadWithProgress(context.Background(), "http://ex/a", "", &buf, nil)
	if err == nil {
		t.Fatal("期望 status=error 返回 error，got nil")
	}
	if !strings.Contains(err.Error(), "errorCode=42") {
		t.Errorf("期望错误含 errorCode=42, got: %v", err)
	}
}

// TestAria2DownloaderCancel 验证 ctx 取消 → Remove + 返回 ctx.Err
func TestAria2DownloaderCancel(t *testing.T) {
	orig := aria2PollInterval
	aria2PollInterval = 10 * time.Millisecond
	t.Cleanup(func() { aria2PollInterval = orig })

	var removed int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
var result interface{} = "gid-cancel"
		if req.Method == "aria2.tellStatus" {
			result = map[string]interface{}{"status": "active"} // 一直 active，等 ctx 取消
		}
		if req.Method == "aria2.remove" {
			atomic.AddInt32(&removed, 1)
			result = "ok"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0", "id": "1", "result": result,
		})
	}))
	t.Cleanup(srv.Close)

	d, err := NewAria2Downloader(srv.URL, "", "", "")
	if err != nil {
		t.Fatalf("NewAria2Downloader 失败: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	// 50ms 后取消，确保轮询一直在 active
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	var buf bytes.Buffer
	_, err = d.DownloadWithProgress(ctx, "http://ex/a", "", &buf, nil)
	if err == nil {
		t.Fatal("期望 ctx 取消返回 error，got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("期望 error 链含 context.Canceled, got: %v", err)
	}
	if atomic.LoadInt32(&removed) != 1 {
		t.Errorf("期望取消后 Remove 1 次, got %d", atomic.LoadInt32(&removed))
	}
}

// sha256Hex 计算输入的 SHA256 十六进制串
func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
