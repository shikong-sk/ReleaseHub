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
	notifysvc "releasehub/backend/internal/services/notify"
	releasesvc "releasehub/backend/internal/services/release"
	repositorysvc "releasehub/backend/internal/services/repository"
	tasklogsvc "releasehub/backend/internal/services/tasklog"

	"gorm.io/gorm"
)

const (
	defaultMaxConcurrentDownloads = 3
	// 任务队列默认并发执行数（任务级串行/有限并发，区别于单任务内的资产下载并发）
	defaultMaxConcurrentTasks = 2
	// 任务队列缓冲大小
	taskQueueBufferSize = 256
	// worker 数量上限（实际并发由 taskTokens 控制，worker 数固定为上限避免频繁启停）
	maxWorkerCount = 16
	// 信号量容量上限（固定，运行时通过令牌计数调整实际并发，不替换 channel）
	maxSemaphoreCapacity = 64
)

// taskJob 描述一个待执行的后台任务
type taskJob struct {
	taskID       uint
	repositoryID uint
	tag          string // 非空表示同步指定 tag
	assetID      uint   // 非零表示重试资产下载
	kind         string // "sync_latest" | "sync_by_tag" | "retry_asset"
}

type Service struct {
	db            *gorm.DB
	checker       *releasesvc.CheckService
	assetService  *assetsvc.Service
	notifier      *notifysvc.Service
	logService    *tasklogsvc.Service
	storageConfig config.StorageConfig

	// 并发控制（运行时可调）
	mu                     stdsync.RWMutex
	maxConcurrentDownloads int
	maxConcurrentTasks     int

	// 任务队列：worker pool 实现任务级排队，避免所有任务并发执行
	taskQueue    chan taskJob
	workerWG     stdsync.WaitGroup
	workersOnce  stdsync.Once
	workersStop  chan struct{}
	stopOnce     stdsync.Once

	// 动态并发限制：用条件变量 + 计数器实现，运行时调整 maxConcurrentTasks 立即生效，无 channel 替换死锁风险
	concurMu    stdsync.Mutex
	concurCond  *stdsync.Cond
	inFlight    int // 当前正在执行的任务数
	maxInFlight int // 当前允许的最大并发数（= maxConcurrentTasks)
}

// startWorkers 启动固定数量的 worker goroutine 消费任务队列
// 使用 sync.Once 保证只启动一次（首次 Enqueue 时懒启动）
func (s *Service) startWorkers() {
	s.workersOnce.Do(func() {
		s.taskQueue = make(chan taskJob, taskQueueBufferSize)
		s.workersStop = make(chan struct{})
		s.concurCond = stdsync.NewCond(&s.concurMu)
		s.concurMu.Lock()
		s.maxInFlight = s.maxConcurrentTasks
		if s.maxInFlight < 1 {
			s.maxInFlight = 1
		}
		s.concurMu.Unlock()
		for i := 0; i < maxWorkerCount; i++ {
			s.workerWG.Add(1)
			go s.taskWorker(i)
		}
	})
}

// taskWorker 消费任务队列并执行任务，通过条件变量限制并发
func (s *Service) taskWorker(id int) {
	defer s.workerWG.Done()
	for {
		select {
		case <-s.workersStop:
			return
		case job, ok := <-s.taskQueue:
			if !ok {
				return
			}
			if !s.acquireSlot() {
				// 服务停止，放回任务不执行
				return
			}
			s.executeJob(job)
			s.releaseSlot()
		}
	}
}

// acquireSlot 等待直到当前执行任务数低于 maxInFlight，或在服务停止时退出
// 返回 true 表示获取成功，false 表示服务已停止应退出
func (s *Service) acquireSlot() bool {
	s.concurMu.Lock()
	for s.inFlight >= s.maxInFlight {
		select {
		case <-s.workersStop:
			s.concurMu.Unlock()
			return false
		default:
		}
		s.concurCond.Wait()
		// 被唤醒后再次检查是否已停止
		select {
		case <-s.workersStop:
			s.concurMu.Unlock()
			return false
		default:
		}
	}
	s.inFlight++
	s.concurMu.Unlock()
	return true
}

// releaseSlot 释放一个执行槽位并唤醒等待的 worker
func (s *Service) releaseSlot() {
	s.concurMu.Lock()
	s.inFlight--
	s.concurCond.Signal()
	s.concurMu.Unlock()
}

