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
	notifysvc "releasehub/backend/internal/services/notify"
	proxysvc "releasehub/backend/internal/services/proxy"
	"releasehub/backend/internal/services/storage"
	"releasehub/backend/internal/services/tasklog"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	db         *gorm.DB
	storage    storage.Driver
	storages   *storage.DriverFactory
	downloader *downloader.HTTPDownloader
	logService *tasklog.Service
	notifier   *notifysvc.Service
	logger     *zap.Logger
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

	service := NewServiceWithFactory(db, storageConfig)
	service.storage = localStorage
	service.logger = zap.NewNop()
	return service, nil
}

// NewServiceWithDriver 使用指定存储驱动创建资产服务
func NewServiceWithDriver(db *gorm.DB, driver storage.Driver) *Service {
	return &Service{
		db:         db,
		storage:    driver,
		downloader: downloader.NewHTTPDownloader(),
		logService: tasklog.NewService(db),
		notifier:   notifysvc.NewService(db),
		logger:     zap.NewNop(),
	}
}

// NewServiceWithDownloaderAndDriver 使用指定下载器和存储驱动创建资产服务
func NewServiceWithDownloaderAndDriver(db *gorm.DB, driver storage.Driver, dl *downloader.HTTPDownloader) *Service {
	return &Service{
		db:         db,
		storage:    driver,
		downloader: dl,
		logService: tasklog.NewService(db),
		notifier:   notifysvc.NewService(db),
		logger:     zap.NewNop(),
	}
}

func NewServiceWithFactory(db *gorm.DB, storageConfig config.StorageConfig) *Service {
	return &Service{
		db:         db,
		storages:   storage.NewDriverFactory(db, storageConfig),
		downloader: downloader.NewHTTPDownloader(),
		logService: tasklog.NewService(db),
		notifier:   notifysvc.NewService(db),
		logger:     zap.NewNop(),
	}
}

func (s *Service) Download(ctx context.Context, assetID uint) (*DownloadResult, error) {
	return s.downloadWithAttempt(ctx, assetID, 1, 3, nil)
}

// DownloadToStorage 下载资产到指定存储
func (s *Service) DownloadToStorage(ctx context.Context, assetID uint, targetStorageID uint) (*DownloadResult, error) {
	return s.downloadWithAttempt(ctx, assetID, 1, 3, &targetStorageID)
}

