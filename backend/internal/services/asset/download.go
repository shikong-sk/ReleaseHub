package asset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	stdsync "sync"
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

	// 运行时下载进度表：assetID → {downloaded, total, updatedAt}
	// 仅存内存，不持久化；task list API 读取注入响应供前端展示真实进度
	progressMu  stdsync.RWMutex
	progressMap map[uint]DownloadProgress

	// 运行时取消表：taskID → cancelFunc，用于中断正在进行中的 HTTP 下载
	cancelMu  stdsync.Mutex
	cancelMap map[uint]context.CancelFunc
}

// DownloadProgress 单次下载的实时进度
type DownloadProgress struct {
	Downloaded int64 // 已下载字节
	Total      int64 // 总字节（未知为 0）
	UpdatedAt  time.Time
}

// SetProgress 设置资产下载进度（下载回调触发，节流到每 1MB 或 2秒一次）
func (s *Service) SetProgress(assetID uint, downloaded, total int64) {
	s.progressMu.Lock()
	if s.progressMap == nil {
		s.progressMap = make(map[uint]DownloadProgress)
	}
	s.progressMap[assetID] = DownloadProgress{
		Downloaded: downloaded,
		Total:      total,
		UpdatedAt:  time.Now().UTC(),
	}
	s.progressMu.Unlock()
}

// ClearProgress 下载结束（成功/失败）后清除进度
func (s *Service) ClearProgress(assetID uint) {
	s.progressMu.Lock()
	delete(s.progressMap, assetID)
	s.progressMu.Unlock()
}

// GetProgress 返回资产当前下载进度，未在下载中返回 false
func (s *Service) GetProgress(assetID uint) (DownloadProgress, bool) {
	s.progressMu.RLock()
	p, ok := s.progressMap[assetID]
	s.progressMu.RUnlock()
	return p, ok
}

// ActiveProgresses 返回所有正在下载的资产进度快照（供 task list API 注入）
func (s *Service) ActiveProgresses() map[uint]DownloadProgress {
	s.progressMu.RLock()
	defer s.progressMu.RUnlock()
	out := make(map[uint]DownloadProgress, len(s.progressMap))
	for k, v := range s.progressMap {
		out[k] = v
	}
	return out
}

// CancelDownloadTask 取消正在进行的下载任务
// 对 running 任务：调用 cancelFunc 中断 HTTP 下载，downloadWithAttempt 检测 ctx 取消后标记 task 为 canceled
// 对 pending 任务：直接在 DB 更新为 canceled（worker 取出时跳过执行）
func (s *Service) CancelDownloadTask(ctx context.Context, taskID uint) error {
	var task models.Task
	if err := s.db.WithContext(ctx).First(&task, taskID).Error; err != nil {
		return fmt.Errorf("查找任务失败: %w", err)
	}
	if task.Status != models.TaskStatusRunning && task.Status != models.TaskStatusPending {
		return fmt.Errorf("任务状态 %s 不支持取消", task.Status)
	}
	// running 任务：先中断 HTTP 下载，downloadWithAttempt 的 context.Canceled 分支会更新 DB
	s.cancelMu.Lock()
	cancel, ok := s.cancelMap[taskID]
	s.cancelMu.Unlock()
	if ok {
		cancel()
		// 等待 downloadWithAttempt 处理取消（它会更新 DB），给一点时间让 goroutine 完成
		// 不阻塞调用方太久，DB 状态最终会由 downloadWithAttempt 更新
		return nil
	}
	// pending 任务（或 running 但尚未注册 cancel）：直接更新 DB
	now := time.Now().UTC()
	return s.db.WithContext(ctx).Model(&models.Task{}).Where("id = ?", taskID).
		Updates(map[string]any{
			"status":        models.TaskStatusCanceled,
			"finished_at":   &now,
			"error_message": "任务已被手动取消",
		}).Error
}

// registerCancel 注册下载取消函数
func (s *Service) registerCancel(taskID uint, cancel context.CancelFunc) {
	s.cancelMu.Lock()
	if s.cancelMap == nil {
		s.cancelMap = make(map[uint]context.CancelFunc)
	}
	s.cancelMap[taskID] = cancel
	s.cancelMu.Unlock()
}

// unregisterCancel 注销下载取消函数
func (s *Service) unregisterCancel(taskID uint) {
	s.cancelMu.Lock()
	delete(s.cancelMap, taskID)
	s.cancelMu.Unlock()
}

// progressThrottle 进度回调节流器：每 threshold 字节或 2 秒触发一次 SetProgress
// 避免高频回调（每个 64KB buffer 触发）造成锁竞争
type progressThrottle struct {
	assetID    uint
	setFn      func(uint, int64, int64)
	mu         stdsync.Mutex
	lastBytes  int64
	lastTime   time.Time
	threshold  int64
}

