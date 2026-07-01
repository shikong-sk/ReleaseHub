package release

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/filter"
	githubsvc "releasehub/backend/internal/services/github"
	notifysvc "releasehub/backend/internal/services/notify"
	"releasehub/backend/internal/services/provider"
	tasklogsvc "releasehub/backend/internal/services/tasklog"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GitHubClient 接口保持向后兼容，内部转换为 ReleaseProvider
type GitHubClient interface {
	GetLatestRelease(ctx context.Context, owner string, repo string, token string) (*githubsvc.Release, error)
	ListAllReleases(ctx context.Context, owner string, repo string, token string, maxPages int) ([]githubsvc.Release, error)
}

type CheckService struct {
	db            *gorm.DB
	github        GitHubClient
	githubFactory *githubsvc.ClientFactory
	providers     *provider.Registry
	notifier      *notifysvc.Service
	logService    *tasklogsvc.Service
	retention     RetentionRunner
	logger        *zap.Logger
}

type CheckResult struct {
	Repository  models.Repository `json:"repository"`
	Release     models.Release    `json:"release"`
	Assets      []models.Asset    `json:"assets"`
	IsNewRelease bool             `json:"isNewRelease"`
	Task        models.Task       `json:"task"`
}

type CheckAllResult struct {
	Repository    models.Repository `json:"repository"`
	Releases      int               `json:"releases"`
	NewReleases   int               `json:"newReleases"`
	TotalAssets   int               `json:"totalAssets"`
	PendingAssets int               `json:"pendingAssets"`
	SkippedAssets int               `json:"skippedAssets"`
	Task          models.Task       `json:"task"`
}

type RetentionRunner interface {
	CleanupRepository(ctx context.Context, repository models.Repository) error
}

func NewCheckService(db *gorm.DB, github GitHubClient) *CheckService {
	return &CheckService{
		db:         db,
		github:     github,
		notifier:   notifysvc.NewService(db),
		logService: tasklogsvc.NewService(db),
		logger:     zap.NewNop(),
	}
}

func (s *CheckService) WithRetention(retention RetentionRunner) *CheckService {
	s.retention = retention
	return s
}

func (s *CheckService) WithLogger(logger *zap.Logger) *CheckService {
	if logger != nil {
		s.logger = logger
	}
	return s
}

func (s *CheckService) WithGitHubFactory(factory *githubsvc.ClientFactory) *CheckService {
	s.githubFactory = factory
	return s
}

func (s *CheckService) WithProviderRegistry(registry *provider.Registry) *CheckService {
	s.providers = registry
	return s
}

// CheckByTag 检查指定 tag 的 Release 并持久化到数据库
func (s *CheckService) CheckByTag(ctx context.Context, repositoryID uint, tag string) (*CheckResult, error) {
	var repository models.Repository
	if err := s.db.WithContext(ctx).First(&repository, repositoryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("仓库不存在")
		}
		return nil, err
	}

	task := models.Task{
		Type:         "check_release",
		RepositoryID: &repository.ID,
		Status:       models.TaskStatusRunning,
		MaxAttempts:  1,
		StartedAt:    ptrTime(time.Now().UTC()),
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("开始检查 %s/%s 指定版本 %s", repository.Owner, repository.Repo, tag))

	token, err := s.githubToken(ctx, repository.GitHubTokenID)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "获取 GitHub Token 失败")
		return nil, err
	}

	releaseProvider, err := s.resolveProvider(ctx, repository)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "创建 Provider 失败")
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("通过 %s Provider 查询 Release %s", repository.Provider, tag))

	providerRelease, err := releaseProvider.GetReleaseByTag(ctx, repository.Owner, repository.Repo, tag, token)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTaskWithLog(ctx, &task, err, fmt.Sprintf("查询 Release %s 失败", tag))
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("发现 Release %s，资产数 %d", providerRelease.TagName, len(providerRelease.Assets)))

	// Tag 过滤
	if tagFiltered, err := s.isTagFiltered(ctx, repository, providerRelease.TagName); err != nil {
		s.failTaskWithLog(ctx, &task, err, "Tag 过滤规则无效")
		return nil, err
	} else if tagFiltered {
		s.appendLog(ctx, task.ID, "info", fmt.Sprintf("Tag %s 被 Tag 过滤规则排除，跳过同步", providerRelease.TagName))
		now := time.Now().UTC()
		task.Status = models.TaskStatusSucceeded
		task.FinishedAt = &now
		_ = s.db.WithContext(ctx).Save(&task).Error
		s.markRepositoryHealthy(ctx, repository.ID, repository.LastReleaseTag)
		return &CheckResult{Repository: repository, Task: task}, nil
	}

	matcher, err := filter.NewMatcher(repository.FilterMode, repository.AssetIncludePatterns, repository.AssetExcludePatterns)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTaskWithLog(ctx, &task, err, "资产过滤规则无效")
		return nil, fmt.Errorf("资产过滤规则无效: %w", err)
	}

	isNewRelease, err := s.releaseMissing(ctx, repository.ID, providerRelease.TagName)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "检查 Release 是否已存在失败")
		return nil, err
	}

	result, err := s.persistProviderReleaseWithIsLatest(ctx, repository, task, providerRelease, matcher, false)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "持久化 Release 数据失败")
		return nil, err
	}

	// 同步指定历史版本不更新仓库的 lastReleaseTag，仅更新状态为健康
	s.markRepositoryHealthy(ctx, repository.ID, repository.LastReleaseTag)

	// 标记任务成功
	now := time.Now().UTC()
	task.Status = models.TaskStatusSucceeded
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(&task).Error

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("检查完成: %s/%s 版本 %s", repository.Owner, repository.Repo, tag))
	result.Task = task
	result.IsNewRelease = isNewRelease
	return result, nil
}

