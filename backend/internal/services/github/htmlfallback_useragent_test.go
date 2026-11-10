package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHTMLFetcherUserAgent 校验网页抓取的 UA 可被注入（镜像站拦截非浏览器 UA 时使用浏览器 UA）
func TestHTMLFetcherUserAgent(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*HTMLFetcher) *HTMLFetcher
		want  string
	}{
		{
			name:  "未注入使用内置 htmlUserAgent",
			setup: func(f *HTMLFetcher) *HTMLFetcher { return f },
			want:  "ReleaseHub-HTMLFallback",
		},
		{
			name:  "注入自定义 UA",
			setup: func(f *HTMLFetcher) *HTMLFetcher { return f.WithUserAgentFunc(func() string { return "Mozilla/5.0 Fetch" }) },
			want:  "Mozilla/5.0 Fetch",
		},
		{
			name:  "函数返回空回退内置 UA",
			setup: func(f *HTMLFetcher) *HTMLFetcher { return f.WithUserAgentFunc(func() string { return "" }) },
			want:  "ReleaseHub-HTMLFallback",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			atomSample := readTestdata(t, "releases_atom_sample.xml")
			var gotUA string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotUA = r.Header.Get("User-Agent")
				_, _ = w.Write([]byte(atomSample))
			}))
			defer server.Close()

			f := tc.setup(NewHTMLFetcher(server.Client(), server.URL, ""))
			if _, err := f.LatestRelease(context.Background(), "cli", "cli"); err != nil {
				t.Fatalf("抓取失败: %v", err)
			}
			if gotUA != tc.want {
				t.Fatalf("User-Agent 不符\n期望: %s\n实际: %s", tc.want, gotUA)
			}
		})
	}
}
