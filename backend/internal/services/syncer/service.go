package syncer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	stdsync "sync"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"
	assetsvc "releasehub/backend/internal/services/asset"
	releasesvc "releasehub/backend/internal/services/release"

	"gorm.io/gorm"
)

const defaultMaxConcurrentDownloads = 3

type Service struct {
	db                     *gorm.DB
	checker                *releasesvc.CheckService
	assetService           *assetsvc.Service
	maxConcurrentDownloads int
}

type Result struct {
	Repository      models.Repository         `json:"repository"`
	Release         models.Release            `json:"release"`
	Assets          []models.Asset            `json:"assets"`
	Task            models.Task               `json:"task"`
	CheckTask       models.Task               `json:"checkTask"`
	DownloadResults []assetsvc.DownloadResult `json:"downloadResults"`
	FailedAssets    []AssetError              `json:"failedAssets"`
}

type AssetError struct {
	AssetID uint   `json:"assetId"`
	Name    string `json:"name"`
	Error   string `json:"error"`
}

func NewService(db *gorm.DB, checker *releasesvc.CheckService, storageConfig config.StorageConfig) (*Service, error) {
	assetService, err := assetsvc.NewService(db, storageConfig)
	if err != nil {
		return nil, err
	}

	return &Service{
		db:                     db,
		checker:                checker,
		assetService:           assetService,
		maxConcurrentDownloads: defaultMaxConcurrentDownloads,
	}, nil
}

func (s *Service) SyncRepository(ctx context.Context, repositoryID uint) (*Result, error) {
	task := models.Task{
		Type:         "sync_release",
		RepositoryID: &repositoryID,
		Status:       models.TaskStatusRunning,
		MaxAttempts:  1,
		StartedAt:    ptrTime(time.Now().UTC()),
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	checkResult, err := s.checker.CheckLatest(ctx, repositoryID)
	if err != nil {
		s.failTask(ctx, &task, err)
		return nil, err
	}

	task.ReleaseID = &checkResult.Release.ID
	result := &Result{
		Repository: checkResult.Repository,
		Release:    checkResult.Release,
		Assets:     checkResult.Assets,
		Task:       task,
		CheckTask:  checkResult.Task,
	}

	assetsToDownload := downloadableAssets(checkResult.Assets)
	downloadResults, failedAssets := s.downloadAssets(ctx, assetsToDownload)
	result.DownloadResults = downloadResults
	result.FailedAssets = failedAssets

	if err := s.reloadAssets(ctx, &result.Assets, checkResult.Release.ID); err != nil {
		s.failTask(ctx, &task, err)
		return result, err
	}

	now := time.Now().UTC()
	task.FinishedAt = &now
	if len(failedAssets) > 0 {
		task.Status = models.TaskStatusFailed
		task.ErrorMessage = joinAssetErrors(failedAssets)
		if err := s.db.WithContext(ctx).Save(&task).Error; err != nil {
			return result, err
		}
		result.Task = task
		return result, errors.New(task.ErrorMessage)
	}

	task.Status = models.TaskStatusSucceeded
	if err := s.db.WithContext(ctx).Save(&task).Error; err != nil {
		return result, err
	}
	result.Task = task
	return result, nil
}

func downloadableAssets(assets []models.Asset) []models.Asset {
	downloadable := make([]models.Asset, 0, len(assets))
	for _, asset := range assets {
		if asset.Status == models.AssetStatusPending || asset.Status == models.AssetStatusFailed {
			downloadable = append(downloadable, asset)
		}
	}

	return downloadable
}

func (s *Service) downloadAssets(ctx context.Context, assets []models.Asset) ([]assetsvc.DownloadResult, []AssetError) {
	if len(assets) == 0 {
		return nil, nil
	}

	limit := s.maxConcurrentDownloads
	if limit < 1 {
		limit = defaultMaxConcurrentDownloads
	}

	semaphore := make(chan struct{}, limit)
	var wg stdsync.WaitGroup
	var mu stdsync.Mutex
	downloadResults := make([]assetsvc.DownloadResult, 0, len(assets))
	failedAssets := make([]AssetError, 0)

	for _, asset := range assets {
		asset := asset
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case <-ctx.Done():
				mu.Lock()
				failedAssets = append(failedAssets, AssetError{
					AssetID: asset.ID,
					Name:    asset.Name,
					Error:   ctx.Err().Error(),
				})
				mu.Unlock()
				return
			case semaphore <- struct{}{}:
				defer func() {
					<-semaphore
				}()
			}

			downloadResult, err := s.assetService.Download(ctx, asset.ID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failedAssets = append(failedAssets, AssetError{
					AssetID: asset.ID,
					Name:    asset.Name,
					Error:   err.Error(),
				})
				return
			}
			downloadResults = append(downloadResults, *downloadResult)
		}()
	}

	wg.Wait()
	return downloadResults, failedAssets
}

func (s *Service) reloadAssets(ctx context.Context, assets *[]models.Asset, releaseID uint) error {
	return s.db.WithContext(ctx).
		Where("release_id = ?", releaseID).
		Order("name ASC").
		Find(assets).Error
}

func (s *Service) failTask(ctx context.Context, task *models.Task, err error) {
	now := time.Now().UTC()
	task.Status = models.TaskStatusFailed
	task.ErrorMessage = err.Error()
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(task).Error
}

func joinAssetErrors(failedAssets []AssetError) string {
	messages := make([]string, 0, len(failedAssets))
	for _, failedAsset := range failedAssets {
		messages = append(messages, fmt.Sprintf("%s: %s", failedAsset.Name, failedAsset.Error))
	}

	return strings.Join(messages, "; ")
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
