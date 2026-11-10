package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	githubsvc "releasehub/backend/internal/services/github"
)

func readTestdata(t *testing.T, name string) string {
	t.Helper()

	body, err := os.ReadFile("../../services/github/testdata/" + name)
	if err != nil {
		t.Fatalf("读取测试数据 %s 失败: %v", name, err)
	}

	return string(body)
}

func TestNormalizeDigest(t *testing.T) {
	cases := []struct{ in, want string }{
		{in: "sha256:f756e785", want: "f756e785"},
		{in: "SHA256:F756E785", want: "f756e785"},
		{in: " f756e785 ", want: "f756e785"},
		{in: "", want: ""},
	}

	for _, tc := range cases {
		if got := normalizeDigest(tc.in); got != tc.want {
			t.Fatalf("normalizeDigest(%q) 期望 %q，实际 %q", tc.in, tc.want, got)
		}
	}
}

// TestGitHubHTMLProviderLatestRelease 校验 HTML Provider 完整链路：
// atom 取最新 tag → 资产页取资产 → 转换为 ProviderRelease
func TestGitHubHTMLProviderLatestRelease(t *testing.T) {
	atomSample := readTestdata(t, "releases_atom_sample.xml")
	assetSample := readTestdata(t, "expanded_assets_sample.html")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases.atom"):
			_, _ = w.Write([]byte(atomSample))
		case strings.Contains(r.URL.Path, "/releases/expanded_assets/"):
			_, _ = w.Write([]byte(assetSample))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// 无代理的 http.Client，base 指向假服务器
	fetcher := githubsvc.NewHTMLFetcher(server.Client(), server.URL, "")
	htmlProvider := NewGitHubHTMLProvider(fetcher)

	if htmlProvider.Name() != "github-html" {
		t.Fatalf("Provider 名称不符: %s", htmlProvider.Name())
	}

	release, err := htmlProvider.GetLatestRelease(context.Background(), "cli", "cli", "")
	if err != nil {
		t.Fatalf("获取最新 Release 失败: %v", err)
	}
	if release.TagName != "v2.103.3" {
		t.Fatalf("tag 不符: %s", release.TagName)
	}
	if len(release.Assets) != 3 {
		t.Fatalf("资产数不符: %d", len(release.Assets))
	}
	if release.Assets[0].Digest == "" {
		t.Fatalf("资产应带有 digest")
	}
	// digest 应为不带 sha256: 前缀的十六进制
	if strings.HasPrefix(release.Assets[0].Digest, "sha256:") {
		t.Fatalf("digest 不应保留 sha256: 前缀: %s", release.Assets[0].Digest)
	}
}

// TestGitHubHTMLProviderListAllUnsupported 全量同步必须走 API
func TestGitHubHTMLProviderListAllUnsupported(t *testing.T) {
	htmlProvider := NewGitHubHTMLProvider(githubsvc.NewHTMLFetcher(nil, "", ""))

	_, err := htmlProvider.ListAllReleases(context.Background(), "cli", "cli", "", 10)
	if !errors.Is(err, githubsvc.ErrHTMLFallbackUnsupported) {
		t.Fatalf("应返回 ErrHTMLFallbackUnsupported，实际: %v", err)
	}
}

// TestGitHubHTMLProviderAssetDownloadURL 资产下载地址回退逻辑
func TestGitHubHTMLProviderAssetDownloadURL(t *testing.T) {
	htmlProvider := NewGitHubHTMLProvider(githubsvc.NewHTMLFetcher(nil, "", ""))

	url, err := htmlProvider.GetAssetDownloadURL(context.Background(), "o", "r",
		ProviderAsset{Name: "a.zip", BrowserDownloadURL: "https://github.com/o/r/releases/download/v1/a.zip"}, "")
	if err != nil || !strings.Contains(url, "a.zip") {
		t.Fatalf("应返回 BrowserDownloadURL，实际 %q, err=%v", url, err)
	}

	if _, err := htmlProvider.GetAssetDownloadURL(context.Background(), "o", "r", ProviderAsset{Name: "a.zip"}, ""); err == nil {
		t.Fatalf("缺少下载地址时应返回错误")
	}
}
