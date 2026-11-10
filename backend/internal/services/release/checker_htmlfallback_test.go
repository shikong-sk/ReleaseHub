package release

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"releasehub/backend/internal/models"
	githubsvc "releasehub/backend/internal/services/github"
	"releasehub/backend/internal/services/provider"
)

// TestHTMLFallbackAvailable 校验网页抓取的可用条件：仅 GitHub 且未配置 Token
func TestHTMLFallbackAvailable(t *testing.T) {
	service := &CheckService{}

	cases := []struct {
		name       string
		repository models.Repository
		token      string
		want       bool
	}{
		{
			name:       "GitHub 无 Token 具备网页抓取能力",
			repository: models.Repository{Provider: "github"},
			token:      "",
			want:       true,
		},
		{
			name:       "Provider 为空视同 GitHub",

			repository: models.Repository{Provider: ""},
			token:      "",
			want:       true,
		},
		{
			name:       "有 Token 时走 API",
			repository: models.Repository{Provider: "github"},
			token:      "ghp_xxx",
			want:       false,
		},
		{
			name:       "Token 仅空白字符视同无 Token",
			repository: models.Repository{Provider: "github"},
			token:      "   ",
			want:       true,
		},
		{
			name:       "非 GitHub Provider 不启用",
			repository: models.Repository{Provider: "gitea"},
			token:      "",
			want:       false,
		},
	}

	for _, tc := range cases {
		if got := service.htmlFallbackAvailable(tc.repository, tc.token); got != tc.want {
			t.Fatalf("%s: 期望 %v，实际 %v", tc.name, tc.want, got)
		}
	}
}

// TestIsHTMLFallbackCause 只有限流/不存在类错误才降级，网络等错误直接透出
func TestIsHTMLFallbackCause(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "API 限流", err: fmt.Errorf("查询失败: %w", githubsvc.ErrRateLimited), want: true},
		{name: "Release 不存在", err: githubsvc.ErrReleaseNotFound, want: true},
		{name: "仓库不存在", err: githubsvc.ErrRepoNotFound, want: true},
		{name: "未知错误", err: errors.New("connection reset by peer"), want: false},
		{name: "空错误", err: nil, want: false},
	}

	for _, tc := range cases {
		if got := isHTMLFallbackCause(tc.err); got != tc.want {
			t.Fatalf("%s: 期望 %v，实际 %v", tc.name, tc.want, got)
		}
	}
}

// TestExpectedSHA256 资产 digest 优先，缺失时回退 release body 中的校验和
func TestExpectedSHA256(t *testing.T) {
	bodyMap := map[string]string{"a.zip": "from-body"}

	// digest 优先
	if got := expectedSHA256(provider.ProviderAsset{Name: "a.zip", Digest: "from-digest"}, bodyMap); got != "from-digest" {
		t.Fatalf("应优先使用 digest，实际 %q", got)
	}
	// digest 缺失时回退
	if got := expectedSHA256(provider.ProviderAsset{Name: "a.zip"}, bodyMap); got != "from-body" {
		t.Fatalf("应回退到 body 校验和，实际 %q", got)
	}
	// 都没有则为空
	if got := expectedSHA256(provider.ProviderAsset{Name: "b.zip"}, bodyMap); got != "" {
		t.Fatalf("应返回空字符串，实际 %q", got)
	}
	// digest 只有空白也视为缺失
	if got := expectedSHA256(provider.ProviderAsset{Name: "a.zip", Digest: "  "}, bodyMap); got != "from-body" {
		t.Fatalf("空白 digest 应回退到 body 校验和，实际 %q", got)
	}
}

// TestHTMLFallbackCooldown 校验限流后进入冷却期、过期后自动恢复
func TestHTMLFallbackCooldown(t *testing.T) {
	service := &CheckService{}

	if service.inHTMLFallbackCooldown(7) {
		t.Fatalf("未标记时不应处于冷却期")
	}

	service.markHTMLFallback(7)
	if !service.inHTMLFallbackCooldown(7) {
		t.Fatalf("标记后应处于冷却期")
	}
	// 其他仓库不受影响
	if service.inHTMLFallbackCooldown(8) {
		t.Fatalf("冷却期应按仓库隔离")
	}

	// 手工写入一个已过期的时间，验证会自动清除
	service.htmlFallbackUntil.Store(uint(9), time.Now().Add(-time.Minute))
	if service.inHTMLFallbackCooldown(9) {
		t.Fatalf("过期后应自动退出冷却期")
	}
	if _, ok := service.htmlFallbackUntil.Load(uint(9)); ok {
		t.Fatalf("过期条目应被清除")
	}

	// 仓库ID为0（尚未入库）时不应记录状态
	service.markHTMLFallback(0)
	if _, ok := service.htmlFallbackUntil.Load(uint(0)); ok {
		t.Fatalf("仓库ID为0时不应写入冷却状态")
	}
}