// UpdateMaxConcurrentTasks 运行时调整任务并发数
// 仅更新 maxInFlight 并广播唤醒，新值对后续 acquire 立即生效，无 channel 替换死锁风险
func (s *Service) UpdateMaxConcurrentTasks(n int) {
	if n < 1 {
		n = 1
	}
	if n > maxWorkerCount {
		n = maxWorkerCount
	}
	s.startWorkers()
	s.concurMu.Lock()
	if n > s.maxInFlight {
		// 放宽上限：唤醒可能因上限阻塞的 worker
		s.maxInFlight = n
		s.concurCond.Broadcast()
	} else if n < s.maxInFlight {
		s.maxInFlight = n
	}
	s.concurMu.Unlock()
	// 同步更新对外可见的 maxConcurrentTasks（用于 MaxConcurrentTasks() 查询）
	s.mu.Lock()
	s.maxConcurrentTasks = n
	s.mu.Unlock()
}

// UpdateMaxConcurrentDownloads 运行时调整单任务内资产下载并发数
func (s *Service) UpdateMaxConcurrentDownloads(n int) {
	if n < 1 {
		n = 1
	}
	s.mu.Lock()
	s.maxConcurrentDownloads = n
	s.mu.Unlock()
}

// MaxConcurrentTasks 返回当前任务并发数
func (s *Service) MaxConcurrentTasks() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxConcurrentTasks
}

// MaxConcurrentDownloads 返回当前资产下载并发数
func (s *Service) MaxConcurrentDownloads() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxConcurrentDownloads
}

// ActiveProgresses 返回所有正在下载的资产进度快照（供 task list API 注入前端展示真实进度）
func (s *Service) ActiveProgresses() map[uint]assetsvc.DownloadProgress {
	return s.assetService.ActiveProgresses()
}

// Stop 停止 worker pool，等待在途任务完成
func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		s.mu.RLock()
		stopCh := s.workersStop
		s.mu.RUnlock()
		if stopCh != nil {
			close(stopCh)
			// 唤醒所有在 acquireSlot 中 cond.Wait 的 worker，使其检查 workersStop 并退出
			s.concurMu.Lock()
			s.concurCond.Broadcast()
			s.concurMu.Unlock()
			s.workerWG.Wait()
		}
	})
}

// executeJob 根据 job 类型分发到对应的执行函数
func (s *Service) executeJob(job taskJob) {
	bgCtx := context.Background()
	// 重新加载任务记录（可能在排队期间状态已变）
	var task models.Task
	if err := s.db.WithContext(bgCtx).First(&task, job.taskID).Error; err != nil {
		return
	}
	// 已被取消的任务跳过
	if task.Status == models.TaskStatusCanceled {
		return
	}
	switch job.kind {
	case "sync_latest":
		s.executeSyncRepository(bgCtx, job.repositoryID, task)
	case "sync_by_tag":
		s.executeSyncByTag(bgCtx, job.repositoryID, job.tag, task)
	case "retry_asset":
		s.executeRetryAsset(bgCtx, job.assetID, task)
	}
}

// enqueueJob 将任务投递到队列，队列满时记录警告但不阻塞调用方
func (s *Service) enqueueJob(job taskJob) {
	s.startWorkers()
	select {
	case s.taskQueue <- job:
	default:
		// 队列满：标记任务失败，避免无限堆积
		s.appendLog(context.Background(), job.taskID, "warn", "任务队列已满，任务未能入队执行")
		_ = s.db.Model(&models.Task{}).Where("id = ?", job.taskID).
			Updates(map[string]any{
				"status":        models.TaskStatusFailed,
				"error_message": "任务队列已满",
				"finished_at":   time.Now().UTC(),
			}).Error
	}
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
		notifier:               notifysvc.NewService(db),
		logService:             tasklogsvc.NewService(db),
		maxConcurrentDownloads: defaultMaxConcurrentDownloads,
		maxConcurrentTasks:     defaultMaxConcurrentTasks,
		storageConfig:          storageConfig,
	}, nil
}