func (s *CheckService) CheckLatest(ctx context.Context, repositoryID uint) (*CheckResult, error) {
	var repository models.Repository
	if err := s.db.WithContext(ctx).First(&repository, repositoryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("仓库不存在")
		}
		return nil, err
	}

	task := models.Task{
		Type:         "check_release",
		RepositoryID: &repository.ID,
		Status:       models.TaskStatusRunning,
		MaxAttempts:  1,
		StartedAt:    ptrTime(time.Now().UTC()),
	}
	if err := s.db.WithContext(ctx).Create(&task).Error; err != nil {
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("开始检查 %s/%s 最新 Release", repository.Owner, repository.Repo))

	token, err := s.githubToken(ctx, repository.GitHubTokenID)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "获取 GitHub Token 失败")
		return nil, err
	}

	releaseProvider, err := s.resolveProvider(ctx, repository)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "创建 Provider 失败")
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("通过 %s Provider 查询最新 Release", repository.Provider))

	providerRelease, err := releaseProvider.GetLatestRelease(ctx, repository.Owner, repository.Repo, token)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTaskWithLog(ctx, &task, err, "查询最新 Release 失败")
		return nil, err
	}

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("发现 Release %s，资产数 %d", providerRelease.TagName, len(providerRelease.Assets)))

	// Tag 过滤：如果最新 tag 不匹配规则，跳过同步
	if tagFiltered, err := s.isTagFiltered(ctx, repository, providerRelease.TagName); err != nil {
		s.failTaskWithLog(ctx, &task, err, "Tag 过滤规则无效")
		return nil, err
	} else if tagFiltered {
		s.appendLog(ctx, task.ID, "info", fmt.Sprintf("Tag %s 被 Tag 过滤规则排除，跳过同步", providerRelease.TagName))
		now := time.Now().UTC()
		task.Status = models.TaskStatusSucceeded
		task.FinishedAt = &now
		_ = s.db.WithContext(ctx).Save(&task).Error
		s.markRepositoryHealthy(ctx, repository.ID, repository.LastReleaseTag)
		return &CheckResult{Repository: repository, Task: task}, nil
	}

	isNewRelease, err := s.releaseMissing(ctx, repository.ID, providerRelease.TagName)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "检查 Release 是否已存在失败")
		return nil, err
	}

	matcher, err := filter.NewMatcher(repository.FilterMode, repository.AssetIncludePatterns, repository.AssetExcludePatterns)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTaskWithLog(ctx, &task, err, "资产过滤规则无效")
		return nil, fmt.Errorf("资产过滤规则无效: %w", err)
	}

	result, err := s.persistProviderReleaseWithIsLatest(ctx, repository, task, providerRelease, matcher, true)
	if err != nil {
		s.failTaskWithLog(ctx, &task, err, "持久化 Release 数据失败")
		return nil, err
	}

	// CheckLatest 检查的就是最新版本，更新 lastReleaseTag 为最新版本
	s.markRepositoryHealthy(ctx, repository.ID, providerRelease.TagName)
	// 重新加载仓库信息以包含 markRepositoryHealthy 的更新
	_ = s.db.WithContext(ctx).First(&result.Repository, repository.ID)

	if s.retention != nil {
		s.appendLog(ctx, task.ID, "info", "执行保留策略清理旧版本")
		_ = s.retention.CleanupRepository(ctx, result.Repository)
	}

	if isNewRelease {
		s.appendLog(ctx, task.ID, "info", fmt.Sprintf("发现新版本 %s，触发通知", providerRelease.TagName))
		s.notifyNewRelease(ctx, result.Repository, result.Release, result.Assets)
	}

	// 标记任务成功
	now := time.Now().UTC()
	task.Status = models.TaskStatusSucceeded
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(&task).Error

	s.appendLog(ctx, task.ID, "info", fmt.Sprintf("检查完成: %s/%s 最新版本 %s", repository.Owner, repository.Repo, providerRelease.TagName))
	result.Task = task
	result.IsNewRelease = isNewRelease
	return result, nil
}

