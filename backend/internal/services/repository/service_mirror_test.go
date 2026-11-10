package repository

import (
	"context"
	"testing"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// 使用内存库避免 Windows 下 TempDir 清理与 SQLite 句柄冲突
	db, err := gorm.Open(sqlite.Open("file:repository_mirror_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Storage{},
		&models.Repository{},
		&models.RepositoryStorage{},
		&models.Release{},
		&models.Asset{},
	); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}

	return db
}

// TestCreateAndUpdateDownloadMirror 覆盖下载镜像字段的创建、更新与清空流程
func TestCreateAndUpdateDownloadMirror(t *testing.T) {
	db := setupRepositoryTestDB(t)
	service := NewService(db, config.StorageConfig{DataDir: t.TempDir()})
	ctx := context.Background()

	created, err := service.Create(ctx, CreateInput{
		Owner:                 "shikong-sk",
		Repo:                  "releasehub",
		IntervalSeconds:       1800,
		FilterMode:            "glob",
		RetentionKeepLatest:   5,
		DownloadMirrorBaseUrl: "  ghproxy.example.com/{url}  ",
	})
	if err != nil {
		t.Fatalf("创建仓库失败: %v", err)
	}
	if created.DownloadMirrorBaseUrl != "ghproxy.example.com/{url}" {
		t.Fatalf("镜像地址应被去除首尾空格，实际为 %q", created.DownloadMirrorBaseUrl)
	}

	// 重新读取确认字段已落库
	stored, err := service.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("读取仓库失败: %v", err)
	}
	if stored.DownloadMirrorBaseUrl != "ghproxy.example.com/{url}" {
		t.Fatalf("镜像地址未持久化，实际为 %q", stored.DownloadMirrorBaseUrl)
	}

	// 更新为新的镜像地址
	next := "https://mirror.example.com"
	updated, err := service.Update(ctx, created.ID, UpdateInput{DownloadMirrorBaseUrl: &next})
	if err != nil {
		t.Fatalf("更新镜像地址失败: %v", err)
	}
	if updated.DownloadMirrorBaseUrl != next {
		t.Fatalf("更新后镜像地址应为 %q，实际为 %q", next, updated.DownloadMirrorBaseUrl)
	}

	// 传空字符串表示关闭镜像
	empty := ""
	cleared, err := service.Update(ctx, created.ID, UpdateInput{DownloadMirrorBaseUrl: &empty})
	if err != nil {
		t.Fatalf("清空镜像地址失败: %v", err)
	}
	if cleared.DownloadMirrorBaseUrl != "" {
		t.Fatalf("清空后镜像地址应为空，实际为 %q", cleared.DownloadMirrorBaseUrl)
	}
}

// TestDownloadMirrorValidation 非法镜像地址应在创建/更新阶段被拒绝
func TestDownloadMirrorValidation(t *testing.T) {
	db := setupRepositoryTestDB(t)
	service := NewService(db, config.StorageConfig{DataDir: t.TempDir()})
	ctx := context.Background()

	_, err := service.Create(ctx, CreateInput{
		Owner:                 "owner",
		Repo:                  "repo",
		IntervalSeconds:       1800,
		FilterMode:            "glob",
		RetentionKeepLatest:   5,
		DownloadMirrorBaseUrl: "ftp://mirror.example.com",
	})
	if !IsInvalidInput(err) {
		t.Fatalf("非法镜像地址应返回 InvalidInput，实际: %v", err)
	}

	created, err := service.Create(ctx, CreateInput{
		Owner:               "owner",
		Repo:                "repo2",
		IntervalSeconds:     1800,
		FilterMode:          "glob",
		RetentionKeepLatest: 5,
	})
	if err != nil {
		t.Fatalf("创建无镜像仓库失败: %v", err)
	}

	bad := "https://"
	if _, err := service.Update(ctx, created.ID, UpdateInput{DownloadMirrorBaseUrl: &bad}); !IsInvalidInput(err) {
		t.Fatalf("更新时非法镜像地址应返回 InvalidInput，实际: %v", err)
	}
}

func TestValidateDownloadMirror(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "空串允许", input: "", wantErr: false},
		{name: "前缀式", input: "https://ghproxy.example.com", wantErr: false},
		{name: "模板式", input: "https://ghproxy.example.com/{url}", wantErr: false},
		{name: "缺 scheme 自动补全", input: "ghproxy.example.com", wantErr: false},
		{name: "非法 scheme", input: "ftp://ghproxy.example.com", wantErr: true},
		{name: "缺少主机", input: "https://", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDownloadMirror(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("期望错误=%v，实际: %v", tc.wantErr, err)
			}
		})
	}
}