// EnqueueSyncRepository 异步同步最新 Release：创建任务记录后立即返回，后台执行同步
func (s *Service) EnqueueSyncRepository(ctx context.Context, repositoryID uint) (*models.Task, error) {
	task := models.Task{
		Type:         "sync_release",
		RepositoryID: &repositoryID,
		Status:       models.TaskStatusPending,
		MaxAttempts:  1,
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("已加入队列: 同步仓库 (ID: %d) 最新版本", repositoryID))

	s.enqueueJob(taskJob{
		taskID:       task.ID,
		repositoryID: repositoryID,
		kind:         "sync_latest",
	})

	return &task, nil
}

// EnqueueSyncByTag 异步同步指定 tag 的 Release：创建任务记录后立即返回，后台执行同步
func (s *Service) EnqueueSyncByTag(ctx context.Context, repositoryID uint, tag string) (*models.Task, error) {
	task := models.Task{
		Type:         "sync_release",
		RepositoryID: &repositoryID,
		Status:       models.TaskStatusPending,
		MaxAttempts:  1,
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("已加入队列: 同步仓库 (ID: %d) 指定版本 %s", repositoryID, tag))

	s.enqueueJob(taskJob{
		taskID:       task.ID,
		repositoryID: repositoryID,
		tag:          tag,
		kind:         "sync_by_tag",
	})

	return &task, nil
}

// EnqueueRetryAsset 异步重试失败资产的下载
func (s *Service) EnqueueRetryAsset(ctx context.Context, assetID uint) (*models.Task, error) {
	task := models.Task{
		Type:        "download_asset",
		Status:      models.TaskStatusPending,
		MaxAttempts: 3,
	}
	// 查找资产所属的 Release 和 Repository
	var asset models.Asset
	if err := s.db.WithContext(ctx).First(&asset, assetID).Error; err != nil {
		return nil, err
	}
	var release models.Release
	if err := s.db.WithContext(ctx).First(&release, asset.ReleaseID).Error; err != nil {
		return nil, err
	}
	task.RepositoryID = &release.RepositoryID
	task.ReleaseID = &release.ID
	task.AssetID = &assetID

	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("已加入队列: 重试下载资产 %s (ID: %d)", asset.Name, assetID))

	s.enqueueJob(taskJob{
		taskID:  task.ID,
		assetID: assetID,
		kind:    "retry_asset",
	})

	return &task, nil
}

// executeRetryAsset 后台执行重试资产下载
func (s *Service) executeRetryAsset(ctx context.Context, assetID uint, task models.Task) {
	task.Status = models.TaskStatusRunning
	task.StartedAt = ptrTime(time.Now().UTC())
	_ = s.db.WithContext(ctx).Save(&task).Error

	// 重置资产状态为 pending
	var asset models.Asset
	if err := s.db.WithContext(ctx).First(&asset, assetID).Error; err != nil {
		s.failTaskWithLog(ctx, &task, err, "查找资产失败")
		return
	}
	asset.Status = models.AssetStatusPending
	asset.ErrorMessage = ""
	_ = s.db.WithContext(ctx).Save(&asset).Error

	// 确定存储：使用资产自身的 StorageID 或回退到仓库配置
	var release models.Release
	if err := s.db.WithContext(ctx).First(&release, asset.ReleaseID).Error; err != nil {
		s.failTaskWithLog(ctx, &task, err, "查找 Release 失败")
		return
	}

	var targetStorageID *uint
	if asset.StorageID != nil && *asset.StorageID > 0 {
		targetStorageID = asset.StorageID
	} else {
		repoSvc := repositorysvc.NewService(s.db, s.storageConfig)
		storageIDs, err := repoSvc.GetRepositoryStorages(ctx, release.RepositoryID)
		if err != nil || len(storageIDs) == 0 {
			s.failTaskWithLog(ctx, &task, fmt.Errorf("无法确定存储目标"), "获取仓库存储配置失败")
			return
		}
		first := storageIDs[0]
		targetStorageID = &first
	}

	result, err := s.assetService.DownloadToStorage(ctx, assetID, *targetStorageID)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, fmt.Sprintf("重试下载资产 %s 失败", asset.Name))
		return
	}

	now := time.Now().UTC()
	task.FinishedAt = &now
	task.Status = models.TaskStatusSucceeded
	_ = s.db.WithContext(ctx).Save(&task).Error

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("重试下载完成: %s", result.Asset.Name))
}