// CheckAll 拉取仓库的所有 Release 并持久化到数据库
func (s *CheckService) CheckAll(ctx context.Context, repositoryID uint) (*CheckAllResult, error) {
	var repository models.Repository
	if err := s.db.WithContext(ctx).First(&repository, repositoryID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("仓库不存在")
		}
		return nil, err
	}

	task := models.Task{
		Type:         "check_all_releases",
		RepositoryID: &repository.ID,
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

	releaseProvider, err := s.resolveProvider(ctx, repository)
	if err != nil {
		s.failTask(ctx, &task, err)
		return nil, err
	}

	// 只拉取保留范围内的版本数量，避免把所有历史版本都加进来
	maxReleases := repository.RetentionKeepLatest
	if maxReleases < 1 {
		maxReleases = 1
	}
	// 每页 100，计算需要的页数（向上取整）
	maxPages := (maxReleases + 99) / 100
	if maxPages < 1 {
		maxPages = 1
	}
	providerReleases, err := releaseProvider.ListAllReleases(ctx, repository.Owner, repository.Repo, token, maxPages)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTask(ctx, &task, err)
		return nil, err
	}

	// 按 published_at 降序排序后截取保留范围内的版本
	if len(providerReleases) > maxReleases {
		providerReleases = providerReleases[:maxReleases]
	}

	matcher, err := filter.NewMatcher(repository.FilterMode, repository.AssetIncludePatterns, repository.AssetExcludePatterns)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTask(ctx, &task, err)
		return nil, fmt.Errorf("资产过滤规则无效: %w", err)
	}

	result := &CheckAllResult{
		Repository: repository,
		Releases:   len(providerReleases),
	}

	newReleases := 0
	totalAssets := 0
	pendingAssets := 0
	skippedAssets := 0

	for i, providerRelease := range providerReleases {
		// Tag 过滤：跳过不匹配的 release
		if tagFiltered, err := s.isTagFiltered(ctx, repository, providerRelease.TagName); err != nil {
			s.logger.Warn("Tag 过滤匹配失败",
				zap.String("tag", providerRelease.TagName),
				zap.Error(err))
			continue
		} else if tagFiltered {
			skippedAssets += len(providerRelease.Assets)
			continue
		}

		isLatest := i == 0
		persistResult, err := s.persistProviderReleaseWithLatest(ctx, repository, &providerRelease, matcher, isLatest)
		if err != nil {
			s.logger.Warn("持久化 Release 失败",
				zap.String("tag", providerRelease.TagName),
				zap.Error(err))
			continue
		}
		if persistResult.isNew {
			newReleases++
		}
		for _, asset := range persistResult.assets {
			totalAssets++
			if asset.Status == models.AssetStatusPending {
				pendingAssets++
			}
			if asset.Status == models.AssetStatusSkipped {
				skippedAssets++
			}
		}
	}

	now := time.Now().UTC()
	repository.LastCheckAt = &now
	if len(providerReleases) > 0 {
		repository.LastReleaseTag = providerReleases[0].TagName
	}
	repository.LastStatus = models.RepositoryStatusHealthy
	_ = s.db.WithContext(ctx).Save(&repository).Error

	// 确保第一个 Release 标记为 is_latest（循环中可能被覆盖）
	if len(providerReleases) > 0 {
		_ = s.db.WithContext(ctx).Model(&models.Release{}).
			Where("repository_id = ? AND tag = ?", repository.ID, providerReleases[0].TagName).
			Update("is_latest", true).Error
	}

	result.NewReleases = newReleases
	result.TotalAssets = totalAssets
	result.PendingAssets = pendingAssets
	result.SkippedAssets = skippedAssets
	result.Repository = repository

	task.Status = models.TaskStatusSucceeded
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(&task).Error

	result.Task = task
	return result, nil
}

