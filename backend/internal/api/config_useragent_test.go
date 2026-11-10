package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// newConfigTestRouter 构造仅用于配置接口测试的路由（带独立内存库，不依赖 GitHub 网络）
func newConfigTestRouter(t *testing.T) (http.Handler, *config.Config, *gorm.DB) {
	t.Helper()

	db := newTestDB(t)
	cfg := &config.Config{
		App:      config.AppConfig{Env: "test"},
		GitHub:   config.GitHubConfig{APIBaseURL: "https://api.github.com"},
		Storage:  config.StorageConfig{DataDir: t.TempDir()},
		Download: config.DownloadConfig{UserAgentPreset: config.UserAgentPresetDefault},
	}
	// 调度器/同步器传 nil：配置接口对 nil 已做保护
	router := NewRouter(Dependencies{
		Config: cfg,
		DB:     db,
		Logger: zap.NewNop(),
	})
	return router, cfg, db
}

// TestConfigGetReturnsUserAgentPresets GET /api/config 应下发预设列表与当前值
func TestConfigGetReturnsUserAgentPresets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, _, _ := newConfigTestRouter(t)

	rec := performRequest(router, http.MethodGet, "/api/config", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码不符: %d, body=%s", rec.Code, rec.Body.String())
	}

	var body configResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}

	if body.UserAgentPreset != config.UserAgentPresetDefault {
		t.Fatalf("默认预设不符: %s", body.UserAgentPreset)
	}
	if len(body.UserAgentPresets) == 0 {
		t.Fatal("响应未包含 UA 预设列表")
	}
	var hasCustom bool
	for _, p := range body.UserAgentPresets {
		if p.Value == config.UserAgentPresetCustom {
			hasCustom = true
			if p.Label == "" {
				t.Fatal("custom 预设缺少展示名")
			}
		}
	}
	if !hasCustom {
		t.Fatal("预设列表缺少 custom 项")
	}
}

// TestConfigUpdateUserAgent 更新 UA 配置应生效并持久化到 app_settings
func TestConfigUpdateUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, cfg, db := newConfigTestRouter(t)

	payload := []byte(`{"userAgentPreset":"chrome-win"}`)
	rec := performRequest(router, http.MethodPut, "/api/config", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败: %d, body=%s", rec.Code, rec.Body.String())
	}

	var body configResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if cfg.Download.UserAgentPreset != "chrome-win" {
		t.Fatalf("配置未更新: %s", cfg.Download.UserAgentPreset)
	}
	if body.UserAgentPreset != "chrome-win" {
		t.Fatalf("响应未返回新值: %s", body.UserAgentPreset)
	}
	// 解析出的 UA 应为 Chrome 浏览器串
	if ua := cfg.DownloadUserAgent(); ua == config.UserAgentPresetDefault || ua == "" {
		t.Fatalf("UA 解析结果不符: %s", ua)
	}

	// 校验持久化
	var setting models.AppSetting
	if err := db.Where("key = ?", "download.user_agent_preset").First(&setting).Error; err != nil {
		t.Fatalf("未持久化预设: %v", err)
	}
	if setting.Value != "chrome-win" {
		t.Fatalf("持久化值不符: %s", setting.Value)
	}
}

// TestConfigUpdateUserAgentCustom 自定义 UA 走 custom 预设时应保存原值
func TestConfigUpdateUserAgentCustom(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, cfg, _ := newConfigTestRouter(t)

	payload := []byte(`{"userAgentPreset":"custom","userAgent":"MyBot/2.0 (+https://example.com)"}`)
	rec := performRequest(router, http.MethodPut, "/api/config", payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败: %d, body=%s", rec.Code, rec.Body.String())
	}

	if got := cfg.DownloadUserAgent(); got != "MyBot/2.0 (+https://example.com)" {
		t.Fatalf("自定义 UA 未生效: %s", got)
	}
}

// TestConfigUpdateUserAgentRejected 非法输入应被拒绝且不改变配置
func TestConfigUpdateUserAgentRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router, cfg, _ := newConfigTestRouter(t)

	cases := []struct {
		name    string
		payload string
	}{
		{name: "不支持的预设", payload: `{"userAgentPreset":"chrome-linux"}`},
		{name: "自定义 UA 含换行", payload: `{"userAgentPreset":"custom","userAgent":"a\r\nX-Injected: 1"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := performRequest(router, http.MethodPut, "/api/config", []byte(tc.payload))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("期望 400，实际 %d, body=%s", rec.Code, rec.Body.String())
			}
			if cfg.Download.UserAgentPreset != config.UserAgentPresetDefault {
				t.Fatalf("非法请求不应改动配置，当前 preset=%s", cfg.Download.UserAgentPreset)
			}
		})
	}
}