// executeSyncRepository 后台执行同步最新版本
// 流程：检查最新版本 → 持久化新资产为 pending → 下载 pending 资产
func (s *Service) executeSyncRepository(ctx context.Context, repositoryID uint, task models.Task) {
	task.Status = models.TaskStatusRunning
	task.StartedAt = ptrTime(time.Now().UTC())
	_ = s.db.WithContext(ctx).Save(&task).Error

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("开始同步仓库 (ID: %d)", repositoryID))

	// 步骤1：检查最新版本并持久化资产元数据
	checkResult, err := s.checker.CheckLatest(ctx, repositoryID)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "检查最新 Release 失败")
		s.notifySyncFailed(ctx, repositoryID, err)
		return
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("检查完成: %s/%s 版本 %s",
		checkResult.Repository.Owner, checkResult.Repository.Repo, checkResult.Release.Tag))

	task.ReleaseID = &checkResult.Release.ID
	_ = s.db.WithContext(ctx).Save(&task).Error

	// 步骤2：从数据库重新查询该 release 下所有 pending/failed 资产
	// CheckLatest 已把新资产设为 pending，这里查询确保拿到最新状态
	var assetsToDownload []models.Asset
	if err := s.db.WithContext(ctx).
		Where("release_id = ? AND status IN ?", checkResult.Release.ID,
			[]models.AssetStatus{models.AssetStatusPending, models.AssetStatusFailed}).
		Find(&assetsToDownload).Error; err != nil {
		s.failTaskWithLog(ctx, &task, err, "查询待下载资产失败")
		return
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("待下载资产 %d 个", len(assetsToDownload)))

	if len(assetsToDownload) == 0 {
		now := time.Now().UTC()
		task.FinishedAt = &now
		task.Status = models.TaskStatusSucceeded
		_ = s.db.WithContext(ctx).Save(&task).Error
		s.appendLog(ctx, task.ID, "info", "同步完成: 无待下载资产")
		// 无变化不发通知
		return
	}

	// 步骤3：获取存储配置并下载
	repoSvc := repositorysvc.NewService(s.db, s.storageConfig)
	storageIDs, err := repoSvc.GetRepositoryStorages(ctx, repositoryID)
	if err != nil || len(storageIDs) == 0 {
		errMsg := fmt.Errorf("仓库没有配置存储目标")
		s.failTaskWithLog(ctx, &task, errMsg, "获取仓库存储配置失败")
		s.notifySyncFailed(ctx, repositoryID, errMsg)
		return
	}

	downloadResults, failedAssets := s.downloadAssetsToStorages(ctx, assetsToDownload, storageIDs)
	s.cleanupOrphanTemplateAssets(ctx, checkResult.Release.ID)

	if len(downloadResults) > 0 {
		s.appendLog(ctx, task.ID, "info", fmt.Sprintf("已下载 %d 个资产", len(downloadResults)))
	}
	if len(failedAssets) > 0 {
		s.appendLog(ctx, task.ID, "warn", fmt.Sprintf("%d 个资产下载失败: %s", len(failedAssets), joinAssetErrors(failedAssets)))
	}

	now := time.Now().UTC()
	task.FinishedAt = &now
	if len(failedAssets) > 0 {
		task.Status = models.TaskStatusFailed
		task.ErrorMessage = joinAssetErrors(failedAssets)
		_ = s.db.WithContext(ctx).Save(&task).Error
		s.notifySyncFailed(ctx, repositoryID, errors.New(task.ErrorMessage))
		return
	}

	task.Status = models.TaskStatusSucceeded
	_ = s.db.WithContext(ctx).Save(&task).Error
	// 有变化才通知：新版本 或 成功下载了资产
	if checkResult.IsNewRelease || len(downloadResults) > 0 {
		s.notifySyncSuccess(ctx, checkResult.Repository, checkResult.Release, len(downloadResults))
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("同步完成: %s/%s 版本 %s，下载 %d 个资产",
		checkResult.Repository.Owner, checkResult.Repository.Repo, checkResult.Release.Tag, len(downloadResults)))
}

