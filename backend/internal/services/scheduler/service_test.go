package scheduler

import (
	"context"
	"path/filepath"
	"sync"
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
		{Provider: "github", Owner: "old", Repo: "repo", Enabled: true, IntervalSeconds: 1800, LastCheckAt: &old, LastStatus: models.RepositoryStatusHealthy},
	}
	for i := range repositories {
		if err := db.Create(&repositories[i]).Error; err != nil {
			t.Fatalf("创建测试仓库失败: %v", err)
		}
	}

	checker := &fakeChecker{}
	service := NewService(db, checker, zap.NewNop(), time.Minute)

	started := service.RunDue(context.Background(), now)
	if started != 2 {
		t.Fatalf("期望调度 2 个仓库，实际 %d", started)
	}

	checker.waitForCalls(t, 2)
	if !checker.called(repositories[0].ID) || !checker.called(repositories[3].ID) {
		t.Fatalf("调度仓库不符合预期: %+v", checker.calls())
	}
}

func TestRunDueSkipsInFlightRepository(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repository := models.Repository{
		Provider:        "github",
		Owner:           "acme",
		Repo:            "tool",
		Enabled:         true,
		IntervalSeconds: 1800,
		LastStatus:      models.RepositoryStatusUnknown,
	}
	if err := db.Create(&repository).Error; err != nil {
		t.Fatalf("创建测试仓库失败: %v", err)
	}

	checker := &blockingChecker{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := NewService(db, checker, zap.NewNop(), time.Minute)

	started := service.RunDue(context.Background(), time.Now().UTC())
	if started != 1 {
		t.Fatalf("期望首次调度 1 个仓库，实际 %d", started)
	}
	checker.waitStarted(t)

	started = service.RunDue(context.Background(), time.Now().UTC())
	if started != 0 {
		t.Fatalf("in-flight 仓库不应重复调度，实际 %d", started)
	}

	close(checker.release)
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

type fakeChecker struct {
	mu       sync.Mutex
	calledID []uint
}

func (f *fakeChecker) CheckLatest(_ context.Context, repositoryID uint) (*releasesvc.CheckResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calledID = append(f.calledID, repositoryID)
	return nil, nil
}

func (f *fakeChecker) waitForCalls(t *testing.T, count int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(f.calls()) == count {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("等待调用超时，当前调用: %+v", f.calls())
}

func (f *fakeChecker) called(repositoryID uint) bool {
	for _, calledID := range f.calls() {
		if calledID == repositoryID {
			return true
		}
	}
	return false
}

func (f *fakeChecker) calls() []uint {
	f.mu.Lock()
	defer f.mu.Unlock()

	calls := make([]uint, len(f.calledID))
	copy(calls, f.calledID)
	return calls
}

type blockingChecker struct {
	startOnce sync.Once
	started   chan struct{}
	release   chan struct{}
}

func (b *blockingChecker) CheckLatest(_ context.Context, _ uint) (*releasesvc.CheckResult, error) {
	b.startOnce.Do(func() {
		close(b.started)
	})
	<-b.release
	return nil, nil
}

func (b *blockingChecker) waitStarted(t *testing.T) {
	t.Helper()

	select {
	case <-b.started:
	case <-time.After(2 * time.Second):
		t.Fatal("等待阻塞 checker 启动超时")
	}
}
