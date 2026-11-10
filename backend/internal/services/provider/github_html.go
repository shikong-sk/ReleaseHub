package provider

import (
	"context"
	"fmt"

	githubsvc "releasehub/backend/internal/services/github"
)

// GitHubHTMLProvider 使用 GitHub 网页端点（atom + expanded_assets）实现 ReleaseProvider。
//
// 用途：未配置 Token 的公开仓库可以完全不消耗 API 配额（未认证配额仅 60 次/小时），
// 同时能救回 /releases/latest 返回 404 的仓库（如 python/cpython）。
//
// 限制：
//   - 私有仓库无法访问（网页端点不接受 PAT）
//   - atom 只返回最近 10 条，无法分页，因此 ListAllReleases 不可用
//   - 拿不到 prerelease/draft 标记与 asset 数字 ID（当前代码未使用这两个字段）
type GitHubHTMLProvider struct {
	fetcher *githubsvc.HTMLFetcher
}

// NewGitHubHTMLProvider 创建 HTML 降级 Provider。
func NewGitHubHTMLProvider(fetcher *githubsvc.HTMLFetcher) *GitHubHTMLProvider {
	return &GitHubHTMLProvider{fetcher: fetcher}
}

func (g *GitHubHTMLProvider) Name() string {
	return "github-html"
}

func (g *GitHubHTMLProvider) GetLatestRelease(ctx context.Context, owner, repo, token string) (*ProviderRelease, error) {
	// token 不参与 HTML 抓取（网页端点无法使用 PAT），保留参数以匹配接口
	release, err := g.fetcher.LatestRelease(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	return toProviderRelease(release), nil
}

func (g *GitHubHTMLProvider) GetReleaseByTag(ctx context.Context, owner, repo, tag, token string) (*ProviderRelease, error) {
	release, err := g.fetcher.ReleaseByTag(ctx, owner, repo, tag)
	if err != nil {
		return nil, err
	}

	return toProviderRelease(release), nil
}

func (g *GitHubHTMLProvider) ListAllReleases(ctx context.Context, owner, repo, token string, maxPages int) ([]ProviderRelease, error) {
	// atom 无分页能力，全量同步必须走 API，这里明确报错而不是返回残缺数据
	return nil, githubsvc.ErrHTMLFallbackUnsupported
}

func (g *GitHubHTMLProvider) GetAssetDownloadURL(ctx context.Context, owner, repo string, asset ProviderAsset, token string) (string, error) {
	if asset.BrowserDownloadURL != "" {
		return asset.BrowserDownloadURL, nil
	}
	if asset.URL != "" {
		return asset.URL, nil
	}

	return "", fmt.Errorf("资产 %s 缺少下载地址", asset.Name)
}