// callback 下载进度回调，节流后写入运行时进度表
func (p *progressThrottle) callback(downloaded int64, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// 首次、每 threshold 字节、或距上次 ≥ 2 秒、或下载完成时写入
	if p.lastBytes == 0 || downloaded-p.lastBytes >= p.threshold ||
		time.Since(p.lastTime) >= 2*time.Second || (total > 0 && downloaded >= total) {
		p.setFn(p.assetID, downloaded, total)
		p.lastBytes = downloaded
		p.lastTime = time.Now()
	}
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
		err := s.db.WithContext(ctx).Where("release_id = ? AND name = ? AND storage_id = ?", asset.ReleaseID, asset.Name, *targetStorageID).
			First(&storageAsset).Error

		if err == nil {
			// 恢复下载地址（旧记录可能缺少下载地址）
			if strings.TrimSpace(storageAsset.BrowserDownloadURL) == "" && strings.TrimSpace(storageAsset.DownloadURL) == "" {
				storageAsset.DownloadURL = asset.DownloadURL
				storageAsset.BrowserDownloadURL = asset.BrowserDownloadURL
				storageAsset.ProviderAssetID = asset.ProviderAssetID
				storageAsset.Size = asset.Size
				storageAsset.ContentType = asset.ContentType
				s.db.WithContext(ctx).Model(&storageAsset).Updates(map[string]any{
					"download_url":         asset.DownloadURL,
					"browser_download_url": asset.BrowserDownloadURL,
					"provider_asset_id":    asset.ProviderAssetID,
					"size":                 asset.Size,
					"content_type":         asset.ContentType,
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

	// 创建可取消 ctx，注册到 cancelMap 供 CancelDownloadTask 中断 HTTP 下载
	dlCtx, cancelDownload := context.WithCancel(ctx)
	s.registerCancel(task.ID, cancelDownload)
	defer s.unregisterCancel(task.ID)
	defer cancelDownload() // 下载结束后释放 context

	// 进度回调：节流到每 1MB 或每 2秒一次，避免高频锁竞争
	progressCtx := &progressThrottle{
		assetID:    asset.ID,
		setFn:      s.SetProgress,
		lastBytes:  0,
		lastTime:   time.Now(),
		threshold:  1024 * 1024, // 1MB
	}

	// 下载 goroutine：从 HTTP 读取写入 pipe（带进度回调）
	downloadDone := make(chan struct{})
	go func() {
		defer close(downloadDone)
		if downloadErr != nil {
			return
		}
		downloadResult, downloadErr = downloadClient.DownloadWithProgress(dlCtx, downloadURL, token, pw, progressCtx.callback)
		if downloadErr != nil {
			_ = pw.CloseWithError(downloadErr)
			return
		}
		_ = pw.Close()
	}()

	// 存储完成或下载失败后清除进度记录
	defer s.ClearProgress(asset.ID)

	// 存储 goroutine：从 pipe 读取写入存储
	storageDone := make(chan struct{})
	go func() {
		defer close(storageDone)
		storedObject, storageErr = storageDriver.Put(dlCtx, objectPath, pr)
		if storageErr != nil {
			pr.CloseWithError(storageErr)
		}
	}()

	// 等待两个 goroutine 完成
	<-downloadDone
	<-storageDone

	if downloadErr != nil {
		// 用户手动取消：标记为 canceled 而非 failed，资产恢复 pending 便于重试
		if errors.Is(downloadErr, context.Canceled) || dlCtx.Err() == context.Canceled {
			now := time.Now().UTC()
			asset.Status = models.AssetStatusPending
			asset.ErrorMessage = "下载已取消"
			_ = s.db.WithContext(ctx).Save(&asset).Error
			task.Status = models.TaskStatusCanceled
			task.FinishedAt = &now
			task.ErrorMessage = "任务已被手动取消"
			_ = s.db.WithContext(ctx).Save(&task).Error
			_ = s.logService.Append(ctx, task.ID, "info", "任务已被手动取消")
			return nil, fmt.Errorf("任务已被手动取消")
		}
		s.markAssetFailed(ctx, &asset, downloadErr)
		s.failTaskWithLog(ctx, &task, downloadErr, "下载请求失败")
		s.notifyDownloadFailed(ctx, repository, release, asset, downloadErr)
		return nil, downloadErr
	}
	if storageErr != nil {
		// 用户手动取消同样处理
		if errors.Is(storageErr, context.Canceled) || dlCtx.Err() == context.Canceled {
			now := time.Now().UTC()
			asset.Status = models.AssetStatusPending
			asset.ErrorMessage = "下载已取消"
			_ = s.db.WithContext(ctx).Save(&asset).Error
			task.Status = models.TaskStatusCanceled
			task.FinishedAt = &now
			task.ErrorMessage = "任务已被手动取消"
			_ = s.db.WithContext(ctx).Save(&task).Error
			_ = s.logService.Append(ctx, task.ID, "info", "任务已被手动取消")
			return nil, fmt.Errorf("任务已被手动取消")
		}
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
	asset.DownloadBytes = downloadResult.Size
	asset.SHA256 = downloadResult.SHA256
	asset.StorageID = &storageID
	asset.ErrorMessage = ""
	asset.DownloadedAt = &now

	// SHA256 远程比对：expected_sha256 存在时与实际校验和比对
	if asset.ExpectedSHA256 != "" && asset.SHA256 != asset.ExpectedSHA256 {
		asset.Status = models.AssetStatusFailed
		asset.ErrorMessage = fmt.Sprintf("SHA256 不匹配: 实际=%s 期望=%s", shortSHA256(asset.SHA256), shortSHA256(asset.ExpectedSHA256))
		task.Status = models.TaskStatusFailed
		task.FinishedAt = &now
		task.ErrorMessage = asset.ErrorMessage

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

		_ = s.logService.Append(ctx, task.ID, "error", asset.ErrorMessage)
		s.notifyDownloadFailed(ctx, repository, release, asset, fmt.Errorf("%s", asset.ErrorMessage))
		return nil, fmt.Errorf("%s", asset.ErrorMessage)
	}

	asset.Status = models.AssetStatusVerified

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
