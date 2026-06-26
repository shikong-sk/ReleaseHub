package provider

import (
	"context"
	"time"
)

// ReleaseProvider Release 提供者统一接口
// GitHub / GitLab / Gitea / Forgejo 均实现此接口
type ReleaseProvider interface {
	// GetLatestRelease 获取仓库最新 Release
	GetLatestRelease(ctx context.Context, owner, repo, token string) (*ProviderRelease, error)
	// ListAllReleases 拉取所有 Release（自动分页，最多 maxPages 页）
	ListAllReleases(ctx context.Context, owner, repo, token string, maxPages int) ([]ProviderRelease, error)
	// GetAssetDownloadURL 获取资产的实际下载地址
	GetAssetDownloadURL(ctx context.Context, owner, repo string, asset ProviderAsset, token string) (string, error)
	// Name 返回提供者名称
	Name() string
}

// ProviderRelease 通用 Release 数据结构
type ProviderRelease struct {
	ID          int64          `json:"id"`
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	HTMLURL     string         `json:"html_url"`
	APIURL      string         `json:"api_url"`
	PublishedAt time.Time      `json:"published_at"`
	Assets      []ProviderAsset `json:"assets"`
}

// ProviderAsset 通用资产数据结构
type ProviderAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	ContentType        string `json:"content_type"`
	URL                string `json:"url"`
	BrowserDownloadURL string `json:"browser_download_url"`
}
