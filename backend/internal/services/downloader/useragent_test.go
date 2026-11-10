package downloader

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestHTTPDownloaderUserAgent 校验 UA 注入：默认值、动态函数、空值回退
func TestHTTPDownloaderUserAgent(t *testing.T) {
	cases := []struct {
		name  string
		setup func() *HTTPDownloader
		want  string
	}{
		{
			name:  "未注入使用默认 UA",
			setup: func() *HTTPDownloader { return NewHTTPDownloader() },
			want:  "ReleaseHub",
		},
		{
			name: "注入自定义 UA",
			setup: func() *HTTPDownloader {
				return NewHTTPDownloader().WithUserAgentFunc(func() string { return "Mozilla/5.0 Test" })
			},
			want: "Mozilla/5.0 Test",
		},
		{
			name: "函数返回空回退默认",
			setup: func() *HTTPDownloader {
				return NewHTTPDownloader().WithUserAgentFunc(func() string { return "" })
			},
			want: "ReleaseHub",
		},
		{
			name: "限速构造也支持注入",
			setup: func() *HTTPDownloader {
				return NewHTTPDownloaderWithLimit(1024).WithUserAgentFunc(func() string { return "Limited/1.0" })
			},
			want: "Limited/1.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotUA string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotUA = r.Header.Get("User-Agent")
				_, _ = w.Write([]byte("hi"))
			}))
			defer server.Close()

			var buf bytes.Buffer
			if _, err := tc.setup().DownloadWithProgress(context.Background(), server.URL+"/f", "", &buf, nil); err != nil {
				t.Fatalf("下载失败: %v", err)
			}
			if gotUA != tc.want {
				t.Fatalf("User-Agent 不符\n期望: %s\n实际: %s", tc.want, gotUA)
			}
		})
	}
}

// TestHTTPDownloaderUserAgentDynamic 确认 UA 每个请求都重新求值（配置热更新即时生效）
func TestHTTPDownloaderUserAgentDynamic(t *testing.T) {
	current := "first/1.0"
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("User-Agent"))
		_, _ = w.Write([]byte("hi"))
	}))
	defer server.Close()

	d := NewHTTPDownloader().WithUserAgentFunc(func() string { return current })

	var buf bytes.Buffer
	if _, err := d.DownloadWithProgress(context.Background(), server.URL+"/f", "", &buf, nil); err != nil {
		t.Fatalf("首次下载失败: %v", err)
	}
	current = "second/2.0"
	buf.Reset()
	if _, err := d.DownloadWithProgress(context.Background(), server.URL+"/f", "", &buf, nil); err != nil {
		t.Fatalf("二次下载失败: %v", err)
	}

	if len(got) != 2 || got[0] != "first/1.0" || got[1] != "second/2.0" {
		t.Fatalf("UA 未按次求值: %v", got)
	}
}

// TestAria2DownloaderUserAgentOption 校验 aria2 的 user-agent 选项下发
// （aria2 用 user-agent 选项而非 header 字段设置 UA）
func TestAria2DownloaderUserAgentOption(t *testing.T) {
	cases := []struct {
		name        string
		uaFn        func() string
		wantUA      string
		wantHasOpt  bool
	}{
		{name: "注入自定义 UA", uaFn: func() string { return "Mozilla/5.0 Aria" }, wantUA: "Mozilla/5.0 Aria", wantHasOpt: true},
		{name: "函数返回空不下发", uaFn: func() string { return "" }, wantHasOpt: false},
		{name: "未注入不下发", uaFn: nil, wantHasOpt: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			completePath := dir + "/asset.bin"
			if err := os.WriteFile(completePath, []byte("hello!"), 0644); err != nil {
				t.Fatalf("准备完成文件失败: %v", err)
			}

			var addURIOptions map[string]interface{}
			var pollCount int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var req struct {
					Method string        `json:"method"`
					ID     string        `json:"id"`
					Params []interface{} `json:"params"`
				}
				_ = json.Unmarshal(body, &req)

				var result interface{}
				switch req.Method {
				case "aria2.addUri":
					// params: [secret?, urls, options]
					for _, p := range req.Params {
						if m, ok := p.(map[string]interface{}); ok {
							addURIOptions = m
						}
					}
					result = "gid1"
				case "aria2.tellStatus":
					pollCount++
					result = map[string]interface{}{
						"status":      "complete",
						"totalLength": "6",
						"files":       []interface{}{map[string]interface{}{"path": completePath}},
					}
				case "aria2.remove":
					result = "ok"
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": result})
			}))
			defer server.Close()

			orig := aria2PollInterval
			aria2PollInterval = 5 * time.Millisecond
			defer func() { aria2PollInterval = orig }()

			d, err := NewAria2Downloader(server.URL, "", "", "")
			if err != nil {
				t.Fatalf("构造失败: %v", err)
			}
			if tc.uaFn != nil {
				d = d.WithUserAgentFunc(tc.uaFn)
			}

			var buf bytes.Buffer
			if _, err := d.DownloadWithProgress(context.Background(), "http://example.com/a.bin", "", &buf, nil); err != nil {
				t.Fatalf("aria2 下载失败: %v", err)
			}

			ua, has := addURIOptions["user-agent"]
			if tc.wantHasOpt {
				if !has {
					t.Fatalf("期望下发 user-agent 选项，实际 options=%v", addURIOptions)
				}
				if ua != tc.wantUA {
					t.Fatalf("user-agent 不符: got %v want %s", ua, tc.wantUA)
				}
			} else if has {
				t.Fatalf("不应下发 user-agent，实际 %v", ua)
			}
		})
	}
}
