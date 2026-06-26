package asset

import (
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
	"releasehub/backend/internal/services/tasklog"

	"gorm.io/gorm"
)

type Service struct {
	db         *gorm.DB
	storage    storage.Driver
	downloader *downloader.HTTPDownloader
	logService *tasklog.Service
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
		logService: tasklog.NewService(db),
	}
}

// NewServiceWithDownloaderAndDriver 使用指定下载器和存储驱动创建资产服务
func NewServiceWithDownloaderAndDriver(db *gorm.DB, driver storage.Driver, dl *downloader.HTTPDownloader) *Service {
	return &Service{
		db:         db,
		storage:    driver,
		downloader: dl,
		logService: tasklog.NewService(db),
	}
}

func (s *Service) Download(ctx context.Context, assetID uint) (*DownloadResult, error) {
	return s.downloadWithAttempt(ctx, assetID, 1, 3)
}

func (s *Service) downloadWithAttempt(ctx context.Context, assetID uint, attempt int, maxAttempts int) (*DownloadResult, error) {
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
		MaxAttempts:  maxAttempts,
		Attempt:      attempt,
		StartedAt:    ptrTime(time.Now().UTC()),
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	// 记录日志
	_ = s.logService.Append(ctx, task.ID, "info",
		fmt.Sprintf("开始下载资产 %s (Release %s)", asset.Name, release.Tag))

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

	// 流式下载：直接写入存储，不在内存中缓存整个文件
	objectPath := buildObjectPath(repository, release, asset)

	// 使用 pipe 连接下载器和存储
	pr, pw := io.Pipe()
	var downloadResult *downloader.Result
	var downloadErr error
	var storedObject *storage.StoredObject
	var storageErr error

	// 下载 goroutine：从 HTTP 读取写入 pipe
	downloadDone := make(chan struct{})
	go func() {
		defer close(downloadDone)
		if downloadErr != nil {
			return
		}
		downloadResult, downloadErr = s.downloader.Download(ctx, downloadURL, token, pw)
		if downloadErr != nil {
			_ = pw.CloseWithError(downloadErr)
			return
		}
		_ = pw.Close()
	}()

	// 存储 goroutine：从 pipe 读取写入存储
	storageDone := make(chan struct{})
	go func() {
		defer close(storageDone)
		storedObject, storageErr = s.storage.Put(ctx, objectPath, pr)
		if storageErr != nil {
			pr.CloseWithError(storageErr)
		}
	}()

	// 等待两个 goroutine 完成
	<-downloadDone
	<-storageDone

	if downloadErr != nil {
		s.markAssetFailed(ctx, &asset, downloadErr)
		s.failTaskWithLog(ctx, &task, downloadErr, "下载请求失败")
		return nil, downloadErr
	}
	if storageErr != nil {
		s.markAssetFailed(ctx, &asset, storageErr)
		s.failTaskWithLog(ctx, &task, storageErr, "存储写入失败")
		return nil, storageErr
	}

	if err := s.storage.SetLatestTag(ctx, repository.Provider, repository.Owner, repository.Repo, release.Tag); err != nil {
		s.markAssetFailed(ctx, &asset, err)
		s.failTaskWithLog(ctx, &task, err, "设置 latest 标签失败")
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

	_ = s.logService.Append(ctx, task.ID, "info",
		fmt.Sprintf("下载完成: %s (%d bytes, SHA256: %s)", asset.Name, downloadResult.Size, shortSHA256(downloadResult.SHA256)))

	return &DownloadResult{
		Asset: asset,
		Task:  task,
	}, nil
}

// RetryDownload 带退避的重试下载
func (s *Service) RetryDownload(ctx context.Context, assetID uint) (*DownloadResult, error) {
	asset, _, _, err := s.loadAssetContext(ctx, assetID)
	if err != nil {
		return nil, err
	}

	// 查找现有失败的下载任务
	var existingTask models.Task
	err = s.db.WithContext(ctx).
		Where("asset_id = ? AND status = ?", assetID, models.TaskStatusFailed).
		Order("created_at DESC").
		First(&existingTask).Error

	attempt := 1
	if err == nil {
		attempt = existingTask.Attempt + 1
		if attempt > existingTask.MaxAttempts {
			return nil, fmt.Errorf("已达到最大重试次数 %d", existingTask.MaxAttempts)
		}
		// 退避：指数退避
		backoff := time.Duration(attempt*attempt) * time.Minute
		_ = s.logService.Append(ctx, existingTask.ID, "info",
			fmt.Sprintf("重试下载 (第 %d 次，退避 %v)", attempt, backoff))
	}

	// 重置资产状态为 pending 以便重新下载
	asset.Status = models.AssetStatusPending
	asset.ErrorMessage = ""
	_ = s.db.WithContext(ctx).Save(&asset).Error

	if existingTask.MaxAttempts <= 0 {
		existingTask.MaxAttempts = 3
	}
	return s.downloadWithAttempt(ctx, assetID, attempt, existingTask.MaxAttempts)
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

// Open 兼容旧接口
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

func (s *Service) failTaskWithLog(ctx context.Context, task *models.Task, err error, logMsg string) {
	s.failTask(ctx, task, err)
	_ = s.logService.Append(ctx, task.ID, "error", logMsg+": "+err.Error())
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

func shortSHA256(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) <= 16 {
		return value
	}
	return value[:16] + "..."
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