// resolveProvider 根据仓库配置选择对应的 ReleaseProvider

// TokenForRepository 返回仓库配置的 GitHub Token（公开方法，供 handler 调用）
func (s *CheckService) TokenForRepository(ctx context.Context, repository *models.Repository) (string, error) {
	return s.githubToken(ctx, repository.GitHubTokenID)
}

// ProviderForRepository 返回仓库对应的 ReleaseProvider（公开方法，供 handler 调用）
func (s *CheckService) ProviderForRepository(ctx context.Context, repository *models.Repository) (provider.ReleaseProvider, error) {
	return s.resolveProvider(ctx, *repository)
}

func (s *CheckService) resolveProvider(ctx context.Context, repository models.Repository) (provider.ReleaseProvider, error) {
	if s.providers != nil {
		// 使用 provider registry，GitHub provider 可根据 proxy 选择 transport
		if repository.Provider == "github" || repository.Provider == "" {
			githubClient, err := s.githubClient(ctx, repository)
			if err != nil {
				return nil, err
			}
			concrete, ok := githubClient.(*githubsvc.Client)
			if !ok {
				return nil, fmt.Errorf("GitHub Client 类型不兼容")
			}
			return provider.NewGitHubProvider(concrete), nil
		}
		return s.providers.GetProvider(repository.Provider, repository.ProviderApiBaseUrl)
	}

	// 回退：仅支持 GitHub
	if repository.Provider != "" && repository.Provider != "github" {
		return nil, fmt.Errorf("未配置 provider registry，暂不支持 provider: %s", repository.Provider)
	}

	githubClient, err := s.githubClient(ctx, repository)
	if err != nil {
		return nil, err
	}
	concrete, ok := githubClient.(*githubsvc.Client)
	if !ok {
		return nil, fmt.Errorf("GitHub Client 类型不兼容")
	}
	return provider.NewGitHubProvider(concrete), nil
}

