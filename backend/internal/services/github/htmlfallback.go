package github

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"releasehub/backend/internal/services/mirror"
)

// ErrHTMLFallbackUnsupported HTML 降级模式不支持该操作（例如全量分页拉取）
var ErrHTMLFallbackUnsupported = errors.New("HTML 降级模式不支持该操作")

const (
	// defaultWebBase GitHub 网页端点根地址
	defaultWebBase = "https://github.com"
	// htmlUserAgent 抓取网页时使用的 UA，便于 GitHub 侧识别
	htmlUserAgent = "ReleaseHub-HTMLFallback"
	// htmlTimeout 单次抓取超时
	htmlTimeout = 30 * time.Second
	// mirrorWebProbeTimeout 通过镜像抓网页的探测超时。
	// 实测部分镜像（如 mirror.skon.top）对网页类路径不返回 404 而是直接挂起，
	// 必须用短超时快速放弃，否则每次检查都要多等满 30 秒。
	mirrorWebProbeTimeout = 8 * time.Second
	// mirrorWebBlockTTL 镜像被判为「不支持网页加速」后的屏蔽时长
	mirrorWebBlockTTL = 30 * time.Minute
)

// mirrorWebBlocked 记录不支持网页加速的镜像地址（key=镜像 base，value=解除时间）。
// 使用包级变量而非实例字段：每次检查都会新建 HTMLFetcher，需要跨实例共享结论。
var mirrorWebBlocked sync.Map

// HTMLFetcher 通过 GitHub 网页端点抓取 Release 信息，不消耗 API 配额。
//
// 使用两个公开端点：
//   - /{owner}/{repo}/releases.atom                        release 列表（固定最近 10 条）
//   - /{owner}/{repo}/releases/expanded_assets/{tag}       指定 tag 的资产列表（一次返回全部）
//
// 注意：私有仓库无法通过该方式访问（网页端点不接受 PAT），必须走 API。
type HTMLFetcher struct {
	client *http.Client
	// base 网页端点基础地址，默认 https://github.com
	base string
	// mirrorBase 可选下载镜像前缀，非空时所有 github.com 请求先走镜像
	mirrorBase string
	// userAgent 动态 User-Agent 来源，nil 时用 htmlUserAgent。
	// 镜像站可能拦截非浏览器 UA，故允许复用全局下载 UA 配置。
	userAgent func() string
}

// WithUserAgentFunc 注入动态 User-Agent 来源（如 cfg.DownloadUserAgent）。
// 返回空串时仍回退 htmlUserAgent，保证不会发出空 UA。
func (f *HTMLFetcher) WithUserAgentFunc(fn func() string) *HTMLFetcher {
	if f != nil {
		f.userAgent = fn
	}
	return f
}

// resolveUserAgent 返回本次抓取使用的 UA，未注入时保持原 htmlUserAgent 行为
func (f *HTMLFetcher) resolveUserAgent() string {
	if f != nil && f.userAgent != nil {
		if ua := f.userAgent(); ua != "" {
			return ua
		}
	}
	return htmlUserAgent
}

// NewHTMLFetcher 创建 HTML 抓取器。
// base 为空时使用 https://github.com；mirrorBase 为空表示不经过下载镜像。
func NewHTMLFetcher(client *http.Client, base string, mirrorBase string) *HTMLFetcher {
	if client == nil {
		client = &http.Client{Timeout: htmlTimeout}
	}
	if strings.TrimSpace(base) == "" {
		base = defaultWebBase
	}

	return &HTMLFetcher{
		client:     client,
		base:       strings.TrimRight(strings.TrimSpace(base), "/"),
		mirrorBase: strings.TrimSpace(mirrorBase),
	}
}

// atomFeed releases.atom 的最小解析结构
type atomFeed struct {
	Entries []struct {
		Title   string `xml:"title"`
		Updated string `xml:"updated"`
		Link    struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Content string `xml:"content"`
	} `xml:"entry"`
}

var atomTagPattern = regexp.MustCompile(`/releases/tag/(.+)$`)

