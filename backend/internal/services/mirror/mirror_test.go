package mirror

import (
	"strings"
	"testing"
)

func TestNormalizeBase(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "带 scheme", input: "https://ghproxy.example.com", want: "https://ghproxy.example.com"},
		{name: "缺 scheme 补 https", input: "ghproxy.example.com/", want: "https://ghproxy.example.com/"},
		{name: "保留路径", input: "https://m.example.com/github", want: "https://m.example.com/github"},
		{name: "http 允许", input: "http://127.0.0.1:8080", want: "http://127.0.0.1:8080"},
		{name: "空串报错", input: "   ", wantErr: true},
		{name: "ftp 不支持", input: "ftp://m.example.com", wantErr: true},
		{name: "缺少主机", input: "https://", wantErr: true},
		{name: "占位符不能出现在主机", input: "https://{url}/x", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeBase(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("期望报错，实际得到 %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("期望成功，实际报错: %v", err)
			}
			if got.String() != tc.want {
				t.Fatalf("期望 %s，实际 %s", tc.want, got.String())
			}
		})
	}
}

func TestValidateBase(t *testing.T) {
	if err := ValidateBase(""); err != nil {
		t.Fatalf("空镜像地址应视为合法（不启用）: %v", err)
	}
	if err := ValidateBase("ghproxy.example.com"); err != nil {
		t.Fatalf("缺 scheme 应被补全: %v", err)
	}
	if err := ValidateBase("ftp://bad"); err == nil {
		t.Fatalf("非法 scheme 应报错")
	}
}

func TestRewriteReleaseAsset(t *testing.T) {
	const assetURL = "https://github.com/shikong-sk/releasehub/releases/download/v1.0.8/releasehub_linux_amd64.tar.gz"

	cases := []struct {
		name        string
		base        string
		raw         string
		wantURL     string
		wantApplied bool
	}{
		{
			name:        "前缀式改写",
			base:        "https://ghproxy.example.com",
			raw:         assetURL,
			wantURL:     "https://ghproxy.example.com/" + assetURL,
			wantApplied: true,
		},
		{
			name:        "前缀式改写（base 带尾斜杠）",
			base:        "https://ghproxy.example.com/",
			raw:         assetURL,
			wantURL:     "https://ghproxy.example.com/" + assetURL,
			wantApplied: true,
		},
		{
			name:        "模板式改写",
			base:        "https://ghproxy.example.com/{url}",
			raw:         assetURL,
			wantURL:     "https://ghproxy.example.com/" + assetURL,
			wantApplied: true,
		},
		{
			name:        "未配置镜像透传",
			base:        "",
			raw:         assetURL,
			wantURL:     assetURL,
			wantApplied: false,
		},
		{
			name:        "非 GitHub 主机不改写",
			base:        "https://ghproxy.example.com",
			raw:         "https://gitlab.com/a/b/-/releases/v1/downloads/x.tar.gz",
			wantURL:     "https://gitlab.com/a/b/-/releases/v1/downloads/x.tar.gz",
			wantApplied: false,
		},
		{
			name:        "objects.githubusercontent.com 代理签名直链不改写",
			base:        "https://ghproxy.example.com",
			raw:         "https://objects.githubusercontent.com/github-production-release-asset/x?X-Amz-Signature=abc",
			wantURL:     "https://objects.githubusercontent.com/github-production-release-asset/x?X-Amz-Signature=abc",
			wantApplied: false,
		},
		{
			name:        "raw.githubusercontent.com 不改写",
			base:        "https://ghproxy.example.com",
			raw:         "https://raw.githubusercontent.com/a/b/main/README.md",
			wantURL:     "https://raw.githubusercontent.com/a/b/main/README.md",
			wantApplied: false,
		},
		{
			name:        "已是镜像地址不重复改写",
			base:        "https://ghproxy.example.com",
			raw:         "https://ghproxy.example.com/" + assetURL,
			wantURL:     "https://ghproxy.example.com/" + assetURL,
			wantApplied: false,
		},
		{
			name:        "空下载地址透传",
			base:        "https://ghproxy.example.com",
			raw:         "  ",
			wantURL:     "  ",
			wantApplied: false,
		},
		{
			name:        "非法 base 透传原地址",
			base:        "ftp://ghproxy.example.com",
			raw:         assetURL,
			wantURL:     assetURL,
			wantApplied: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RewriteReleaseAsset(tc.base, tc.raw)
			if got.Applied != tc.wantApplied {
				t.Fatalf("Applied 期望 %v，实际 %v（原因: %s）", tc.wantApplied, got.Applied, got.Reason)
			}
			if got.URL != tc.wantURL {
				t.Fatalf("URL 期望 %s，实际 %s", tc.wantURL, got.URL)
			}
			if !got.Applied && strings.TrimSpace(tc.base) != "" && got.Reason == "" {
				t.Fatalf("未改写时应给出原因，便于排查")
			}
		})
	}
}