// executeSyncByTag 后台执行同步指定版本
func (s *Service) executeSyncByTag(ctx context.Context, repositoryID uint, tag string, task models.Task) {
	task.Status = models.TaskStatusRunning
	task.StartedAt = ptrTime(time.Now().UTC())
	_ = s.db.WithContext(ctx).Save(&task).Error

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("开始同步仓库 (ID: %d) 指定版本 %s", repositoryID, tag))

	checkResult, err := s.checker.CheckByTag(ctx, repositoryID, tag)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, fmt.Sprintf("检查指定版本 %s 失败", tag))
		s.notifySyncFailed(ctx, repositoryID, err)
		return
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("检查完成: %s/%s 版本 %s",
		checkResult.Repository.Owner, checkResult.Repository.Repo, checkResult.Release.Tag))

	task.ReleaseID = &checkResult.Release.ID
	_ = s.db.WithContext(ctx).Save(&task).Error

	assetsToDownload := downloadableAssets(checkResult.Assets)
	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("待下载资产 %d 个（跳过 %d 个）",
		len(assetsToDownload), len(checkResult.Assets)-len(assetsToDownload)))

	// 为每个配置的存储目标分别下载
	repoSvc := repositorysvc.NewService(s.db, s.storageConfig)
	storageIDs, err := repoSvc.GetRepositoryStorages(ctx, repositoryID)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "获取仓库存储配置失败")
		s.notifySyncFailed(ctx, repositoryID, err)
		return
	}
	if len(storageIDs) == 0 {
		s.failTaskWithLog(ctx, &task, fmt.Errorf("仓库没有配置存储目标"), "仓库没有配置存储目标")
		s.notifySyncFailed(ctx, repositoryID, fmt.Errorf("仓库没有配置存储目标"))
		return
	}

	downloadResults, failedAssets := s.downloadAssetsToStorages(ctx, assetsToDownload, storageIDs)

	// 清理 checker 创建的 StorageID=NULL 模板记录，避免多存储场景下文件树重复
	s.cleanupOrphanTemplateAssets(ctx, checkResult.Release.ID)

	if len(downloadResults) > 0 {
		s.appendLog(ctx, task.ID, "info", fmt.Sprintf("已下载 %d 个资产", len(downloadResults)))
	}
	if len(failedAssets) > 0 {
		s.appendLog(ctx, task.ID, "warn", fmt.Sprintf("%d 个资产下载失败: %s", len(failedAssets), joinAssetErrors(failedAssets)))
	}

	now := time.Now().UTC()
	task.FinishedAt = &now
	if len(failedAssets) > 0 {
		task.Status = models.TaskStatusFailed
		task.ErrorMessage = joinAssetErrors(failedAssets)
		_ = s.db.WithContext(ctx).Save(&task).Error
		s.notifySyncFailed(ctx, repositoryID, errors.New(task.ErrorMessage))
		return
	}

	task.Status = models.TaskStatusSucceeded
	_ = s.db.WithContext(ctx).Save(&task).Error
	// 有变化才通知：新版本 或 成功下载了资产
	if checkResult.IsNewRelease || len(downloadResults) > 0 {
		s.notifySyncSuccess(ctx, checkResult.Repository, checkResult.Release, len(downloadResults))
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("同步完成: %s/%s 版本 %s，下载 %d 个资产",
		checkResult.Repository.Owner, checkResult.Repository.Repo, checkResult.Release.Tag, len(downloadResults)))
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

// downloadAssetsToStorages 为每个配置的存储分别下载资产
// 同一个资产文件会被下载到多个存储（如果仓库配置了多存储）
//
// 智能处理已有 StorageID 的记录：
//   - 如果 asset.StorageID != nil，说明该记录已绑定具体存储，只下载到该存储
//   - 如果 asset.StorageID == nil，说明是模板记录，为每个配置的存储分别创建记录并下载
func (s *Service) downloadAssetsToStorages(ctx context.Context, assets []models.Asset, storageIDs []uint) ([]assetsvc.DownloadResult, []AssetError) {
	if len(assets) == 0 || len(storageIDs) == 0 {
		return nil, nil
	}

	limit := s.MaxConcurrentDownloads()
	if limit < 1 {
		limit = defaultMaxConcurrentDownloads
	}

	semaphore := make(chan struct{}, limit)
	var wg stdsync.WaitGroup
	var mu stdsync.Mutex
	downloadResults := make([]assetsvc.DownloadResult, 0)
	failedAssets := make([]AssetError, 0)

	// 构建「asset → 需要下载到的存储列表」映射
	type downloadJob struct {
		assetID   uint
		storageID uint
		name      string
	}
	jobs := make([]downloadJob, 0, len(assets)*len(storageIDs))

	for _, asset := range assets {
		if asset.StorageID != nil && *asset.StorageID > 0 {
			// 已绑定具体存储的记录：只下载到该存储
			jobs = append(jobs, downloadJob{
				assetID:   asset.ID,
				storageID: *asset.StorageID,
				name:      asset.Name,
			})
		} else {
			// 模板记录（StorageID=NULL）：为每个配置的存储分别下载
			for _, sid := range storageIDs {
				jobs = append(jobs, downloadJob{
					assetID:   asset.ID,
					storageID: sid,
					name:      asset.Name,
				})
			}
		}
	}

	for _, job := range jobs {
		wg.Add(1)
		go func(j downloadJob) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				mu.Lock()
				failedAssets = append(failedAssets, AssetError{
					AssetID: j.assetID,
					Name:    j.name,
					Error:   ctx.Err().Error(),
				})
				mu.Unlock()
				return
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			}

			downloadResult, err := s.assetService.DownloadToStorage(ctx, j.assetID, j.storageID)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failedAssets = append(failedAssets, AssetError{
					AssetID: j.assetID,
					Name:    fmt.Sprintf("%s (存储 %d)", j.name, j.storageID),
					Error:   err.Error(),
				})
				return
			}
			downloadResults = append(downloadResults, *downloadResult)
		}(job)
	}

	wg.Wait()
	return downloadResults, failedAssets
}