// Releases 通过 atom 抓取最近发布的一批 Release（GitHub 固定返回最近 10 条）。
// 返回结果按发布时间倒序，与网页展示顺序一致。
func (f *HTMLFetcher) Releases(ctx context.Context, owner string, repo string) ([]Release, error) {
	endpoint := f.webURL(path.Join(owner, repo, "releases.atom"))
	body, status, err := f.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		// 仓库不存在或为私有仓库（匿名访问同样返回 404，无法区分）
		return nil, fmt.Errorf("仓库 %s/%s 不存在或为私有仓库（HTML 抓取不支持私有仓库）", owner, repo)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("抓取 %s/%s 的 releases.atom 失败: HTTP %d", owner, repo, status)
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("解析 releases.atom 失败: %w", err)
	}

	releases := make([]Release, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		tag := parseTagFromLink(entry.Link.Href)
		if tag == "" {
			continue
		}
		release := Release{
			TagName: tag,
			Name:    strings.TrimSpace(entry.Title),
			// atom 的 content 是转义后的 HTML，反转义后与 API 的 body 等价
			Body:    html.UnescapeString(entry.Content),
			HTMLURL: fmt.Sprintf("%s/%s/%s/releases/tag/%s", defaultWebBase, owner, repo, url.PathEscape(tag)),
		}
		if publishedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.Updated)); err == nil {
			release.PublishedAt = publishedAt
		}
		if release.Name == "" {
			release.Name = tag
		}
		releases = append(releases, release)
	}

	return releases, nil
}

// LatestRelease 取 atom 中最新的一条，再补齐资产列表。
//
// 相比 /releases/latest 重定向，atom 更可靠：实测 golang/go、torvalds/linux、
// python/cpython 等仓库的 /releases/latest 会重定向回 /releases（甚至返回 404）。
func (f *HTMLFetcher) LatestRelease(ctx context.Context, owner string, repo string) (*Release, error) {
	releases, err := f.Releases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("仓库 %s/%s 没有可用的 Release", owner, repo)
	}

	latest := releases[0]
	assets, err := f.assets(ctx, owner, repo, latest.TagName)
	if err != nil {
		return nil, err
	}
	latest.Assets = assets

	return &latest, nil
}

// ReleaseByTag 抓取指定 tag 的 Release 及其资产列表。
func (f *HTMLFetcher) ReleaseByTag(ctx context.Context, owner string, repo string, tag string) (*Release, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, fmt.Errorf("tag 不能为空")
	}
	// tag 可包含 / 等特殊字符（如 release/1.0），必须逐段转义，否则会破坏路径
	escapedTag := escapeTag(tag)

	assets, err := f.assetsWithEscapedTag(ctx, owner, repo, tag, escapedTag)
	if err != nil {
		return nil, err
	}

	release := Release{
		TagName: tag,
		Name:    tag,
		HTMLURL: fmt.Sprintf("%s/%s/%s/releases/tag/%s", defaultWebBase, owner, repo, escapedTag),
		Assets:  assets,
	}
	return &release, nil
}

// assets 抓取指定 tag 的资产列表。
func (f *HTMLFetcher) assets(ctx context.Context, owner string, repo string, tag string) ([]Asset, error) {
	return f.assetsWithEscapedTag(ctx, owner, repo, tag, escapeTag(tag))
}

func (f *HTMLFetcher) assetsWithEscapedTag(ctx context.Context, owner string, repo string, tag string, escapedTag string) ([]Asset, error) {
	endpoint := f.webURL(path.Join(owner, repo, "releases", "expanded_assets") + "/" + escapedTag)
	body, status, err := f.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("Release %s 不存在", tag)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("抓取 %s/%s 的资产列表失败: HTTP %d", owner, repo, status)
	}

	return ParseExpandedAssets(string(body)), nil
}

// 资产行以 <a href="/o/r/releases/download/tag/name" 起始，其后片段包含大小、digest 与时间
var assetAnchorPattern = regexp.MustCompile(`<a href="(/[^"]+/releases/download/[^"]+)"`)

// size 取的是完整文本节点（>14.4 MB<），避免误匹配资产名中的数字
var assetSizePattern = regexp.MustCompile(`>(\d+(?:\.\d+)?)\s?(B|KB|MB|GB|TB)<`)

var assetDigestPattern = regexp.MustCompile(`sha256:([0-9a-f]{64})`)

var assetTimePattern = regexp.MustCompile(`<relative-time datetime="([^"]+)"`)

// ParseExpandedAssets 解析 expanded_assets 返回的 HTML 片段。
//
// 只保留 /releases/download/ 形式的真实资产链接：GitHub 会额外渲染
// /archive/refs/tags/{tag}.zip|tar.gz 两个源码包链接，它们不属于 release assets，
// 必须排除，否则会把源码包当成资产入库。
func ParseExpandedAssets(page string) []Asset {
	matches := assetAnchorPattern.FindAllStringSubmatchIndex(page, -1)
	assets := make([]Asset, 0, len(matches))

	for i, loc := range matches {
		end := len(page)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		// 当前资产链接到下一个资产链接之间的片段，包含该资产的元信息
		segment := page[loc[1]:end]
		linkPath := page[loc[2]:loc[3]]

		name := path.Base(linkPath)
		if unescaped, err := url.PathUnescape(name); err == nil {
			name = unescaped
		}
		if name == "" || name == "." || name == "/" {
			continue
		}

		asset := Asset{
			Name:               name,
			BrowserDownloadURL: defaultWebBase + linkPath,
		}
		if size := assetSizePattern.FindStringSubmatch(segment); len(size) > 2 {
			asset.Size = sizeToBytes(size[1], size[2])
		}
		if digest := assetDigestPattern.FindStringSubmatch(segment); len(digest) > 1 {
			asset.Digest = digest[1]
		}
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			asset.ContentType = contentType
		}
		assets = append(assets, asset)
	}

	return assets
}

