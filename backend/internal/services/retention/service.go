package retention

import (
	"context"
	"fmt"
	"strings"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/storage"

	"gorm.io/gorm"
)

type Service struct {
	db      *gorm.DB
	storage *storage.LocalStorage
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

	return NewServiceWithStorage(db, localStorage), nil
}

func NewServiceWithStorage(db *gorm.DB, localStorage *storage.LocalStorage) *Service {
	return &Service{
		db:      db,
		storage: localStorage,
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

	result := &CleanupResult{
		Task:            &task,
		DeletedReleases: len(outdatedReleases),
	}

	var assets []models.Asset
	if err := s.db.WithContext(ctx).
		Where("release_id IN ?", releaseIDs).
		Find(&assets).Error; err != nil {
		s.failTask(ctx, &task, err)
		return result, err
	}

	for _, asset := range assets {
		if strings.TrimSpace(asset.StoragePath) == "" {
			continue
		}
		if err := s.storage.Delete(asset.StoragePath); err != nil {
			cleanupErr := fmt.Errorf("删除资产文件 %s 失败: %w", asset.StoragePath, err)
			s.failTask(ctx, &task, cleanupErr)
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
		s.failTask(ctx, &task, err)
		return result, err
	}

	return result, nil
}

func (s *Service) failTask(ctx context.Context, task *models.Task, err error) {
	now := time.Now().UTC()
	task.Status = models.TaskStatusFailed
	task.ErrorMessage = err.Error()
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(task).Error
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