// persistProviderRelease 将 ProviderRelease 持久化到数据库
func (s *CheckService) persistProviderReleaseWithIsLatest(ctx context.Context, repository models.Repository, task models.Task, pRelease *provider.ProviderRelease, matcher *filter.Matcher, isLatest bool) (*CheckResult, error) {
	now := time.Now().UTC()
	result := &CheckResult{}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 仅当 isLatest=true 时清除其他版本的 is_latest 标记并设当前版本为最新
		if isLatest {
			if err := tx.Model(&models.Release{}).
				Where("repository_id = ?", repository.ID).
				Update("is_latest", false).Error; err != nil {
				return err
			}
		}

		publishedAt := pRelease.PublishedAt
		release := models.Release{
			RepositoryID:      repository.ID,
			ProviderReleaseID: pRelease.ID,
			Tag:               pRelease.TagName,
			Name:              pRelease.Name,
			PublishedAt:       &publishedAt,
			Body:              pRelease.Body,
			HTMLURL:           pRelease.HTMLURL,
			APIURL:            pRelease.APIURL,
			IsLatest:          isLatest,
			SyncStatus:        "checked",
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "repository_id"},
				{Name: "tag"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"provider_release_id",
				"name",
				"published_at",
				"body",
				"html_url",
				"api_url",
				"is_latest",
				"sync_status",
				"updated_at",
			}),
		}).Create(&release).Error; err != nil {
			return err
		}

		if err := tx.Where("repository_id = ? AND tag = ?", repository.ID, pRelease.TagName).First(&release).Error; err != nil {
			return err
		}

		assets := make([]models.Asset, 0, len(pRelease.Assets))
		sha256Map := parseSHA256Map(pRelease.Body)
		for _, pAsset := range pRelease.Assets {
			matched, err := matcher.Match(pAsset.Name)
			if err != nil {
				return err
			}

			status := models.AssetStatusSkipped
			if matched {
				status = models.AssetStatusPending
			}

			// 查找是否已有同 release+name 的资产记录（不限 storage_id）
			// 如果已有，更新元数据即可，不创建新记录
			var existingAssets []models.Asset
			if err := tx.Where("release_id = ? AND name = ?", release.ID, pAsset.Name).
				Find(&existingAssets).Error; err != nil {
				return err
			}

				if len(existingAssets) > 0 {
					// 已有记录：根据当前状态分别处理
					for i := range existingAssets {
						ea := &existingAssets[i]

						updates := map[string]any{
							"provider_asset_id":    pAsset.ID,
							"size":                 pAsset.Size,
							"content_type":         pAsset.ContentType,
							"download_url":         pAsset.URL,
							"browser_download_url": pAsset.BrowserDownloadURL,
							"expected_sha256":      sha256Map[pAsset.Name],
							"updated_at":           now,
						}

						if !matched {
							// 不匹配的资产：保持 skipped 状态
							updates["status"] = models.AssetStatusSkipped
						} else if ea.StorageID != nil && *ea.StorageID > 0 &&
							(ea.Status == models.AssetStatusVerified || ea.Status == models.AssetStatusDownloaded) {
							// 已绑定存储且已下载成功的记录：只更新元数据，不重置下载状态
							// 避免重复下载已存在的文件
						} else if ea.StorageID != nil && *ea.StorageID > 0 &&
							(ea.Status == models.AssetStatusPending || ea.Status == models.AssetStatusFailed) {
							// 已绑定存储但未完成的记录：保持 storage_id，重置为 pending 重试
							updates["status"] = models.AssetStatusPending
							updates["storage_path"] = ""
							updates["downloaded_at"] = gorm.Expr("NULL")
						} else {
							// 模板记录（storage_id=NULL）：保持模板状态，由 syncer 分配存储
							updates["status"] = models.AssetStatusPending
							updates["storage_path"] = ""
							updates["downloaded_at"] = gorm.Expr("NULL")
						}

						if err := tx.Model(&existingAssets[i]).Updates(updates).Error; err != nil {
							return err
						}
					}
			} else {
				// 没有记录，创建新的（不设 StorageID，由 syncer 下载时按存储创建）
				asset := models.Asset{
					ReleaseID:          release.ID,
					ProviderAssetID:    pAsset.ID,
					Name:               pAsset.Name,
					Size:               pAsset.Size,
					ContentType:        pAsset.ContentType,
					DownloadURL:        pAsset.URL,
					BrowserDownloadURL: pAsset.BrowserDownloadURL,
					ExpectedSHA256:     sha256Map[pAsset.Name],
					Status:             status,
				}
				if err := tx.Create(&asset).Error; err != nil {
					return err
				}
			}
		}

		if err := tx.Where("release_id = ?", release.ID).Order("name ASC").Find(&assets).Error; err != nil {
			return err
		}

		repository.LastCheckAt = &now
		repository.LastStatus = models.RepositoryStatusHealthy
		// LastReleaseTag 不在此处更新，由调用方根据场景决定
		if err := tx.Save(&repository).Error; err != nil {
			return err
		}

		task.Status = models.TaskStatusSucceeded
		task.FinishedAt = &now
		if err := tx.Save(&task).Error; err != nil {
			return err
		}

		result.Repository = repository
		result.Release = release
		result.Assets = assets
		result.Task = task
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

type persistResult struct {
	isNew  bool
	assets []models.Asset
}

