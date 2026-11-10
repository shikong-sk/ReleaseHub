package repository

import (
	"context"
	"strings"
	"testing"

	"releasehub/backend/internal/config"
)

// TestCreateRepositoryDownloadUserAgent 覆盖仓库级 UA 的创建、读取与更新流程
func TestCreateRepositoryDownloadUserAgent(t *testing.T) {
	db := setupRepositoryTestDB(t)
	service := NewService(db, config.StorageConfig{DataDir: t.TempDir()})
	ctx := context.Background()

	created, err := service.Create(ctx, CreateInput{
		Owner:                   "acme",
		Repo:                    "ua-repo",
		IntervalSeconds:         1800,
		FilterMode:              "glob",
		RetentionKeepLatest:     5,
		DownloadUserAgentPreset: "  chrome-win  ",
		DownloadUserAgent:       "",
	})
	if err != nil {
		t.Fatalf("创建仓库失败: %v", err)
	}
	if created.DownloadUserAgentPreset != "chrome-win" {
		t.Fatalf("preset 应被去除首尾空格，实际 %q", created.DownloadUserAgentPreset)
	}

	// 自定义 UA：preset 切到 custom 并写入 UA 值
	customUA := "MyRepoAgent/1.0 (+https://example.com)"
	preset := "custom"
	ua := customUA
	updated, err := service.Update(ctx, created.ID, UpdateInput{
		DownloadUserAgentPreset: &preset,
		DownloadUserAgent:       &ua,
	})
	if err != nil {
		t.Fatalf("更新仓库失败: %v", err)
	}
	if updated.DownloadUserAgentPreset != "custom" || updated.DownloadUserAgent != customUA {
		t.Fatalf("UA 未正确更新: preset=%q ua=%q", updated.DownloadUserAgentPreset, updated.DownloadUserAgent)
	}

	// 清空自定义值：应回到「跟随全局」（preset 空）语义
	empty := ""
	cleared, err := service.Update(ctx, created.ID, UpdateInput{
		DownloadUserAgentPreset: &empty,
		DownloadUserAgent:       &empty,
	})
	if err != nil {
		t.Fatalf("清空 UA 失败: %v", err)
	}
	if cleared.DownloadUserAgentPreset != "" || cleared.DownloadUserAgent != "" {
		t.Fatalf("UA 未清空: preset=%q ua=%q", cleared.DownloadUserAgentPreset, cleared.DownloadUserAgent)
	}
}

// TestRepositoryDownloadUserAgentValidation 非法 preset / 含换行 UA 应被拒绝且不落库
func TestRepositoryDownloadUserAgentValidation(t *testing.T) {
	db := setupRepositoryTestDB(t)
	service := NewService(db, config.StorageConfig{DataDir: t.TempDir()})
	ctx := context.Background()

	cases := []struct {
		name   string
		input  CreateInput
		errHas string
	}{
		{
			name: "不支持的预设",
			input: CreateInput{
				Owner: "acme", Repo: "bad-preset", IntervalSeconds: 1800, FilterMode: "glob", RetentionKeepLatest: 5,
				DownloadUserAgentPreset: "chrome-linux",
			},
			errHas: "userAgentPreset",
		},
		{
			name: "自定义 UA 含换行",
			input: CreateInput{
				Owner: "acme", Repo: "crlf-ua", IntervalSeconds: 1800, FilterMode: "glob", RetentionKeepLatest: 5,
				DownloadUserAgentPreset: "custom",
				DownloadUserAgent:       "a\r\nX-Injected: 1",
			},
			errHas: "换行",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Create(ctx, tc.input); err == nil {
				t.Fatalf("期望校验失败，实际创建成功")
			} else if !strings.Contains(err.Error(), tc.errHas) {
				t.Fatalf("错误信息应包含 %q，实际: %v", tc.errHas, err)
			}

			// 确认没有写入数据库
			var count int64
			if err := db.Table("repositories").Where("owner = ? AND repo = ?", tc.input.Owner, tc.input.Repo).Count(&count).Error; err != nil {
				t.Fatalf("查询失败: %v", err)
			}
			if count != 0 {
				t.Fatalf("校验失败的仓库不应落库，实际 %d 条", count)
			}
		})
	}
}
