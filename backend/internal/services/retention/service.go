package retention

import (
	"context"
	"fmt"
	"strings"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/storage"
	tasklogsvc "releasehub/backend/internal/services/tasklog"

	"gorm.io/gorm"
)

// Service 保留策略清理服务
type Service struct {
	db         *gorm.DB
	storage    storage.Driver // 兼容旧接口，仅当 storages 为 nil 时使用
	storages   *storage.DriverFactory
	logService *tasklogsvc.Service
}

type CleanupResult struct {
	Task             *models.Task `json:"task,omitempty"`
	DeletedReleases  int          `json:"deletedReleases"`
	DeletedAssets    int          `json:"deletedAssets"`
	DeletedFilePaths []string     `json:"deletedFilePaths"`
}

func NewService(db *gorm.DB, storageConfig config.StorageConfig) (*Service, error) {
	localStorage, err := storage.NewLocalStorage(storageConfig.DataDir)
	if err != nil {
		return nil, err
	}

	return NewServiceWithDriver(db, localStorage), nil
}

// NewServiceWithFactory 使用 DriverFactory 创建服务，按仓库动态选择存储
func NewServiceWithFactory(db *gorm.DB, storageConfig config.StorageConfig) *Service {
	return &Service{
		db:         db,
		storages:   storage.NewDriverFactory(db, storageConfig),
		logService: tasklogsvc.NewService(db),
	}
}

func NewServiceWithDriver(db *gorm.DB, driver storage.Driver) *Service {
	return &Service{
		db:         db,
		storage:    driver,
		logService: tasklogsvc.NewService(db),
	}
}

func (s *Service) CleanupRepository(ctx context.Context, repository models.Repository) error {
	_, err := s.Cleanup(ctx, repository)
	return err
}

func (s *Service) Cleanup(ctx context.Context, repository models.Repository) (*CleanupResult, error) {
	keepLatest := repository.RetentionKeepLatest
	if keepLatest < 1 {
		keepLatest = 1
	}

	var releases []models.Release
	if err := s.db.WithContext(ctx).
		Where("repository_id = ?", repository.ID).
		Order("published_at DESC, created_at DESC").
		Find(&releases).Error; err != nil {
		return nil, err
	}

	if len(releases) <= keepLatest {
		return &CleanupResult{}, nil
	}

	outdatedReleases := releases[keepLatest:]
	releaseIDs := make([]uint, 0, len(outdatedReleases))
	for _, release := range outdatedReleases {
		releaseIDs = append(releaseIDs, release.ID)
	}

	task := models.Task{
		Type:         "cleanup_release",
		RepositoryID: &repository.ID,
		Status:       models.TaskStatusRunning,
		MaxAttempts:  1,
		StartedAt:    ptrTime(time.Now().UTC()),
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf(
		"开始清理: 保留最近 %d 个版本，删除 %d 个旧版本",
		keepLatest, len(outdatedReleases),
	))

	result := &CleanupResult{
		Task:            &task,
		DeletedReleases: len(outdatedReleases),
	}

	var assets []models.Asset
	if err := s.db.WithContext(ctx).
		Where("release_id IN ?", releaseIDs).
		Find(&assets).Error; err != nil {
		s.failTaskWithLog(ctx, &task, err, "查询旧版本资产失败")
		return result, err
	}

	// 按仓库动态选择存储驱动
	driver, err := s.storageDriver(ctx, repository)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "获取仓库存储驱动失败")
		return result, err
	}

	for _, asset := range assets {
		if strings.TrimSpace(asset.StoragePath) == "" {
			continue
		}
		if err := driver.Delete(ctx, asset.StoragePath); err != nil {
			cleanupErr := fmt.Errorf("删除资产文件 %s 失败: %w", asset.StoragePath, err)
			s.failTaskWithLog(ctx, &task, cleanupErr, "删除存储文件失败")
			return result, cleanupErr
		}
		result.DeletedFilePaths = append(result.DeletedFilePaths, asset.StoragePath)
	}

	now := time.Now().UTC()
	result.DeletedAssets = len(assets)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(assets) > 0 {
			if err := tx.Model(&models.Asset{}).
				Where("release_id IN ?", releaseIDs).
				Updates(map[string]any{
					"status":       models.AssetStatusDeleted,
					"storage_path": "",
					"updated_at":   now,
				}).Error; err != nil {
				return err
			}
			if err := tx.Where("release_id IN ?", releaseIDs).Delete(&models.Asset{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Model(&models.Release{}).
			Where("id IN ?", releaseIDs).
			Updates(map[string]any{
				"is_latest":   false,
				"sync_status": "deleted",
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Where("id IN ?", releaseIDs).Delete(&models.Release{}).Error; err != nil {
			return err
		}

		task.Status = models.TaskStatusSucceeded
		task.FinishedAt = &now
		if err := tx.Save(&task).Error; err != nil {
			return err
		}

		result.Task = &task
		return nil
	}); err != nil {
		s.failTaskWithLog(ctx, &task, err, "清理事务失败")
		return result, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf(
		"清理完成: 删除 %d 个旧版本，%d 个资产文件",
		result.DeletedReleases, result.DeletedAssets,
	))

	return result, nil
}

// storageDriver 按仓库配置动态选择存储驱动
func (s *Service) storageDriver(ctx context.Context, repository models.Repository) (storage.Driver, error) {
	if s.storages != nil {
		return s.storages.DriverForRepository(ctx, repository)
	}
	return s.storage, nil
}

func (s *Service) failTask(ctx context.Context, task *models.Task, err error) {
	now := time.Now().UTC()
	task.Status = models.TaskStatusFailed
	task.ErrorMessage = err.Error()
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(task).Error
}

// failTaskWithLog 标记任务失败并写入日志
func (s *Service) failTaskWithLog(ctx context.Context, task *models.Task, err error, logMsg string) {
	s.failTask(ctx, task, err)
	s.appendLog(ctx, task.ID, "error", logMsg+": "+err.Error())
}

// appendLog 写入任务日志
func (s *Service) appendLog(ctx context.Context, taskID uint, level string, message string) {
	if s.logService != nil {
		_ = s.logService.Append(ctx, taskID, level, message)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
