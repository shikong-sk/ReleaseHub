package scheduler

import (
	"context"
	"sync"
	"time"

	"releasehub/backend/internal/models"
	releasesvc "releasehub/backend/internal/services/release"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Checker interface {
	CheckLatest(ctx context.Context, repositoryID uint) (*releasesvc.CheckResult, error)
}

type Service struct {
	db       *gorm.DB
	checker  Checker
	logger   *zap.Logger
	interval time.Duration

	mu       sync.Mutex
	inFlight map[uint]struct{}
}

func NewService(db *gorm.DB, checker Checker, logger *zap.Logger, interval time.Duration) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	if interval <= 0 {
		interval = time.Minute
	}

	return &Service{
		db:       db,
		checker:  checker,
		logger:   logger,
		interval: interval,
		inFlight: map[uint]struct{}{},
	}
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		s.logger.Info("Scheduler 已启动", zap.Duration("interval", s.interval))
		defer s.logger.Info("Scheduler 已停止")

		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				s.RunDue(ctx, time.Now().UTC())
				timer.Reset(s.interval)
			}
		}
	}()
}

func (s *Service) RunDue(ctx context.Context, now time.Time) int {
	repositories, err := s.dueRepositories(ctx, now)
	if err != nil {
		s.logger.Error("查询待检查仓库失败", zap.Error(err))
		return 0
	}

	started := 0
	for _, repository := range repositories {
		if !s.tryMarkRunning(repository.ID) {
			continue
		}

		started++
		repositoryID := repository.ID
		go func() {
			defer s.unmarkRunning(repositoryID)
			if _, err := s.checker.CheckLatest(ctx, repositoryID); err != nil {
				s.logger.Warn("定时检查 Release 失败", zap.Uint("repositoryID", repositoryID), zap.Error(err))
			}
		}()
	}

	return started
}

func (s *Service) dueRepositories(ctx context.Context, now time.Time) ([]models.Repository, error) {
	var repositories []models.Repository
	if err := s.db.WithContext(ctx).
		Where("enabled = ?", true).
		Find(&repositories).Error; err != nil {
		return nil, err
	}

	due := make([]models.Repository, 0, len(repositories))
	for _, repository := range repositories {
		if repository.LastCheckAt == nil {
			due = append(due, repository)
			continue
		}

		interval := time.Duration(repository.IntervalSeconds) * time.Second
		if interval <= 0 {
			interval = 30 * time.Minute
		}
		if !repository.LastCheckAt.Add(interval).After(now) {
			due = append(due, repository)
		}
	}

	return due, nil
}

func (s *Service) tryMarkRunning(repositoryID uint) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.inFlight[repositoryID]; ok {
		return false
	}

	s.inFlight[repositoryID] = struct{}{}
	return true
}

func (s *Service) unmarkRunning(repositoryID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.inFlight, repositoryID)
}
