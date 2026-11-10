package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func readTestdata(t *testing.T, name string) string {
	t.Helper()

	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("读取测试数据 %s 失败: %v", name, err)
	}

	return string(body)
}

// TestParseExpandedAssets 用真实抓取的 HTML 片段校验资产解析
func TestParseExpandedAssets(t *testing.T) {
	html := readTestdata(t, "expanded_assets_sample.html")
	assets := ParseExpandedAssets(html)

	if len(assets) != 3 {
		t.Fatalf("期望解析出 3 个资产，实际 %d 个", len(assets))
	}

	checksum := assets[0]
	if checksum.Name != "gh_2.103.3_checksums.txt" {
		t.Fatalf("首个资产名不符: %s", checksum.Name)
	}
	if checksum.BrowserDownloadURL != "https://github.com/cli/cli/releases/download/v2.103.3/gh_2.103.3_checksums.txt" {
		t.Fatalf("下载地址不符: %s", checksum.BrowserDownloadURL)
	}
	if checksum.Digest != "f756e7859d90dbb5e85aae5194206c37da220bec9bac23d3d659078a8e766fdc" {
		t.Fatalf("digest 不符: %s", checksum.Digest)
	}
	if checksum.Size != 1966 {
		t.Fatalf("大小换算不符（1.92 KB 应约等于 1966 字节），实际 %d", checksum.Size)
	}
	if checksum.ContentType == "" {
		t.Fatalf("应按扩展名推断出 ContentType")
	}

	// 第二个资产用于确认分段解析不会串行（每个资产有自己的 digest 与大小）
	if assets[1].Name != "gh_2.103.3_linux_386.deb" {
		t.Fatalf("第二个资产名不符: %s", assets[1].Name)
	}
	if assets[1].Digest == checksum.Digest {
		t.Fatalf("不同资产不应共用同一个 digest")
	}
	if assets[1].Size <= checksum.Size {
		t.Fatalf("deb(13.5 MB) 应大于 checksums(1.92 KB)，实际 %d vs %d", assets[1].Size, checksum.Size)
	}
}

// TestParseExpandedAssetsSourceOnly 校验「只有源码包」的页面不会被误判为资产
func TestParseExpandedAssetsSourceOnly(t *testing.T) {
	html := readTestdata(t, "expanded_assets_source_only.html")
	assets := ParseExpandedAssets(html)

	if len(assets) != 0 {
		t.Fatalf("源码包链接（/archive/refs/tags/）不应被解析为资产，实际解析出 %d 个: %v", len(assets), assets)
	}
}

func TestSizeToBytes(t *testing.T) {
	cases := []struct {
		num   string
		unit  string
		want  int64
	}{
		{num: "512", unit: "B", want: 512},
		{num: "1.5", unit: "KB", want: 1536},
		{num: "14.4", unit: "MB", want: 15099494},
		{num: "2", unit: "GB", want: 2147483648},
		{num: "bad", unit: "MB", want: 0},
		{num: "1", unit: "XB", want: 0},
	}

	for _, tc := range cases {
		if got := sizeToBytes(tc.num, tc.unit); got != tc.want {
			t.Fatalf("sizeToBytes(%s %s) 期望 %d，实际 %d", tc.num, tc.unit, tc.want, got)
		}
	}
}

func TestEscapeTag(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "v1.0.0", want: "v1.0.0"},
		{in: "release/1.0", want: "release/1.0"},
		{in: "v1.0.0+build 1", want: "v1.0.0+build%201"},
	}

	for _, tc := range cases {
		if got := escapeTag(tc.in); got != tc.want {
			t.Fatalf("escapeTag(%q) 期望 %q，实际 %q", tc.in, tc.want, got)
		}
	}
}

func TestParseTagFromLink(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "https://github.com/cli/cli/releases/tag/v2.103.3", want: "v2.103.3"},
		{in: "https://github.com/o/r/releases/tag/release%2F1.0", want: "release/1.0"},
		{in: "", want: ""},
		{in: "https://github.com/o/r/releases", want: ""},
	}

	for _, tc := range cases {
		if got := parseTagFromLink(tc.in); got != tc.want {
			t.Fatalf("parseTagFromLink(%q) 期望 %q，实际 %q", tc.in, tc.want, got)
		}
	}
}

