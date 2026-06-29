package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

type Release struct {
	ID          int64     `json:"id"`
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

type Asset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func NewClient(apiBaseURL string) (*Client, error) {
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = "https://api.github.com"
	}

	baseURL, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("GitHub API 地址无效: %w", err)
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}, nil
}

// NewClientWithTransport 创建使用指定 Transport 的 GitHub 客户端（用于代理支持）
func NewClientWithTransport(apiBaseURL string, transport *http.Transport) (*Client, error) {
	if strings.TrimSpace(apiBaseURL) == "" {
		apiBaseURL = "https://api.github.com"
	}

	baseURL, err := url.Parse(apiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("GitHub API 地址无效: %w", err)
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   20 * time.Second,
			Transport: transport,
		},
	}, nil
}

// doWithRetry 执行 HTTP 请求，遇到 429/5xx 自动退避重试（最多 3 次）
func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	const maxAttempts = 3
	baseDelay := 2 * time.Second

	for attempt := 0; ; attempt++ {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		// 需要重试的状态码
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < maxAttempts-1 {
			resp.Body.Close()

			delay := baseDelay * time.Duration(1<<attempt) // 2s, 4s, 8s
			const maxDelay = 60 * time.Second
			// 429 时优先用 Retry-After 头，但限制上界防止长时间阻塞
			if resp.StatusCode == http.StatusTooManyRequests {
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
						raDelay := time.Duration(secs) * time.Second
						if raDelay < maxDelay {
							delay = raDelay
						} else {
							delay = maxDelay
						}
					}
				}
			}
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delay):
			}
			continue
		}

		return resp, nil
	}
}

// GetLatestRelease 获取仓库最新的 Release
func (c *Client) GetLatestRelease(ctx context.Context, owner string, repo string, token string) (*Release, error) {
	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, "repos", owner, repo, "releases", "latest")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ReleaseHub")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub Release 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("GitHub Release 不存在或无权限访问")
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("GitHub API 限流或拒绝访问: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub API 返回异常状态: HTTP %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 响应失败: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, fmt.Errorf("GitHub Release 响应缺少 tag_name")
	}

	return &release, nil
}

// ListReleases 分页列出仓库的所有 Release（按发布时间降序）
func (c *Client) ListReleases(ctx context.Context, owner string, repo string, token string, page int, perPage int) ([]Release, error) {
	if perPage < 1 {
		perPage = 30
	}
	if perPage > 100 {
		perPage = 100
	}
	if page < 1 {
		page = 1
	}

	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, "repos", owner, repo, "releases")
	q := endpoint.Query()
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("per_page", fmt.Sprintf("%d", perPage))
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ReleaseHub")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub Releases 列表失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("仓库不存在或无权限访问")
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("GitHub API 限流或拒绝访问: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub API 返回异常状态: HTTP %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("解析 GitHub Releases 列表响应失败: %w", err)
	}

	return releases, nil
}

// ListAllReleases 拉取仓库的所有 Release（自动分页，最多 maxPages 页）
func (c *Client) ListAllReleases(ctx context.Context, owner string, repo string, token string, maxPages int) ([]Release, error) {
	if maxPages < 1 {
		maxPages = 10
	}

	var allReleases []Release
	page := 1
	perPage := 100

	for page <= maxPages {
		releases, err := c.ListReleases(ctx, owner, repo, token, page, perPage)
		if err != nil {
			return allReleases, err
		}
		if len(releases) == 0 {
			break
		}
		allReleases = append(allReleases, releases...)
		if len(releases) < perPage {
			break
		}
		page++
	}

	return allReleases, nil
}

// GetReleaseByTag 根据 tag 获取指定版本的 Release
// GitHub API: GET /repos/{owner}/{repo}/releases/tags/{tag}
func (c *Client) GetReleaseByTag(ctx context.Context, owner string, repo string, tag string, token string) (*Release, error) {
	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, "repos", owner, repo, "releases", "tags", tag)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ReleaseHub")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub Release (tag: %s) 失败: %w", tag, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("Release %s 不存在或无权限访问", tag)
	}
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("GitHub API 限流或拒绝访问: HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub API 返回异常状态: HTTP %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 响应失败: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return nil, fmt.Errorf("GitHub Release 响应缺少 tag_name")
	}

	return &release, nil
}