func (s *Service) downloadWithAttempt(ctx context.Context, assetID uint, attempt int, maxAttempts int, targetStorageID *uint) (*DownloadResult, error) {
	asset, release, repository, err := s.loadAssetContext(ctx, assetID)
	if err != nil {
		return nil, err
	}
	if asset.Status == models.AssetStatusSkipped {
		return nil, fmt.Errorf("资产已被过滤跳过，不能下载")
	}

	// 多存储下载：如果指定了目标存储，查找或创建该存储上的 asset 记录
	if targetStorageID != nil {
		// 查找该 release+name+storage 的记录
		var storageAsset models.Asset
		err := s.db.WithContext(ctx).Unscoped().
			Where("release_id = ? AND name = ? AND storage_id = ?", asset.ReleaseID, asset.Name, *targetStorageID).
			First(&storageAsset).Error

		if err == nil {
			// 如果记录已被软删除，先恢复它（清除 deleted_at）
			if storageAsset.DeletedAt.Valid {
				s.db.WithContext(ctx).Model(&storageAsset).Update("deleted_at", nil)
				storageAsset.DeletedAt = gorm.DeletedAt{}
			}
			// 恢复下载地址（软删除的旧记录可能缺少下载地址）
			if strings.TrimSpace(storageAsset.BrowserDownloadURL) == "" && strings.TrimSpace(storageAsset.DownloadURL) == "" {
				storageAsset.DownloadURL = asset.DownloadURL
				storageAsset.BrowserDownloadURL = asset.BrowserDownloadURL
				storageAsset.ProviderAssetID = asset.ProviderAssetID
				storageAsset.Size = asset.Size
				storageAsset.ContentType = asset.ContentType
				s.db.WithContext(ctx).Model(&storageAsset).Updates(map[string]any{
					"download_url":          asset.DownloadURL,
					"browser_download_url":  asset.BrowserDownloadURL,
					"provider_asset_id":     asset.ProviderAssetID,
					"size":                  asset.Size,
					"content_type":          asset.ContentType,
				})
			}
			// 该存储上已有记录
			if storageAsset.Status == models.AssetStatusVerified || storageAsset.Status == models.AssetStatusDownloaded {
				// 已完成下载，直接返回
				return &DownloadResult{Asset: storageAsset}, nil
			}
			// 需要继续下载，使用这条记录
			asset = storageAsset
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			// 该存储上没有记录，创建新的 asset 记录（复制模板数据）
			newAsset := models.Asset{
				ReleaseID:          asset.ReleaseID,
				ProviderAssetID:    asset.ProviderAssetID,
				Name:               asset.Name,
				Size:               asset.Size,
				ContentType:        asset.ContentType,
				DownloadURL:        asset.DownloadURL,
				BrowserDownloadURL: asset.BrowserDownloadURL,
				StorageID:          targetStorageID,
				Status:             models.AssetStatusPending,
			}
			if err := s.db.WithContext(ctx).Create(&newAsset).Error; err != nil {
				return nil, fmt.Errorf("创建存储资产记录失败: %w", err)
			}
			asset = newAsset
		}
		// asset 可能被替换，但 release/repository 不变
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
	var storageDriver storage.Driver
	var storageID uint
	if targetStorageID != nil {
		// 指定存储：直接通过 storage ID 创建驱动
		if s.storages != nil {
			var storageModel models.Storage
			if err := s.storages.DB().WithContext(ctx).First(&storageModel, *targetStorageID).Error; err != nil {
				s.failTask(ctx, &task, err)
				return nil, err
			}
			storageDriver, err = storage.NewDriverFromModel(storageModel)
			if err != nil {
				s.failTask(ctx, &task, err)
				return nil, err
			}
			storageID = *targetStorageID
		} else {
			d, id, e := s.storageDriverAndID(ctx, repository)
			if e != nil {
				s.failTask(ctx, &task, e)
				return nil, e
			}
			storageDriver, storageID = d, id
		}
	} else {
		// 未指定存储：使用仓库配置的默认存储
		var e error
		storageDriver, storageID, e = s.storageDriverAndID(ctx, repository)
		if e != nil {
			s.failTask(ctx, &task, e)
			return nil, e
		}
	}
	downloadClient, err := s.downloaderForRepository(ctx, repository)
	if err != nil {
		s.failTask(ctx, &task, err)
		return nil, err
	}

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
		downloadResult, downloadErr = downloadClient.Download(ctx, downloadURL, token, pw)
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
		storedObject, storageErr = storageDriver.Put(ctx, objectPath, pr)
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
		s.notifyDownloadFailed(ctx, repository, release, asset, downloadErr)
		return nil, downloadErr
	}
	if storageErr != nil {
		s.markAssetFailed(ctx, &asset, storageErr)
		s.failTaskWithLog(ctx, &task, storageErr, "存储写入失败")
		s.notifyDownloadFailed(ctx, repository, release, asset, storageErr)
		return nil, storageErr
	}

	// 仅当该 Release 是最新版本时才更新 latest 标签，避免同步历史版本时覆盖
	if release.IsLatest {
		if err := storageDriver.SetLatestTag(ctx, repository.Provider, repository.Owner, repository.Repo, release.Tag); err != nil {
			s.logger.Warn("设置 latest 标签失败，不影响下载结果",
				zap.String("repo", fmt.Sprintf("%s/%s", repository.Owner, repository.Repo)),
				zap.String("tag", release.Tag),
				zap.Error(err))
		}
	}

	now := time.Now().UTC()
	asset.StoragePath = storedObject.Path
	asset.Size = downloadResult.Size
	asset.SHA256 = downloadResult.SHA256
	asset.Status = models.AssetStatusVerified
	asset.StorageID = &storageID
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
	s.notifyDownloadOK(ctx, repository, release, asset)

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
	return s.downloadWithAttempt(ctx, assetID, attempt, existingTask.MaxAttempts, nil)
}