// TestLatestReleaseWithServer 通过本地假服务器验证完整抓取流程（atom → 资产页）
func TestLatestReleaseWithServer(t *testing.T) {
	atomSample := readTestdata(t, "releases_atom_sample.xml")
	assetSample := readTestdata(t, "expanded_assets_sample.html")

	var requestedPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases.atom"):
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = w.Write([]byte(atomSample))
		case strings.Contains(r.URL.Path, "/releases/expanded_assets/"):
			_, _ = w.Write([]byte(assetSample))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	fetcher := NewHTMLFetcher(server.Client(), server.URL, "")
	release, err := fetcher.LatestRelease(context.Background(), "cli", "cli")
	if err != nil {
		t.Fatalf("抓取最新 Release 失败: %v", err)
	}

	if release.TagName != "v2.103.3" {
		t.Fatalf("tag 不符: %s", release.TagName)
	}
	if release.Body == "" {
		t.Fatalf("atom 的 content 应被解析为 release body")
	}
	if len(release.Assets) != 3 {
		t.Fatalf("资产数不符: %d", len(release.Assets))
	}
	if len(requestedPaths) != 2 {
		t.Fatalf("应只请求 atom 与资产页共 2 次，实际 %d 次: %v", len(requestedPaths), requestedPaths)
	}
}

// TestLatestReleaseNotFound 仓库不存在/私有仓库时返回可识别的错误
func TestLatestReleaseNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher := NewHTMLFetcher(server.Client(), server.URL, "")
	_, err := fetcher.LatestRelease(context.Background(), "owner", "private-repo")
	if err == nil {
		t.Fatalf("期望返回错误")
	}
	if !isNotFoundError(err) {
		t.Fatalf("错误应可识别为不存在/私有仓库，实际: %v", err)
	}
}

// TestLatestReleaseThroughMirror 配置镜像时，网页端点请求应走镜像前缀
func TestLatestReleaseThroughMirror(t *testing.T) {
	atomSample := readTestdata(t, "releases_atom_sample.xml")

	var gotPaths []string
	mirrorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/releases.atom") {
			_, _ = w.Write([]byte(atomSample))
			return
		}
		// 资产页返回空列表，本用例只关心请求是否走了镜像
		_, _ = w.Write([]byte("<div></div>"))
	}))
	defer mirrorServer.Close()

	// base 固定为 github.com，镜像前缀式改写后请求会打到假镜像服务
	fetcher := NewHTMLFetcher(mirrorServer.Client(), defaultWebBase, mirrorServer.URL)
	release, err := fetcher.LatestRelease(context.Background(), "cli", "cli")
	if err != nil {
		t.Fatalf("通过镜像抓取失败: %v", err)
	}
	if release.TagName != "v2.103.3" {
		t.Fatalf("tag 不符: %s", release.TagName)
	}
	if len(gotPaths) == 0 {
		t.Fatalf("镜像服务未收到请求")
	}
	// 前缀式镜像：路径形如 /https://github.com/cli/cli/releases.atom
	if !strings.Contains(gotPaths[0], "https://github.com/cli/cli/releases.atom") {
		t.Fatalf("镜像请求路径不符合前缀式改写规则: %s", gotPaths[0])
	}
}

// hostRouter 按 host 分发请求，便于在不联网的情况下模拟「镜像 + 直连」两条链路
type hostRouter struct {
	handlers map[string]http.HandlerFunc
	calls    map[string]int
}

func (r *hostRouter) RoundTrip(req *http.Request) (*http.Response, error) {
	host := req.URL.Host
	r.calls[host]++
	handler, ok := r.handlers[host]
	if !ok {
		return nil, fmt.Errorf("未预期的请求 host: %s", host)
	}
	recorder := httptest.NewRecorder()
	handler(recorder, req)

	return recorder.Result(), nil
}

func (r *hostRouter) count(host string) int {
	return r.calls[host]
}