// sizeToBytes 把页面上的显示值（如 14.4 MB）换算成近似的字节数。
// 显示值经过四舍五入，无法还原精确字节数；下载完成后会用真实大小覆盖。
func sizeToBytes(num string, unit string) int64 {
	value, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	multipliers := map[string]float64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}
	multiplier, ok := multipliers[unit]
	if !ok {
		return 0
	}

	return int64(value * multiplier)
}

// escapeTag 逐段转义 tag，保留 / 分隔符（部分仓库使用 release/1.0 形式）
func escapeTag(tag string) string {
	segments := strings.Split(tag, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}

	return strings.Join(segments, "/")
}

func parseTagFromLink(href string) string {
	trimmed := strings.TrimSpace(href)
	if trimmed == "" {
		return ""
	}
	if matched := atomTagPattern.FindStringSubmatch(trimmed); len(matched) > 1 {
		if unescaped, err := url.PathUnescape(matched[1]); err == nil {
			return unescaped
		}
		return matched[1]
	}

	return ""
}

// webURL 拼接网页端点地址；配置了镜像时先走镜像前缀。
func (f *HTMLFetcher) webURL(relativePath string) string {
	return f.base + "/" + strings.TrimLeft(relativePath, "/")
}

// get 请求网页端点。配置镜像时先尝试镜像，失败则回退直连 github.com。
func (f *HTMLFetcher) get(ctx context.Context, target string) ([]byte, int, error) {
	if f.mirrorBase == "" || mirrorWebUnsupported(f.mirrorBase) {
		return f.doGet(ctx, target)
	}

	mirrored := mirror.RewriteReleaseAsset(f.mirrorBase, target)
	if !mirrored.Applied {
		return f.doGet(ctx, target)
	}

	// 镜像探测使用更短的超时：镜像多只代理下载地址，对网页路径可能直接挂起
	probeCtx, cancel := context.WithTimeout(ctx, mirrorWebProbeTimeout)
	body, status, err := f.doGet(probeCtx, mirrored.URL)
	cancel()

	if err == nil && status == http.StatusOK {
		return body, status, nil
	}

	// 明确「不支持网页」或直接挂起时，把该镜像拉黑一段时间，避免每次检查都白等
	if err != nil || status == http.StatusForbidden || status == http.StatusNotFound {
		markMirrorWebUnsupported(f.mirrorBase)
	}

	// 回退直连 github.com
	directBody, directStatus, directErr := f.doGet(ctx, target)
	if directErr == nil {
		return directBody, directStatus, nil
	}
	// 直连也失败时，若镜像给出了明确响应则交回上层判断状态码，否则合并报错
	if err != nil {
		return nil, 0, fmt.Errorf("镜像抓取失败（%v），回退直连仍失败: %w", err, directErr)
	}

	return body, status, nil
}

// markMirrorWebUnsupported 临时屏蔽该镜像的网页抓取能力
func markMirrorWebUnsupported(mirrorBase string) {
	mirrorWebBlocked.Store(mirrorBase, time.Now().Add(mirrorWebBlockTTL))
}

// mirrorWebUnsupported 判断镜像是否仍在屏蔽期内
func mirrorWebUnsupported(mirrorBase string) bool {
	value, ok := mirrorWebBlocked.Load(mirrorBase)
	if !ok {
		return false
	}
	until, ok := value.(time.Time)
	if !ok || time.Now().After(until) {
		mirrorWebBlocked.Delete(mirrorBase)
		return false
	}

	return true
}

func (f *HTMLFetcher) doGet(ctx context.Context, target string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", f.resolveUserAgent())
	req.Header.Set("Accept", "text/html,application/atom+xml,application/xml")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	// 限制读取大小，避免异常页面耗尽内存
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return body, resp.StatusCode, nil
}

// isNotFoundError 判断是否为「仓库/Release 不存在或私有」
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()

	return strings.Contains(message, "不存在或为私有仓库") || strings.Contains(message, "不存在")
}
