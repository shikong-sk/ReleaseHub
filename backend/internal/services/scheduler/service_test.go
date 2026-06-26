package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/database"
	"releasehub/backend/internal/models"
	releasesvc "releasehub/backend/internal/services/release"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestRunDueSchedulesOnlyEnabledDueRepositories(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	recent := now.Add(-5 * time.Minute)
	old := now.Add(-1 * time.Hour)

	repositories := []models.Repository{
		{Provider: "github", Owner: "due", Repo: "never-checked", Enabled: true, IntervalSeconds: 1800, LastStatus: models.RepositoryStatusUnknown},
		{Provider: "github", Owner: "disabled", Repo: "repo", Enabled: false, IntervalSeconds: 1800, LastStatus: models.RepositoryStatusUnknown},
		{Provider: "github", Owner: "fresh", Repo: "repo", Enabled: true, IntervalSeconds: 1800, LastCheckAt: &recent, LastStatus: models.RepositoryStatusHealthy},
		{Provider: "github", Owner: "overdue", Repo: "repo", Enabled: true, IntervalSeconds: 1800, LastCheckAt: &old, LastStatus: models.RepositoryStatusHealthy},
	}
	for _, r := range repositories {
		if err := db.WithContext(context.Background()).Create(&r).Error; err != nil {
			t.Fatalf("创建测试仓库失败: %v", err)
		}
	}

	svc := NewService(db, (*releasesvc.CheckService)(nil), zap.NewNop(), time.Minute)
	started := svc.RunDue(context.Background(), now)
	if started != 2 {
		t.Fatalf("期望启动 2 个任务，实际 %d", started)
	}
}

func TestInFlightPreventsDuplicate(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	repo := models.Repository{
		Provider: "github", Owner: "acme", Repo: "tool", Enabled: true,
		IntervalSeconds: 1800, LastStatus: models.RepositoryStatusUnknown,
	}
	if err := db.WithContext(context.Background()).Create(&repo).Error; err != nil {
		t.Fatalf("创建测试仓库失败: %v", err)
	}

	svc := NewService(db, (*releasesvc.CheckService)(nil), zap.NewNop(), time.Minute)
	now := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)

	// 手动标记 in-flight，模拟一个正在运行的任务
	svc.tryMarkRunning(repo.ID)

	started := svc.RunDue(context.Background(), now)
	if started != 0 {
		t.Fatalf("in-flight 时应跳过，实际启动 %d", started)
	}

	svc.unmarkRunning(repo.ID)
	started = svc.RunDue(context.Background(), now)
	if started != 1 {
		t.Fatalf("in-flight 释放后应启动 1 个任务，实际 %d", started)
	}
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(config.DatabaseConfig{
		Driver: "sqlite",
		DSN:    filepath.Join(t.TempDir(), "scheduler-test.db"),
	})
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("迁移测试数据库失败: %v", err)
	}
	return db
}
