// Package mirror 提供 GitHub 下载镜像（反向代理 / URL 前缀）的地址改写能力。
//
// 注意：镜像不是传输层代理。http/socks5 代理通过 http.Transport 拦截连接，
// 而镜像站属于应用层 URL 改写，因此必须作用于下载地址本身，无法复用 Proxy 模型。
package mirror

import (
	"fmt"
	"net/url"
	"strings"
)

// urlPlaceholder 镜像模板占位符，例如 https://ghproxy.example.com/{url}
const urlPlaceholder = "{url}"

// githubAssetHosts 需要被镜像改写的 GitHub 主机集合。
// 只包含 release 资产实际出现的主机，避免误改 API / 网页地址。
var githubAssetHosts = map[string]struct{}{
	"github.com":     {},
	"www.github.com": {},
}

// Rewrite 描述一次改写结果。
type Rewrite struct {
	URL     string // 改写后的下载地址；未改写时等于原始地址
	Applied bool   // 是否真正发生了改写
	Reason  string // 未改写/失败原因，便于写任务日志排查
}

// withScheme 在缺少 scheme 时补 https://，便于用户直接粘贴 ghproxy.example.com。
func withScheme(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		return "https://" + trimmed
	}

	return trimmed
}

// NormalizeBase 校验并规范化镜像基础地址。
// 允许省略 scheme（自动补 https://），仅接受 http/https 且必须带 host。
func NormalizeBase(baseURL string) (*url.URL, error) {
	normalized := withScheme(baseURL)
	if normalized == "" {
		return nil, fmt.Errorf("镜像地址为空")
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("镜像地址解析失败: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("镜像地址只支持 http/https，当前为 %s", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return nil, fmt.Errorf("镜像地址缺少主机名")
	}
	// 防止把占位符留在主机位置导致拼接出错误地址
	if strings.Contains(parsed.Host, urlPlaceholder) || strings.Contains(parsed.Host, "%7Burl%7D") {
		return nil, fmt.Errorf("镜像地址的占位符 {url} 不能出现在主机名中")
	}

	return parsed, nil
}

// ValidateBase 校验用户输入的镜像地址，空字符串表示不启用镜像。
func ValidateBase(baseURL string) error {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}

	_, err := NormalizeBase(baseURL)

	return err
}

// Build 基于镜像基础地址改写原始下载地址。
// 支持两种形态：
//  1. 模板式：https://ghproxy.example.com/{url} → 占位符被替换为原始地址
//  2. 前缀式：https://mirror.example.com → 原始地址按键拼接在末尾
func Build(baseURL string, raw string) (string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw, false, fmt.Errorf("原始下载地址为空")
	}

	mirrorURL, err := NormalizeBase(baseURL)
	if err != nil {
		return raw, false, err
	}

	// 占位符判断必须基于原始字符串：url.Parse 会把 { } 转义成 %7B %7D
	baseStr := withScheme(baseURL)
	var target string
	if strings.Contains(baseStr, urlPlaceholder) {
		target = strings.Replace(baseStr, urlPlaceholder, raw, 1)
	} else {
		target = strings.TrimRight(mirrorURL.String(), "/") + "/" + raw
	}

	parsed, err := url.Parse(target)
	if err != nil {
		return raw, false, fmt.Errorf("镜像地址拼接结果无效: %w", err)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return raw, false, fmt.Errorf("镜像地址拼接结果缺少主机名")
	}

	return parsed.String(), true, nil
}

// RewriteReleaseAsset 判断并按需改写 release 资产下载地址。
// 只有 GitHub 资产主机才会改写；其余地址（含已指向镜像的地址）原样返回。
func RewriteReleaseAsset(baseURL string, raw string) Rewrite {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Rewrite{URL: raw, Reason: "下载地址为空"}
	}
	if strings.TrimSpace(baseURL) == "" {
		return Rewrite{URL: raw, Reason: "未配置下载镜像"}
	}

	parsedRaw, err := url.Parse(trimmed)
	if err != nil {
		return Rewrite{URL: raw, Reason: fmt.Sprintf("原始下载地址无效: %v", err)}
	}
	if _, ok := githubAssetHosts[strings.ToLower(parsedRaw.Hostname())]; !ok {
		// 非 GitHub 资产地址（gitlab/gitea 等）不做改写
		return Rewrite{URL: raw, Reason: fmt.Sprintf("非 GitHub 资产地址: %s", parsedRaw.Hostname())}
	}

	mirrorURL, err := NormalizeBase(baseURL)
	if err != nil {
		return Rewrite{URL: raw, Reason: err.Error()}
	}

	// 已经是镜像地址的链接不重复改写，避免多层嵌套
	baseStr := strings.TrimRight(withScheme(baseURL), "/")
	if strings.HasPrefix(trimmed, strings.Replace(baseStr, urlPlaceholder, "", 1)) {
		return Rewrite{URL: raw, Reason: "下载地址已指向镜像"}
	}
	// 模板式镜像：目标 host 与镜像 host 相同即视为已改写
	if strings.EqualFold(parsedRaw.Hostname(), mirrorURL.Hostname()) {
		return Rewrite{URL: raw, Reason: "下载地址已指向镜像"}
	}

	rewritten, applied, err := Build(baseURL, trimmed)
	if err != nil {
		return Rewrite{URL: raw, Reason: err.Error()}
	}
	if !applied {
		return Rewrite{URL: raw, Reason: "镜像未应用"}
	}

	return Rewrite{URL: rewritten, Applied: true}
}
