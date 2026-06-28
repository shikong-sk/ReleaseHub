package retention

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/database"
	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/storage"

	"gorm.io/gorm"
)

func TestCleanupRepositoryKeepsLatestReleasesAndDeletesOldAssets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(t.TempDir(), "releasehub-retention-test.db"),
	})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}

	localStorage, err := storage.NewLocalStorage(filepath.Join(t.TempDir(), "releases"))
	if err != nil {
		t.Fatalf("初始化本地存储失败: %v", err)
	}

	repository := models.Repository{
		Provider:            "github",
		Owner:               "acme",
		Repo:                "tool",
		Enabled:             true,
		IntervalSeconds:     1800,
		FilterMode:          "glob",
		RetentionKeepLatest: 2,
		LastStatus:          models.RepositoryStatusUnknown,
	}
	if err := db.WithContext(ctx).Create(&repository).Error; err != nil {
		t.Fatalf("创建仓库失败: %v", err)
	}

	releases := createRetentionTestReleases(t, ctx, db, localStorage, repository)
	oldRelease := releases[0]
	oldAssetPath := "github/acme/tool/v1.0.0/tool-linux-amd64.tar.gz"
	keptAssetPath := "github/acme/tool/v1.2.0/tool-linux-amd64.tar.gz"

	service := NewServiceWithDriver(db, localStorage)
	result, err := service.Cleanup(ctx, repository)
	if err != nil {
		t.Fatalf("清理旧版本失败: %v", err)
	}

	if result.DeletedReleases != 1 || result.DeletedAssets != 1 {
		t.Fatalf("清理统计不符合预期: %+v", result)
	}
	if result.Task == nil || result.Task.Status != models.TaskStatusSucceeded {
		t.Fatalf("清理任务状态不符合预期: %+v", result.Task)
	}

	if _, _, err := localStorage.Open(ctx, oldAssetPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("旧资产文件应被删除，实际错误: %v", err)
	}
	if reader, _, err := localStorage.Open(ctx, keptAssetPath); err != nil {
		t.Fatalf("保留版本资产文件不应被删除: %v", err)
	} else {
		_ = reader.Close()
	}

	var remainingReleases []models.Release
	if err := db.WithContext(ctx).Order("published_at DESC").Find(&remainingReleases).Error; err != nil {
		t.Fatalf("查询剩余 Release 失败: %v", err)
	}
	if len(remainingReleases) != 2 {
		t.Fatalf("应只保留 2 个 Release，实际 %d", len(remainingReleases))
	}
	if remainingReleases[0].Tag != "v1.2.0" || remainingReleases[1].Tag != "v1.1.0" {
		t.Fatalf("保留的 Release 不符合预期: %+v", remainingReleases)
	}

	// 硬删除验证：旧 Release 和 Asset 应完全不存在
	var deletedRelease models.Release
	if err := db.WithContext(ctx).First(&deletedRelease, oldRelease.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("旧 Release 应被硬删除，实际错误: %v", err)
	}

	var deletedAsset models.Asset
	if err := db.WithContext(ctx).
		Where("release_id = ?", oldRelease.ID).
		First(&deletedAsset).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("旧 Asset 应被硬删除，实际错误: %v", err)
	}

	var task models.Task
	if err := db.WithContext(ctx).
		Where("type = ? AND repository_id = ?", "cleanup_release", repository.ID).
		First(&task).Error; err != nil {
		t.Fatalf("查询清理任务失败: %v", err)
	}
	if task.Status != models.TaskStatusSucceeded {
		t.Fatalf("清理任务应成功，实际: %+v", task)
	}
}

func createRetentionTestReleases(t *testing.T, ctx context.Context, db *gorm.DB, localStorage *storage.LocalStorage, repository models.Repository) []models.Release {
	t.Helper()

	baseTime := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	tags := []string{"v1.0.0", "v1.1.0", "v1.2.0"}
	releases := make([]models.Release, 0, 3)
	for index, tag := range tags {
		publishedAt := baseTime.AddDate(0, 0, index)
		release := models.Release{
			RepositoryID: repository.ID,
			Tag:          tag,
			Name:         tag,
			PublishedAt:  &publishedAt,
			SyncStatus:   "checked",
			IsLatest:     index == 2,
		}
		if err := db.WithContext(ctx).Create(&release).Error; err != nil {
			t.Fatalf("创建 Release 失败: %v", err)
		}

		objectPath := filepath.ToSlash(filepath.Join("github", "acme", "tool", tag, "tool-linux-amd64.tar.gz"))
		if _, err := localStorage.Put(ctx, objectPath, strings.NewReader("asset-"+tag)); err != nil {
			t.Fatalf("写入测试资产失败: %v", err)
		}

		asset := models.Asset{
			ReleaseID:    release.ID,
			Name:         "tool-linux-amd64.tar.gz",
			StoragePath:  objectPath,
			Status:       models.AssetStatusVerified,
			DownloadedAt: ptrTimeForTest(publishedAt),
		}
		if err := db.WithContext(ctx).Create(&asset).Error; err != nil {
			t.Fatalf("创建 Asset 失败: %v", err)
		}
		releases = append(releases, release)
	}

	return releases
}

func ptrTimeForTest(t time.Time) *time.Time {
	return &t
}
