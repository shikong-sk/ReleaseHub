package scheduler

import (
	"context"
	"sync"
	"time"

	"releasehub/backend/internal/models"
	releasesvc "releasehub/backend/internal/services/release"
	syncersvc "releasehub/backend/internal/services/syncer"
	tasklogsvc "releasehub/backend/internal/services/tasklog"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const defaultMaxConcurrent = 5

type Service struct {
	db            *gorm.DB
	checker       *releasesvc.CheckService
	syncer        *syncersvc.Service
	logger        *zap.Logger
	logService    *tasklogsvc.Service
	interval      time.Duration
	maxConcurrent int

	mu       sync.Mutex
	inFlight map[uint]struct{}

	// 全局并发信号量，控制同时运行的同步/检查任务数
	semaphore chan struct{}
}

// UpdateInterval 运行时更新调度间隔
func (s *Service) UpdateInterval(interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	s.interval = interval
}

// UpdateMaxConcurrent 运行时更新最大并发数
func (s *Service) UpdateMaxConcurrent(maxConcurrent int) {
	if maxConcurrent < 1 {
		maxConcurrent = defaultMaxConcurrent
	}
	s.maxConcurrent = maxConcurrent
	// 重建信号量以匹配新的并发上限
	s.semaphore = make(chan struct{}, maxConcurrent)
}

func NewService(db *gorm.DB, checker *releasesvc.CheckService, logger *zap.Logger, interval time.Duration) *Service {
	return NewServiceWithConcurrency(db, checker, logger, interval, defaultMaxConcurrent)
}

func NewServiceWithConcurrency(db *gorm.DB, checker *releasesvc.CheckService, logger *zap.Logger, interval time.Duration, maxConcurrent int) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	if interval <= 0 {
		interval = time.Minute
	}
	if maxConcurrent < 1 {
		maxConcurrent = defaultMaxConcurrent
	}

	return &Service{
		db:            db,
		checker:       checker,
		logger:        logger,
		logService:    tasklogsvc.NewService(db),
		interval:      interval,
		maxConcurrent: maxConcurrent,
		inFlight:      map[uint]struct{}{},
		semaphore:     make(chan struct{}, maxConcurrent),
	}
}

// WithSyncer 设置同步服务，定时任务将执行同步（检查+下载）而非仅检查
func (s *Service) WithSyncer(syncer *syncersvc.Service) *Service {
	s.syncer = syncer
	return s
}

func (s *Service) Start(ctx context.Context) {
	go func() {
		s.logger.Info("Scheduler 已启动",
			zap.Duration("interval", s.interval),
			zap.Int("maxConcurrent", s.maxConcurrent),
		)
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

	if len(repositories) > 0 {
		s.logger.Info("Scheduler 触发定时任务",
			zap.Int("dueCount", len(repositories)),
		)
	}

	started := 0
	for _, repository := range repositories {
		if !s.tryMarkRunning(repository.ID) {
			continue
		}

		// 非阻塞获取信号量：如果已达并发上限则跳过本次
		select {
		case s.semaphore <- struct{}{}:
		default:
			s.unmarkRunning(repository.ID)
			s.logger.Debug("跳过仓库同步：已达全局并发上限",
				zap.Uint("repositoryID", repository.ID),
				zap.Int("maxConcurrent", s.maxConcurrent),
			)
			continue
		}

		started++
		repositoryID := repository.ID
		go func() {
			defer func() {
				<-s.semaphore
				s.unmarkRunning(repositoryID)
			}()
			if s.syncer != nil {
				if _, syncErr := s.syncer.EnqueueSyncRepository(ctx, repositoryID); syncErr != nil {
					s.logger.Warn("定时同步仓库失败", zap.Uint("repositoryID", repositoryID), zap.Error(syncErr))
				}
			} else if s.checker != nil {
				if _, checkErr := s.checker.CheckLatest(ctx, repositoryID); checkErr != nil {
					s.logger.Warn("定时检查 Release 失败", zap.Uint("repositoryID", repositoryID), zap.Error(checkErr))
				}
			}
		}()
	}

	// 在同步仓库之后，重试失败的资产下载
	s.retryFailedAssetsInScheduling(ctx)

	return started
}

// retryFailedAssetsInScheduling 在调度循环中重试最近失败的资产下载。
// 按≤5次自动重试、指数退避（attempt²分钟）策略执行，
// 通过检查 pending/running 任务避免与同步入队重复。
func (s *Service) retryFailedAssetsInScheduling(ctx context.Context) {
	if s.syncer == nil {
		return
	}

	var failedAssets []models.Asset
	if err := s.db.WithContext(ctx).
		Where("status = ?", models.AssetStatusFailed).
		Order("updated_at ASC").
		Limit(5).
		Find(&failedAssets).Error; err != nil {
		return
	}

	retried := 0
	now := time.Now().UTC()
	for _, asset := range failedAssets {
		// 检查失败次数，超过 5 次不再自动重试
		var failCount int64
		s.db.WithContext(ctx).
			Model(&models.Task{}).
			Where("asset_id = ? AND status = ?", asset.ID, models.TaskStatusFailed).
			Count(&failCount)
		if failCount >= 5 {
			continue
		}

		// 去重：已有 pending/running 任务的资产跳过，避免与同步入队重复
		var inFlightCount int64
		s.db.WithContext(ctx).
			Model(&models.Task{}).
			Where("asset_id = ? AND status IN ?", asset.ID, []models.TaskStatus{
				models.TaskStatusPending, models.TaskStatusRunning,
			}).
			Count(&inFlightCount)
		if inFlightCount > 0 {
			continue
		}

		// 指数退避：backoff = failCount² 分钟，距离最近一次失败不足 backoff 则跳过
		backoff := time.Duration(failCount*failCount) * time.Minute
		var lastFailedTask models.Task
		if err := s.db.WithContext(ctx).
			Where("asset_id = ? AND status = ?", asset.ID, models.TaskStatusFailed).
			Order("finished_at DESC").
			First(&lastFailedTask).Error; err == nil &&
			lastFailedTask.FinishedAt != nil &&
			lastFailedTask.FinishedAt.Add(backoff).After(now) {
			continue
		}

		if _, err := s.syncer.EnqueueRetryAsset(ctx, asset.ID); err != nil {
			s.logger.Debug("重试下载资产失败",
				zap.Uint("assetID", asset.ID),
				zap.String("name", asset.Name),
				zap.Error(err),
			)
			continue
		}
		retried++
	}

	if retried > 0 {
		s.logger.Info("Scheduler 自动重试失败资产", zap.Int("retried", retried))
	}
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