// OpenReader 返回资产的读取流（兼容所有存储驱动）
func (s *Service) OpenReader(ctx context.Context, assetID uint) (*models.Asset, *storage.StoredObject, io.ReadCloser, error) {
	asset, _, repository, err := s.loadAssetContext(ctx, assetID)
	if err != nil {
		return nil, nil, nil, err
	}
	if strings.TrimSpace(asset.StoragePath) == "" {
		return nil, nil, nil, fmt.Errorf("资产尚未下载")
	}

	// 优先用 Asset 自身的 StorageID 确定存储驱动
	storageDriver, err := s.driverForAsset(ctx, &asset, repository)
	if err != nil {
		return nil, nil, nil, err
	}

	reader, object, err := storageDriver.Open(ctx, asset.StoragePath)
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
	asset, _, repository, err := s.loadAssetContext(ctx, assetID)
	if err != nil {
		return err
	}

	if strings.TrimSpace(asset.StoragePath) != "" {
		// 优先用 Asset 自身的 StorageID 确定存储驱动
		storageDriver, err := s.driverForAsset(ctx, &asset, repository)
		if err != nil {
			return err
		}
		if err := storageDriver.Delete(ctx, asset.StoragePath); err != nil {
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

	release, err := s.loadRelease(ctx, asset.ReleaseID)
	if err != nil {
		return asset, models.Release{}, models.Repository{}, err
	}

	repository, err := s.loadRepository(ctx, release.RepositoryID)
	if err != nil {
		return asset, release, models.Repository{}, err
	}

	return asset, release, repository, nil
}

func (s *Service) loadRelease(ctx context.Context, releaseID uint) (models.Release, error) {
	var release models.Release
	if err := s.db.WithContext(ctx).First(&release, releaseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return release, fmt.Errorf("Release 不存在")
		}
		return release, err
	}
	return release, nil
}

func (s *Service) loadRepository(ctx context.Context, repositoryID uint) (models.Repository, error) {
	var repository models.Repository
	if err := s.db.WithContext(ctx).First(&repository, repositoryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return repository, fmt.Errorf("仓库不存在")
		}
		return repository, err
	}
	return repository, nil
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

// storageDriverAndID 返回仓库对应的存储驱动和存储 ID
func (s *Service) storageDriverAndID(ctx context.Context, repository models.Repository) (storage.Driver, uint, error) {
	if s.storages != nil {
		return s.storages.DriverAndStorageID(ctx, repository)
	}
	return s.storage, 0, nil
}

// storageDriver 仅返回驱动（兼容旧调用）
func (s *Service) storageDriver(ctx context.Context, repository models.Repository) (storage.Driver, error) {
	d, _, err := s.storageDriverAndID(ctx, repository)
	return d, err
}
// driverForAsset 根据资产的 StorageID 确定存储驱动
// 优先使用 Asset 自身的 StorageID（文件实际所在存储），回退到仓库配置
func (s *Service) driverForAsset(ctx context.Context, asset *models.Asset, repository models.Repository) (storage.Driver, error) {
	if asset.StorageID != nil && *asset.StorageID > 0 {
		if s.storages != nil {
			var storageModel models.Storage
			if err := s.storages.DB().WithContext(ctx).First(&storageModel, *asset.StorageID).Error; err == nil {
				return storage.NewDriverFromModel(storageModel)
			}
		}
	}
	return s.storageDriver(ctx, repository)
}


func (s *Service) downloaderForRepository(ctx context.Context, repository models.Repository) (*downloader.HTTPDownloader, error) {
	if repository.ProxyID == nil {
		return s.downloader, nil
	}

	transport, err := proxysvc.TransportForRepository(ctx, s.db, repository)
	if err != nil {
		return nil, err
	}
	return downloader.NewHTTPDownloaderWithTransport(transport), nil
}

func (s *Service) notifyDownloadOK(ctx context.Context, repository models.Repository, release models.Release, asset models.Asset) {
	if s.notifier == nil {
		return
	}
	title := fmt.Sprintf("ReleaseHub 下载完成: %s/%s", repository.Owner, repository.Repo)
	message := fmt.Sprintf("版本: %s\n资产: %s\nSHA256: %s", release.Tag, asset.Name, shortSHA256(asset.SHA256))
	_ = s.notifier.Notify(ctx, notifysvc.EventDownloadOK, title, message)
}

func (s *Service) notifyDownloadFailed(ctx context.Context, repository models.Repository, release models.Release, asset models.Asset, err error) {
	if s.notifier == nil {
		return
	}
	title := fmt.Sprintf("ReleaseHub 下载失败: %s/%s", repository.Owner, repository.Repo)
	message := fmt.Sprintf("版本: %s\n资产: %s\n错误: %s", release.Tag, asset.Name, err.Error())
	_ = s.notifier.Notify(ctx, notifysvc.EventDownloadErr, title, message)
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