// TestMirrorWebFallbackWhenUnsupported 镜像拒绝网页请求时应快速回退直连，并记住该镜像不支持
func TestMirrorWebFallbackWhenUnsupported(t *testing.T) {
	resetMirrorWebBlocked(t)

	atomSample := readTestdata(t, "releases_atom_sample.xml")
	assetSample := readTestdata(t, "expanded_assets_sample.html")

	router := &hostRouter{
		calls: map[string]int{},
		handlers: map[string]http.HandlerFunc{
			// 镜像：明确拒绝网页（真实镜像 mirror.skon.top 的返回形态）
			"mirror.local": func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"Content type not allowed","message":"检测到网页类型，本服务不支持加速网页"}`))
			},
			// 直连 github.com：正常返回
			"github.com": func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/releases.atom") {
					_, _ = w.Write([]byte(atomSample))
					return
				}
				_, _ = w.Write([]byte(assetSample))
			},
		},
	}

	client := &http.Client{Transport: router}
	fetcher := NewHTMLFetcher(client, defaultWebBase, "https://mirror.local")

	release, err := fetcher.LatestRelease(context.Background(), "cli", "cli")
	if err != nil {
		t.Fatalf("镜像不支持网页时应回退直连成功，实际错误: %v", err)
	}
	if release.TagName != "v2.103.3" {
		t.Fatalf("tag 不符: %s", release.TagName)
	}
	if router.count("github.com") != 2 {
		t.Fatalf("直连应收到 atom + 资产页共 2 次请求，实际 %d 次", router.count("github.com"))
	}

	// 镜像已被屏蔽：再抓一次不应再打镜像
	mirrorCalls := router.count("mirror.local")
	if _, err := fetcher.LatestRelease(context.Background(), "cli", "cli"); err != nil {
		t.Fatalf("第二次抓取失败: %v", err)
	}
	if router.count("mirror.local") != mirrorCalls {
		t.Fatalf("镜像被拉黑后不应再被请求，实际从 %d 增加到 %d", mirrorCalls, router.count("mirror.local"))
	}
}

// TestMirrorWebUsedWhenSupported 镜像支持网页时应优先使用，不再回退直连
func TestMirrorWebUsedWhenSupported(t *testing.T) {
	resetMirrorWebBlocked(t)

	atomSample := readTestdata(t, "releases_atom_sample.xml")
	assetSample := readTestdata(t, "expanded_assets_sample.html")

	router := &hostRouter{
		calls: map[string]int{},
		handlers: map[string]http.HandlerFunc{
			"mirror.local": func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/releases.atom") {
					_, _ = w.Write([]byte(atomSample))
					return
				}
				_, _ = w.Write([]byte(assetSample))
			},
			"github.com": func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("镜像可用时不应回退直连: %s", r.URL.Path)
			},
		},
	}

	fetcher := NewHTMLFetcher(&http.Client{Transport: router}, defaultWebBase, "https://mirror.local")
	if _, err := fetcher.LatestRelease(context.Background(), "cli", "cli"); err != nil {
		t.Fatalf("通过镜像抓取失败: %v", err)
	}
	if router.count("github.com") != 0 {
		t.Fatalf("镜像可用时不应请求直连，实际 %d 次", router.count("github.com"))
	}
}

// TestMirrorWebBlockedExpiry 屏蔽条目过期后应自动恢复尝试
func TestMirrorWebBlockedExpiry(t *testing.T) {
	resetMirrorWebBlocked(t)

	if mirrorWebUnsupported("https://mirror.local") {
		t.Fatalf("初始状态不应被屏蔽")
	}

	markMirrorWebUnsupported("https://mirror.local")
	if !mirrorWebUnsupported("https://mirror.local") {
		t.Fatalf("标记后应被屏蔽")
	}

	// 手工写入过期时间，验证自动解除
	mirrorWebBlocked.Store("https://mirror.local", time.Now().Add(-time.Minute))
	if mirrorWebUnsupported("https://mirror.local") {
		t.Fatalf("过期后应自动解除屏蔽")
	}
}

// resetMirrorWebBlocked 清理包级屏蔽表，避免用例之间互相影响
func resetMirrorWebBlocked(t *testing.T) {
	t.Helper()

	mirrorWebBlocked.Range(func(key, _ any) bool {
		mirrorWebBlocked.Delete(key)
		return true
	})
	t.Cleanup(func() {
		mirrorWebBlocked.Range(func(key, _ any) bool {
			mirrorWebBlocked.Delete(key)
			return true
		})
	})
}