func (s *CheckService) persistProviderReleaseWithLatest(ctx context.Context, repository models.Repository, pRelease *provider.ProviderRelease, matcher *filter.Matcher, isLatest bool) (*persistResult, error) {
	now := time.Now().UTC()

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Release{}).
			Where("repository_id = ?", repository.ID).
			Update("is_latest", false).Error; err != nil {
			return err
		}

		publishedAt := pRelease.PublishedAt
		release := models.Release{
			RepositoryID:      repository.ID,
			ProviderReleaseID: pRelease.ID,
			Tag:               pRelease.TagName,
			Name:              pRelease.Name,
			PublishedAt:       &publishedAt,
			Body:              pRelease.Body,
			HTMLURL:           pRelease.HTMLURL,
			APIURL:            pRelease.APIURL,
			IsLatest:          isLatest,
			SyncStatus:        "checked",
		}

		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "repository_id"},
				{Name: "tag"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"provider_release_id",
				"name",
				"published_at",
				"body",
				"html_url",
				"api_url",
				"is_latest",
				"sync_status",
				"updated_at",
			}),
		}).Create(&release).Error; err != nil {
			return err
		}

		if err := tx.Where("repository_id = ? AND tag = ?", repository.ID, pRelease.TagName).First(&release).Error; err != nil {
			return err
		}

		sha256Map := parseSHA256Map(pRelease.Body)
		for _, pAsset := range pRelease.Assets {
			matched, matchErr := matcher.Match(pAsset.Name)
			if matchErr != nil {
				return matchErr
			}

			status := models.AssetStatusSkipped
			if matched {
				status = models.AssetStatusPending
			}

			// 查找是否已有同 release+name 的资产记录（不限 storage_id）
			var existingAssets []models.Asset
			if err := tx.Where("release_id = ? AND name = ?", release.ID, pAsset.Name).
				Find(&existingAssets).Error; err != nil {
				return err
			}

				if len(existingAssets) > 0 {
					// 已有记录：根据当前状态分别处理
					for i := range existingAssets {
						ea := &existingAssets[i]

						updates := map[string]any{
							"provider_asset_id":    pAsset.ID,
							"size":                 pAsset.Size,
							"content_type":         pAsset.ContentType,
							"download_url":         pAsset.URL,
							"browser_download_url": pAsset.BrowserDownloadURL,
							"expected_sha256":      sha256Map[pAsset.Name],
							"updated_at":           now,
						}

						if !matched {
							// 不匹配的资产：保持 skipped 状态
							updates["status"] = models.AssetStatusSkipped
						} else if ea.StorageID != nil && *ea.StorageID > 0 &&
							(ea.Status == models.AssetStatusVerified || ea.Status == models.AssetStatusDownloaded) {
							// 已绑定存储且已下载成功的记录：只更新元数据，不重置下载状态
						} else if ea.StorageID != nil && *ea.StorageID > 0 &&
							(ea.Status == models.AssetStatusPending || ea.Status == models.AssetStatusFailed) {
							// 已绑定存储但未完成的记录：保持 storage_id，重置为 pending 重试
							updates["status"] = models.AssetStatusPending
							updates["storage_path"] = ""
							updates["downloaded_at"] = gorm.Expr("NULL")
						} else {
							// 模板记录（storage_id=NULL）：保持模板状态，由 syncer 分配存储
							updates["status"] = models.AssetStatusPending
							updates["storage_path"] = ""
							updates["downloaded_at"] = gorm.Expr("NULL")
						}

						if err := tx.Model(&existingAssets[i]).Updates(updates).Error; err != nil {
							return err
						}
					}
			} else {
				// 没有记录，创建新的
				asset := models.Asset{
					ReleaseID:          release.ID,
					ProviderAssetID:    pAsset.ID,
					Name:               pAsset.Name,
					Size:               pAsset.Size,
					ContentType:        pAsset.ContentType,
					DownloadURL:        pAsset.URL,
					BrowserDownloadURL: pAsset.BrowserDownloadURL,
					ExpectedSHA256:     sha256Map[pAsset.Name],
					Status:             status,
				}
				if err := tx.Create(&asset).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	var assets []models.Asset
	var release models.Release
	_ = s.db.WithContext(ctx).Where("repository_id = ? AND tag = ?", repository.ID, pRelease.TagName).First(&release).Error
	_ = s.db.WithContext(ctx).Where("release_id = ?", release.ID).Order("name ASC").Find(&assets).Error
	return &persistResult{isNew: true, assets: assets}, nil
}

func (s *CheckService) githubToken(ctx context.Context, tokenID *uint) (string, error) {
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

func (s *CheckService) githubClient(ctx context.Context, repository models.Repository) (GitHubClient, error) {
	if s.githubFactory != nil {
		return s.githubFactory.ClientForRepository(ctx, repository)
	}
	if s.github == nil {
		return nil, fmt.Errorf("GitHub Client 未初始化")
	}
	return s.github, nil
}

func (s *CheckService) releaseMissing(ctx context.Context, repositoryID uint, tag string) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&models.Release{}).
		Where("repository_id = ? AND tag = ?", repositoryID, tag).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *CheckService) notifyNewRelease(ctx context.Context, repository models.Repository, release models.Release, assets []models.Asset) {
	if s.notifier == nil {
		return
	}
	title := fmt.Sprintf("ReleaseHub 发现新版本: %s/%s", repository.Owner, repository.Repo)
	message := fmt.Sprintf("版本: %s\n资产数量: %d\n页面: %s", release.Tag, len(assets), release.HTMLURL)
	_ = s.notifier.Notify(ctx, notifysvc.EventNewRelease, title, message)
}

