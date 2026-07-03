package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// GitLabProvider 适配 GitLab Release API
type GitLabProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewGitLabProvider(apiBaseURL string) *GitLabProvider {
	return NewGitLabProviderWithTransport(apiBaseURL, nil)
}

// NewGitLabProviderWithTransport 创建带可选代理 transport 的 GitLabProvider
func NewGitLabProviderWithTransport(apiBaseURL string, transport *http.Transport) *GitLabProvider {
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = "https://gitlab.com/api/v4"
	}
	client := &http.Client{Timeout: 20 * time.Second}
	if transport != nil {
		client.Transport = transport
	}
	return &GitLabProvider{
		baseURL:    strings.TrimRight(apiBaseURL, "/"),
		httpClient: client,
	}
}

func (g *GitLabProvider) Name() string {
	return "gitlab"
}

func (g *GitLabProvider) GetLatestRelease(ctx context.Context, owner, repo, token string) (*ProviderRelease, error) {
	releases, err := g.ListAllReleases(ctx, owner, repo, token, 1)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("GitLab Release 不存在")
	}
	return &releases[0], nil
}

func (g *GitLabProvider) GetReleaseByTag(ctx context.Context, owner, repo, tag, token string) (*ProviderRelease, error) {
	projectPath := url.PathEscape(owner + "/" + repo)
	endpoint := fmt.Sprintf("%s/projects/%s/releases/%s", g.baseURL, projectPath, url.PathEscape(tag))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ReleaseHub")
	if token != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitLab Release (tag: %s) 失败: %w", tag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Release %s 不存在", tag)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitLab API 返回异常状态: HTTP %d", resp.StatusCode)
	}

	var glRelease gitLabRelease
	if err := json.NewDecoder(resp.Body).Decode(&glRelease); err != nil {
		return nil, fmt.Errorf("解析 GitLab Release 响应失败: %w", err)
	}

	result := toGitLabProviderRelease(glRelease)
	return &result, nil
}

func (g *GitLabProvider) ListAllReleases(ctx context.Context, owner, repo, token string, maxPages int) ([]ProviderRelease, error) {
	if maxPages < 1 {
		maxPages = 10
	}
	projectPath := url.PathEscape(owner + "/" + repo)

	var allReleases []ProviderRelease
	page := 1
	perPage := 20

	for page <= maxPages {
		endpoint := fmt.Sprintf("%s/projects/%s/releases?page=%d&per_page=%d", g.baseURL, projectPath, page, perPage)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "ReleaseHub")
		if token != "" {
			req.Header.Set("PRIVATE-TOKEN", token)
		}

		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求 GitLab Releases 失败: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("GitLab API 返回异常状态: HTTP %d", resp.StatusCode)
		}

		var glReleases []gitLabRelease
		if err := json.NewDecoder(resp.Body).Decode(&glReleases); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("解析 GitLab Releases 响应失败: %w", err)
		}
		resp.Body.Close()

		if len(glReleases) == 0 {
			break
		}

		for _, r := range glReleases {
			allReleases = append(allReleases, toGitLabProviderRelease(r))
		}

		if len(glReleases) < perPage {
			break
		}
		page++
	}

	return allReleases, nil
}

func (g *GitLabProvider) GetAssetDownloadURL(ctx context.Context, owner, repo string, asset ProviderAsset, token string) (string, error) {
	if asset.BrowserDownloadURL != "" {
		return asset.BrowserDownloadURL, nil
	}
	if asset.URL != "" {
		return asset.URL, nil
	}
	return "", nil
}

// GitLab Release JSON 结构
type gitLabRelease struct {
	TagName     string              `json:"tag_name"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	HTMLURL     string              `json:"_links.web"`
	CreatedAt   time.Time           `json:"created_at"`
	ReleasedAt  time.Time           `json:"released_at"`
	Assets      gitLabReleaseAssets `json:"assets"`
}

type gitLabReleaseAssets struct {
	Links []gitLabAssetLink `json:"links"`
}

type gitLabAssetLink struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"link_type"`
}

func toGitLabProviderRelease(r gitLabRelease) ProviderRelease {
	assets := make([]ProviderAsset, 0, len(r.Assets.Links))
	for _, link := range r.Assets.Links {
		assets = append(assets, ProviderAsset{
			ID:                 link.ID,
			Name:               link.Name,
			URL:                link.URL,
			BrowserDownloadURL: link.URL,
			ContentType:        "application/octet-stream",
		})
	}

	publishedAt := r.ReleasedAt
	if publishedAt.IsZero() {
		publishedAt = r.CreatedAt
	}

	return ProviderRelease{
		TagName:     r.TagName,
		Name:        r.Name,
		Body:        r.Description,
		HTMLURL:     r.HTMLURL,
		PublishedAt: publishedAt,
		Assets:      assets,
	}
}

// 确保引用 path
var _ = path.Join