func (s *Service) downloadAssets(ctx context.Context, assets []models.Asset) ([]assetsvc.DownloadResult, []AssetError) {
	if len(assets) == 0 {
		return nil, nil
	}

	limit := s.MaxConcurrentDownloads()
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

func (s *Service) notifySyncSuccess(ctx context.Context, repository models.Repository, release models.Release, downloaded int) {
	if s.notifier == nil {
		return
	}
	title := fmt.Sprintf("ReleaseHub 同步完成: %s/%s", repository.Owner, repository.Repo)
	message := fmt.Sprintf("版本: %s\n下载资产: %d", release.Tag, downloaded)
	_ = s.notifier.Notify(ctx, notifysvc.EventSyncSuccess, title, message)
}

func (s *Service) notifySyncFailed(ctx context.Context, repositoryID uint, err error) {
	if s.notifier == nil {
		return
	}
	var repository models.Repository
	title := "ReleaseHub 同步失败"
	if dbErr := s.db.WithContext(ctx).First(&repository, repositoryID).Error; dbErr == nil {
		title = fmt.Sprintf("ReleaseHub 同步失败: %s/%s", repository.Owner, repository.Repo)
	}
	_ = s.notifier.Notify(ctx, notifysvc.EventSyncFailed, title, err.Error())
}

// cleanupOrphanTemplateAssets 清理 checker 创建的 StorageID=NULL 的模板 Asset 记录
// 当仓库配置了多个存储时，checker 会为每个资产创建一条 StorageID=NULL 的记录，
// 而 downloadAssetsToStorages 会为每个存储创建带 StorageID 的实际记录。
// 模板记录永远停留在 pending 状态，造成文件树中重复显示，需要清理。
func (s *Service) cleanupOrphanTemplateAssets(ctx context.Context, releaseID uint) {
	// 查找该 Release 下 StorageID=NULL 且状态为 pending 的记录
	var orphanIDs []uint
	s.db.WithContext(ctx).
		Model(&models.Asset{}).
		Where("release_id = ? AND storage_id IS NULL AND status = ?", releaseID, models.AssetStatusPending).
		Pluck("id", &orphanIDs)

	if len(orphanIDs) == 0 {
		return
	}

	// 对每个幽灵记录，检查是否存在同名的已下载记录（任一存储上）
	for _, id := range orphanIDs {
		var asset models.Asset
		if err := s.db.WithContext(ctx).First(&asset, id).Error; err != nil {
			continue
		}

		// 检查是否有同 release+name 的非 NULL 存储记录已成功
		var verifiedCount int64
		s.db.WithContext(ctx).
			Model(&models.Asset{}).
			Where("release_id = ? AND name = ? AND storage_id IS NOT NULL AND status IN ?",
				asset.ReleaseID, asset.Name,
				[]models.AssetStatus{models.AssetStatusVerified, models.AssetStatusDownloaded}).
			Count(&verifiedCount)

		if verifiedCount > 0 {
			// 有已成功的实际记录，可以安全删除模板记录
			s.db.WithContext(ctx).Delete(&asset)
		}
	}
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
