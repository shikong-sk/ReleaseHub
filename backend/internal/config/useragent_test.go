package config

import "testing"

// TestResolveUserAgent 覆盖 preset/custom 组合的解析优先级
func TestResolveUserAgent(t *testing.T) {
	cases := []struct {
		name   string
		preset string
		custom string
		want   string
	}{
		{name: "默认预设", preset: UserAgentPresetDefault, want: defaultUserAgent},
		{name: "空预设回退默认", preset: "", want: defaultUserAgent},
		{name: "内置 chrome", preset: "chrome-win", want: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"},
		{name: "custom 使用自定义值", preset: UserAgentPresetCustom, custom: "MyBot/1.0", want: "MyBot/1.0"},
		{name: "custom 空值回退默认", preset: UserAgentPresetCustom, custom: "", want: defaultUserAgent},
		{name: "custom 仅空白回退默认", preset: UserAgentPresetCustom, custom: "   ", want: defaultUserAgent},
		{name: "预设优先于自定义", preset: "firefox-win", custom: "MyBot/1.0", want: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0"},
		{name: "未知预设回退自定义", preset: "not-exist", custom: "MyBot/1.0", want: "MyBot/1.0"},
		{name: "未知预设无自定义回退默认", preset: "not-exist", want: defaultUserAgent},
		{name: "自定义值含换行被忽略", preset: UserAgentPresetCustom, custom: "a\r\nSet-Cookie: x", want: defaultUserAgent},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveUserAgent(tc.preset, tc.custom); got != tc.want {
				t.Fatalf("ResolveUserAgent(%q, %q) = %q, want %q", tc.preset, tc.custom, got, tc.want)
			}
		})
	}
}

// TestDownloadUserAgent 校验 Config 方法直接反映当前字段（热更新后即时生效）
func TestDownloadUserAgent(t *testing.T) {
	var nilCfg *Config
	if got := nilCfg.DownloadUserAgent(); got != defaultUserAgent {
		t.Fatalf("nil Config 应回退默认 UA，got %q", got)
	}

	cfg := &Config{Download: DownloadConfig{UserAgentPreset: UserAgentPresetCustom, UserAgent: "MyBot/1.0"}}
	if got := cfg.DownloadUserAgent(); got != "MyBot/1.0" {
		t.Fatalf("custom 未生效，got %q", got)
	}

	// 模拟热更新：直接改字段后应立刻反映
	cfg.Download.UserAgentPreset = "edge-win"
	if got := cfg.DownloadUserAgent(); got == "MyBot/1.0" || got == "" {
		t.Fatalf("切换预设未生效，got %q", got)
	}
}

// TestValidateUserAgentPreset 校验 preset 白名单
func TestValidateUserAgentPreset(t *testing.T) {
	valid := []string{"", UserAgentPresetDefault, UserAgentPresetCustom, "chrome-win", "edge-win", "firefox-win", "safari-mac", "chrome-android"}
	for _, p := range valid {
		if err := ValidateUserAgentPreset(p); err != nil {
			t.Fatalf("preset %q 应合法: %v", p, err)
		}
	}
	if err := ValidateUserAgentPreset("chrome-linux"); err == nil {
		t.Fatal("未收录的 preset 应报错")
	}
}

// TestValidateUserAgent 校验自定义 UA 的换行注入与长度限制
func TestValidateUserAgent(t *testing.T) {
	if err := ValidateUserAgent(""); err != nil {
		t.Fatalf("空 UA 应合法: %v", err)
	}
	if err := ValidateUserAgent("Mozilla/5.0 normal"); err != nil {
		t.Fatalf("普通 UA 应合法: %v", err)
	}
	if err := ValidateUserAgent("a\r\nb"); err == nil {
		t.Fatal("含 CRLF 的 UA 应被拒绝（HTTP 头注入）")
	}
	if err := ValidateUserAgent("a\nb"); err == nil {
		t.Fatal("含 LF 的 UA 应被拒绝")
	}
	long := make([]byte, maxUserAgentLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateUserAgent(string(long)); err == nil {
		t.Fatal("超长 UA 应被拒绝")
	}
}

// TestUserAgentPresetsCopy 确认返回的是副本，外部修改不影响包级内置表
func TestUserAgentPresetsCopy(t *testing.T) {
	presets := UserAgentPresets()
	if len(presets) < 3 {
		t.Fatalf("预设数量异常: %d", len(presets))
	}

	// 校验 custom 项存在且 UA 为空（前端据此显示自定义输入框）
	var foundCustom bool
	for _, p := range presets {
		if p.Value == UserAgentPresetCustom {
			foundCustom = true
			if p.UA != "" {
				t.Fatalf("custom 预设 UA 应为空，got %q", p.UA)
			}
		}
	}
	if !foundCustom {
		t.Fatal("预设列表缺少 custom 项")
	}

	presets[0].UA = "mutated"
	if UserAgentPresets()[0].UA == "mutated" {
		t.Fatal("UserAgentPresets 返回了内部切片，外部修改污染了包级状态")
	}
}
