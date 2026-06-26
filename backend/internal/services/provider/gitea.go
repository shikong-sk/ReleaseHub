package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GiteaProvider 适配 Gitea/Forgejo Release API
// Gitea 和 Forgejo 的 Release API 兼容，共用此 Provider
type GiteaProvider struct {
	baseURL    string
	httpClient *http.Client
}

func NewGiteaProvider(apiBaseURL string) *GiteaProvider {
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = "https://gitea.com/api/v1"
	}
	return &GiteaProvider{
		baseURL: strings.TrimRight(apiBaseURL, "/"),
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (g *GiteaProvider) Name() string {
	return "gitea"
}

func (g *GiteaProvider) GetLatestRelease(ctx context.Context, owner, repo, token string) (*ProviderRelease, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases/latest", g.baseURL, url.PathEscape(owner), url.PathEscape(repo))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ReleaseHub")
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Gitea Release 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Gitea Release 不存在或无权限访问")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Gitea API 返回异常状态: HTTP %d", resp.StatusCode)
	}

	var release giteaRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 Gitea Release 响应失败: %w", err)
	}

	result := toGiteaProviderRelease(release)
	return &result, nil
}

func (g *GiteaProvider) ListAllReleases(ctx context.Context, owner, repo, token string, maxPages int) ([]ProviderRelease, error) {
	if maxPages < 1 {
		maxPages = 10
	}

	var allReleases []ProviderRelease
	page := 1
	perPage := 30

	for page <= maxPages {
		endpoint := fmt.Sprintf("%s/repos/%s/%s/releases?page=%d&limit=%d",
			g.baseURL, url.PathEscape(owner), url.PathEscape(repo), page, perPage)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "ReleaseHub")
		if token != "" {
			req.Header.Set("Authorization", "token "+token)
		}

		resp, err := g.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("请求 Gitea Releases 失败: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("Gitea API 返回异常状态: HTTP %d", resp.StatusCode)
		}

		var releases []giteaRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("解析 Gitea Releases 响应失败: %w", err)
		}
		resp.Body.Close()

		if len(releases) == 0 {
			break
		}

		for _, r := range releases {
			allReleases = append(allReleases, toGiteaProviderRelease(r))
		}

		if len(releases) < perPage {
			break
		}
		page++
	}

	return allReleases, nil
}

func (g *GiteaProvider) GetAssetDownloadURL(ctx context.Context, owner, repo string, asset ProviderAsset, token string) (string, error) {
	if asset.BrowserDownloadURL != "" {
		return asset.BrowserDownloadURL, nil
	}
	if asset.URL != "" {
		return asset.URL, nil
	}
	return "", nil
}

// Gitea Release JSON 结构（与 Forgejo 兼容）
type giteaRelease struct {
	ID          int64         `json:"id"`
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Body        string        `json:"body"`
	HTMLURL     string        `json:"html_url"`
	URL         string        `json:"url"`
	PublishedAt time.Time     `json:"published_at"`
	Assets      []giteaAsset  `json:"assets"`
}

type giteaAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func toGiteaProviderRelease(r giteaRelease) ProviderRelease {
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
	return ProviderRelease{
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

// 确保引用 url
var _ = url.PathEscape
