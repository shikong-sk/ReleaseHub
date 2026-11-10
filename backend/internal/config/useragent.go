package config

import (
	"fmt"
	"strings"
)

// 内置 User-Agent 预设标识（与前端下拉选项、持久化配置值一致）
const (
	// UserAgentPresetDefault 使用程序默认 UA（ReleaseHub），便于上游识别来源
	UserAgentPresetDefault = "default"
	// UserAgentPresetCustom 使用用户自定义 UA 字符串（DownloadConfig.UserAgent）
	UserAgentPresetCustom = "custom"
)

// defaultUserAgent 程序默认 UA，与改造前的硬编码值保持一致
const defaultUserAgent = "ReleaseHub"

// maxUserAgentLength 自定义 UA 长度上限，防止误填超长串塞爆请求头
const maxUserAgentLength = 512

// UserAgentPreset 内置 UA 预设项：Value 为持久化值，Label 供前端展示，UA 为实际请求头值
type UserAgentPreset struct {
	Value string `json:"value"`
	Label string `json:"label"`
	UA    string `json:"ua"`
}

// builtinUserAgentPresets 内置常见浏览器 UA。
// 场景：部分 GitHub 镜像站/加速服务会对非浏览器 UA 返回人机验证页（HTTP 403 + HTML），
// 此时把下载请求伪装成浏览器 UA 即可正常取到文件。
var builtinUserAgentPresets = []UserAgentPreset{
	{
		Value: UserAgentPresetDefault,
		Label: "默认（ReleaseHub）",
		UA:    defaultUserAgent,
	},
	{
		Value: "chrome-win",
		Label: "Chrome / Windows",
		UA:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	},
	{
		Value: "edge-win",
		Label: "Edge / Windows",
		UA:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
	},
	{
		Value: "firefox-win",
		Label: "Firefox / Windows",
		UA:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
	},
	{
		Value: "safari-mac",
		Label: "Safari / macOS",
		UA:    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
	},
	{
		Value: "chrome-android",
		Label: "Chrome / Android",
		UA:    "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36",
	},
	{
		Value: UserAgentPresetCustom,
		Label: "自定义",
		UA:    "",
	},
}

// UserAgentPresets 返回内置预设副本，供 API 响应/前端下拉使用。
// 返回副本而非内部切片，避免调用方修改污染包级状态。
func UserAgentPresets() []UserAgentPreset {
	out := make([]UserAgentPreset, len(builtinUserAgentPresets))
	copy(out, builtinUserAgentPresets)
	return out
}

// lookupPresetUA 按 Value 查内置预设的 UA，未命中返回 false
func lookupPresetUA(preset string) (string, bool) {
	for _, p := range builtinUserAgentPresets {
		if p.Value == preset {
			return p.UA, true
		}
	}
	return "", false
}

// ValidateUserAgentPreset 校验 preset 合法性：空、内置值或 custom
func ValidateUserAgentPreset(preset string) error {
	if preset == "" {
		return nil
	}
	if _, ok := lookupPresetUA(preset); ok {
		return nil
	}
	return fmt.Errorf("userAgentPreset 不支持: %s（可选 default/custom/chrome-win/edge-win/firefox-win/safari-mac/chrome-android）", preset)
}

// ValidateUserAgent 校验自定义 UA：禁止换行（HTTP 头注入）、限长
func ValidateUserAgent(ua string) error {
	if strings.ContainsAny(ua, "\r\n") {
		return fmt.Errorf("userAgent 不能包含换行符")
	}
	if len(ua) > maxUserAgentLength {
		return fmt.Errorf("userAgent 长度不能超过 %d", maxUserAgentLength)
	}
	return nil
}

// ResolveUserAgent 解析最终使用的 User-Agent：
//   - preset 命中内置非 custom 预设 → 用预设 UA
//   - preset 为 custom 或空但配置了 UserAgent → 用自定义值
//   - 其余情况（含未识别的 preset、自定义值为空）→ 回退默认 UA
//
// 说明：未识别的 preset 不报错而是回退默认，保证脏数据（旧版本库、手工改库）
// 不会让下载链路因为一个 UA 字段彻底不可用。
func ResolveUserAgent(preset, custom string) string {
	if ua, ok := lookupPresetUA(strings.TrimSpace(preset)); ok && ua != "" {
		return ua
	}
	if v := strings.TrimSpace(custom); v != "" && !strings.ContainsAny(v, "\r\n") {
		return v
	}
	return defaultUserAgent
}

// DownloadUserAgent 返回当前配置解析出的下载 User-Agent。
// 每次都从 Config 读取，配置热更新后下一次请求即生效（无需重建下载器）。
func (c *Config) DownloadUserAgent() string {
	if c == nil {
		return defaultUserAgent
	}
	return ResolveUserAgent(c.Download.UserAgentPreset, c.Download.UserAgent)
}
