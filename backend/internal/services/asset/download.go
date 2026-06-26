package asset

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/downloader"
	"releasehub/backend/internal/services/storage"

	"gorm.io/gorm"
)

type Service struct {
	db         *gorm.DB
	storage    storage.Driver
	downloader *downloader.HTTPDownloader
}

type DownloadResult struct {
	Asset models.Asset `json:"asset"`
	Task  models.Task  `json:"task"`
}

func NewService(db *gorm.DB, storageConfig config.StorageConfig) (*Service, error) {
	localStorage, err := storage.NewLocalStorage(storageConfig.DataDir)
	if err != nil {
		return nil, err
	}

	return NewServiceWithDriver(db, localStorage), nil
}

// NewServiceWithDriver 使用指定存储驱动创建资产服务
func NewServiceWithDriver(db *gorm.DB, driver storage.Driver) *Service {
	return &Service{
		db:         db,
		storage:    driver,
		downloader: downloader.NewHTTPDownloader(),
	}
}

func (s *Service) Download(ctx context.Context, assetID uint) (*DownloadResult, error) {
	asset, release, repository, err := s.loadAssetContext(ctx, assetID)
	if err != nil {
		return nil, err
	}
	if asset.Status == models.AssetStatusSkipped {
		return nil, fmt.Errorf("资产已被过滤跳过，不能下载")
	}

	task := models.Task{
		Type:         "download_asset",
		RepositoryID: &repository.ID,
		ReleaseID:    &release.ID,
		AssetID:      &asset.ID,
		Status:       models.TaskStatusRunning,
		MaxAttempts:  1,
		StartedAt:    ptrTime(time.Now().UTC()),
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	token, err := s.githubToken(ctx, repository.GitHubTokenID)
	if err != nil {
		s.failTask(ctx, &task, err)
		return nil, err
	}

	downloadURL := asset.BrowserDownloadURL
	if strings.TrimSpace(downloadURL) == "" {
		downloadURL = asset.DownloadURL
	}
	if strings.TrimSpace(downloadURL) == "" {
		err := fmt.Errorf("资产缺少下载地址")
		s.failTask(ctx, &task, err)
		return nil, err
	}

	var buffer bytes.Buffer
	downloadResult, err := s.downloader.Download(ctx, downloadURL, token, &buffer)
	if err != nil {
		s.markAssetFailed(ctx, &asset, err)
		s.failTask(ctx, &task, err)
		return nil, err
	}

	objectPath := buildObjectPath(repository, release, asset)
	storedObject, err := s.storage.Put(ctx, objectPath, &buffer)
	if err != nil {
		s.markAssetFailed(ctx, &asset, err)
		s.failTask(ctx, &task, err)
		return nil, err
	}
	if err := s.storage.SetLatestTag(ctx, repository.Provider, repository.Owner, repository.Repo, release.Tag); err != nil {
		s.markAssetFailed(ctx, &asset, err)
		s.failTask(ctx, &task, err)
		return nil, err
	}

	now := time.Now().UTC()
	asset.StoragePath = storedObject.Path
	asset.Size = downloadResult.Size
	asset.SHA256 = downloadResult.SHA256
	asset.Status = models.AssetStatusVerified
	asset.ErrorMessage = ""
	asset.DownloadedAt = &now

	task.Status = models.TaskStatusSucceeded
	task.FinishedAt = &now

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&asset).Error; err != nil {
			return err
		}
		if err := tx.Save(&task).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &DownloadResult{
		Asset: asset,
		Task:  task,
	}, nil
}

// OpenReader 返回资产的读取流（兼容所有存储驱动）
func (s *Service) OpenReader(ctx context.Context, assetID uint) (*models.Asset, *storage.StoredObject, io.ReadCloser, error) {
	asset, _, _, err := s.loadAssetContext(ctx, assetID)
	if err != nil {
		return nil, nil, nil, err
	}
	if strings.TrimSpace(asset.StoragePath) == "" {
		return nil, nil, nil, fmt.Errorf("资产尚未下载")
	}

	reader, object, err := s.storage.Open(ctx, asset.StoragePath)
	if err != nil {
		return nil, nil, nil, err
	}

	return &asset, object, reader, nil
}

// Open 兼容旧接口，返回 *os.File（仅 Local 存储驱动可用）
func (s *Service) Open(ctx context.Context, assetID uint) (*models.Asset, *storage.StoredObject, io.ReadCloser, error) {
	return s.OpenReader(ctx, assetID)
}

func (s *Service) Delete(ctx context.Context, assetID uint) error {
	asset, _, _, err := s.loadAssetContext(ctx, assetID)
	if err != nil {
		return err
	}

	if strings.TrimSpace(asset.StoragePath) != "" {
		if err := s.storage.Delete(ctx, asset.StoragePath); err != nil {
			return err
		}
	}

	asset.Status = models.AssetStatusDeleted
	asset.StoragePath = ""
	asset.ErrorMessage = ""
	asset.DownloadedAt = nil

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&asset).Error; err != nil {
			return err
		}
		return tx.Delete(&asset).Error
	})
}

func (s *Service) loadAssetContext(ctx context.Context, assetID uint) (models.Asset, models.Release, models.Repository, error) {
	var asset models.Asset
	if err := s.db.WithContext(ctx).First(&asset, assetID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return asset, models.Release{}, models.Repository{}, fmt.Errorf("资产不存在")
		}
		return asset, models.Release{}, models.Repository{}, err
	}

	var release models.Release
	if err := s.db.WithContext(ctx).First(&release, asset.ReleaseID).Error; err != nil {
		return asset, release, models.Repository{}, err
	}

	var repository models.Repository
	if err := s.db.WithContext(ctx).First(&repository, release.RepositoryID).Error; err != nil {
		return asset, release, repository, err
	}

	return asset, release, repository, nil
}

func (s *Service) githubToken(ctx context.Context, tokenID *uint) (string, error) {
	if tokenID == nil {
		return "", nil
	}

	var token models.GitHubToken
	if err := s.db.WithContext(ctx).First(&token, *tokenID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("GitHub Token 不存在")
		}
		return "", err
	}

	return token.Token, nil
}

func (s *Service) markAssetFailed(ctx context.Context, asset *models.Asset, err error) {
	asset.Status = models.AssetStatusFailed
	asset.ErrorMessage = err.Error()
	_ = s.db.WithContext(ctx).Save(asset).Error
}

func (s *Service) failTask(ctx context.Context, task *models.Task, err error) {
	now := time.Now().UTC()
	task.Status = models.TaskStatusFailed
	task.ErrorMessage = err.Error()
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(task).Error
}

func buildObjectPath(repository models.Repository, release models.Release, asset models.Asset) string {
	return filepath.ToSlash(filepath.Join(
		"github",
		safeSegment(repository.Owner),
		safeSegment(repository.Repo),
		safeSegment(release.Tag),
		filepath.Base(asset.Name),
	))
}

func safeSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	if value == "" || value == "." || value == ".." {
		return "_"
	}
	return value
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
