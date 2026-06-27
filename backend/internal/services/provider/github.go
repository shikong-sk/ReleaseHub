package provider

import (
	"context"

	githubsvc "releasehub/backend/internal/services/github"
)

// GitHubProvider 将 GitHub Client 适配为 ReleaseProvider 接口
type GitHubProvider struct {
	client *githubsvc.Client
}

func NewGitHubProvider(client *githubsvc.Client) *GitHubProvider {
	return &GitHubProvider{client: client}
}

func (g *GitHubProvider) Name() string {
	return "github"
}

func (g *GitHubProvider) GetLatestRelease(ctx context.Context, owner, repo, token string) (*ProviderRelease, error) {
	release, err := g.client.GetLatestRelease(ctx, owner, repo, token)
	if err != nil {
		return nil, err
	}
	return toProviderRelease(release), nil
}

func (g *GitHubProvider) GetReleaseByTag(ctx context.Context, owner, repo, tag, token string) (*ProviderRelease, error) {
	release, err := g.client.GetReleaseByTag(ctx, owner, repo, tag, token)
	if err != nil {
		return nil, err
	}
	return toProviderRelease(release), nil
}

func (g *GitHubProvider) ListAllReleases(ctx context.Context, owner, repo, token string, maxPages int) ([]ProviderRelease, error) {
	releases, err := g.client.ListAllReleases(ctx, owner, repo, token, maxPages)
	if err != nil {
		return nil, err
	}
	result := make([]ProviderRelease, 0, len(releases))
	for _, r := range releases {
		result = append(result, *toProviderRelease(&r))
	}
	return result, nil
}

func (g *GitHubProvider) GetAssetDownloadURL(ctx context.Context, owner, repo string, asset ProviderAsset, token string) (string, error) {
	if asset.BrowserDownloadURL != "" {
		return asset.BrowserDownloadURL, nil
	}
	if asset.URL != "" {
		return asset.URL, nil
	}
	return "", nil
}

func toProviderRelease(r *githubsvc.Release) *ProviderRelease {
	assets := make([]ProviderAsset, 0, len(r.Assets))
	for _, a := range r.Assets {
		assets = append(assets, ProviderAsset{
			ID:                 a.ID,
			Name:               a.Name,
			Size:               a.Size,
			ContentType:        a.ContentType,
			URL:                a.URL,
			BrowserDownloadURL: a.BrowserDownloadURL,
		})
	}
	return &ProviderRelease{
		ID:          r.ID,
		TagName:     r.TagName,
		Name:        r.Name,
		Body:        r.Body,
		HTMLURL:     r.HTMLURL,
		APIURL:      r.URL,
		PublishedAt: r.PublishedAt,
		Assets:      assets,
	}
}

// newGitHubClient 创建 GitHub Client（供 registry 使用）
func newGitHubClient(apiBaseURL string) (*githubsvc.Client, error) {
	return githubsvc.NewClient(apiBaseURL)
}
