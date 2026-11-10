package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetReleaseByTagEscapesTag 校验 tag 中的 "/" 不会被当成路径分隔符。
//
// 真实场景：仓库使用 release/1.0 这类含斜杠的 tag，GitHub API 要求编码为 %2F，
// 直接用 path.Join 拼接会得到 /releases/tags/release/1.0 从而 404。
func TestGetReleaseByTagEscapesTag(t *testing.T) {
	cases := []struct {
		name         string
		tag          string
		wantRawPath  string
		wantDecoded  string
	}{
		{
			name:        "普通 tag",
			tag:         "v1.0.0",
			wantRawPath: "/repos/cli/cli/releases/tags/v1.0.0",
			wantDecoded: "/repos/cli/cli/releases/tags/v1.0.0",
		},
		{
			name:        "含斜杠的 tag",
			tag:         "release/1.0",
			wantRawPath: "/repos/cli/cli/releases/tags/release%2F1.0",
			wantDecoded: "/repos/cli/cli/releases/tags/release/1.0",
		},
		{
			name:        "含空格与加号的 tag",
			tag:         "v1.0 build+1",
			wantRawPath: "/repos/cli/cli/releases/tags/v1.0%20build+1",
			wantDecoded: "/repos/cli/cli/releases/tags/v1.0 build+1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotURI, gotRawPath, gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotURI = r.RequestURI
				gotRawPath = r.URL.RawPath
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(Release{ID: 1, TagName: tc.tag, Name: tc.tag})
			}))
			defer server.Close()

			client, err := NewClient(server.URL)
			if err != nil {
				t.Fatalf("创建客户端失败: %v", err)
			}

			release, err := client.GetReleaseByTag(context.Background(), "cli", "cli", tc.tag, "")
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}
			if release.TagName != tc.tag {
				t.Fatalf("tag 不符: %s", release.TagName)
			}
			if gotURI != tc.wantRawPath {
				t.Fatalf("请求路径不符\n期望: %s\n实际: %s", tc.wantRawPath, gotURI)
			}
			if gotPath != tc.wantDecoded {
				t.Fatalf("服务端解码后的路径不符\n期望: %s\n实际: %s", tc.wantDecoded, gotPath)
			}
			// RawPath 只有在与 Path 解码结果一致时才非空，用于确认 %2F 未被破坏
			if tc.tag == "release/1.0" && gotRawPath != tc.wantRawPath {
				t.Fatalf("RawPath 不符\n期望: %s\n实际: %s", tc.wantRawPath, gotRawPath)
			}
		})
	}
}

// TestGetLatestReleasePath 确认普通请求路径未受 tag 转义改动影响
func TestGetLatestReleasePath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":1,"tag_name":"v1.0.0","name":"v1.0.0"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("创建客户端失败: %v", err)
	}
	if _, err := client.GetLatestRelease(context.Background(), "cli", "cli", ""); err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if gotPath != "/repos/cli/cli/releases/latest" {
		t.Fatalf("路径不符: %s", gotPath)
	}
}