func (s *CheckService) failTask(ctx context.Context, task *models.Task, err error) {
	now := time.Now().UTC()
	task.Status = models.TaskStatusFailed
	task.ErrorMessage = err.Error()
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(task).Error
}

// failTaskWithLog 标记任务失败并写入日志
func (s *CheckService) failTaskWithLog(ctx context.Context, task *models.Task, err error, logMsg string) {
	s.failTask(ctx, task, err)
	s.appendLog(ctx, task.ID, "error", logMsg+": "+err.Error())
}

// appendLog 写入任务日志（忽略错误，日志写入失败不阻断主流程）
func (s *CheckService) appendLog(ctx context.Context, taskID uint, level string, message string) {
	if s.logService != nil {
		_ = s.logService.Append(ctx, taskID, level, message)
	}
}

// markRepositoryHealthy 更新仓库状态为健康
func (s *CheckService) markRepositoryHealthy(ctx context.Context, repositoryID uint, latestTag string) {
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&models.Repository{}).
		Where("id = ?", repositoryID).
		Updates(map[string]any{
			"last_check_at":    now,
			"last_status":      models.RepositoryStatusHealthy,
			"last_release_tag": latestTag,
		}).Error
}

func (s *CheckService) markRepositoryFailed(ctx context.Context, repositoryID uint) {
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&models.Repository{}).
		Where("id = ?", repositoryID).
		Updates(map[string]any{
			"last_check_at": now,
			"last_status":   models.RepositoryStatusFailed,
		}).Error
}

// parseSHA256Map 从 release body 解析 sha256 校验和映射
// 支持格式: "sha256:abc123  filename" 或 "SHA256 (file) = hash" 或 "<filename>.sha256" 行
// ponytail: 正则覆盖常见格式，不追求100%覆盖，漏匹配时 expected_sha256 留空不影响下载
func parseSHA256Map(body string) map[string]string {
	m := map[string]string{}
	// 格式1: sha256:hash  filename  或  hash  filename
	re := regexp.MustCompile(`(?m)(?:sha256:)?([0-9a-fA-F]{64})\s+(\S+)`)
	for _, match := range re.FindAllStringSubmatch(body, -1) {
		hash := strings.ToLower(match[1])
		filename := filepath.Base(match[2])
		m[filename] = hash
	}
	// 格式2: SHA256 (filename) = hash
	re2 := regexp.MustCompile(`(?m)SHA256\s*\((\S+)\)\s*=\s*([0-9a-fA-F]{64})`)
	for _, match := range re2.FindAllStringSubmatch(body, -1) {
		hash := strings.ToLower(match[2])
		filename := filepath.Base(match[1])
		m[filename] = hash
	}
	return m
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

// isTagFiltered 检查 tag 是否被仓库的 Tag 过滤规则排除
// 返回 true 表示该 tag 应被跳过
func (s *CheckService) isTagFiltered(ctx context.Context, repository models.Repository, tag string) (bool, error) {
	if repository.TagFilterMode == "" {
		return false, nil
	}
	tagMatcher, err := filter.NewMatcher(repository.TagFilterMode, repository.TagIncludePattern, repository.TagExcludePattern)
	if err != nil {
		return false, fmt.Errorf("Tag 过滤规则无效: %w", err)
	}
	matched, err := tagMatcher.Match(tag)
	if err != nil {
		return false, fmt.Errorf("Tag 过滤匹配失败: %w", err)
	}
	return !matched, nil
}
