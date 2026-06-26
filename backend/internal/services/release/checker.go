package release

import (
	"context"
	"errors"
	"fmt"
	"time"

	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/filter"
	githubsvc "releasehub/backend/internal/services/github"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GitHubClient interface {
	GetLatestRelease(ctx context.Context, owner string, repo string, token string) (*githubsvc.Release, error)
	ListAllReleases(ctx context.Context, owner string, repo string, token string, maxPages int) ([]githubsvc.Release, error)
}

type CheckService struct {
	db        *gorm.DB
	github    GitHubClient
	retention RetentionRunner
	logger    *zap.Logger
}

type CheckResult struct {
	Repository models.Repository `json:"repository"`
	Release    models.Release    `json:"release"`
	Assets     []models.Asset    `json:"assets"`
	Task       models.Task       `json:"task"`
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
		db:     db,
		github: github,
		logger: zap.NewNop(),
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

	token, err := s.githubToken(ctx, repository.GitHubTokenID)
	if err != nil {
		s.failTask(ctx, &task, err)
		return nil, err
	}

	githubRelease, err := s.github.GetLatestRelease(ctx, repository.Owner, repository.Repo, token)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTask(ctx, &task, err)
		return nil, err
	}

	matcher, err := filter.NewMatcher(repository.FilterMode, repository.AssetIncludePatterns, repository.AssetExcludePatterns)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTask(ctx, &task, err)
		return nil, fmt.Errorf("资产过滤规则无效: %w", err)
	}

	result, err := s.persistRelease(ctx, repository, task, githubRelease, matcher)
	if err != nil {
		s.failTask(ctx, &task, err)
		return nil, err
	}

	if s.retention != nil {
		_ = s.retention.CleanupRepository(ctx, result.Repository)
	}

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

	githubReleases, err := s.github.ListAllReleases(ctx, repository.Owner, repository.Repo, token, 10)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTask(ctx, &task, err)
		return nil, err
	}

	matcher, err := filter.NewMatcher(repository.FilterMode, repository.AssetIncludePatterns, repository.AssetExcludePatterns)
	if err != nil {
		s.markRepositoryFailed(ctx, repository.ID)
		s.failTask(ctx, &task, err)
		return nil, fmt.Errorf("资产过滤规则无效: %w", err)
	}

	result := &CheckAllResult{
		Repository: repository,
		Releases:   len(githubReleases),
	}

	newReleases := 0
	totalAssets := 0
	pendingAssets := 0
	skippedAssets := 0

	for i, githubRelease := range githubReleases {
		isLatest := i == 0
		persistResult, err := s.persistReleaseWithLatest(ctx, repository, &githubRelease, matcher, isLatest)
		if err != nil {
			s.logger.Warn("持久化 Release 失败",
				zap.String("tag", githubRelease.TagName),
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
	if len(githubReleases) > 0 {
		repository.LastReleaseTag = githubReleases[0].TagName
	}
	repository.LastStatus = models.RepositoryStatusHealthy
	_ = s.db.WithContext(ctx).Save(&repository).Error

	result.NewReleases = newReleases
	result.TotalAssets = totalAssets
	result.PendingAssets = pendingAssets
	result.SkippedAssets = skippedAssets
	result.Repository = repository

	task.Status = models.TaskStatusSucceeded
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(&task).Error
	result.Task = task

	if s.retention != nil {
		_ = s.retention.CleanupRepository(ctx, result.Repository)
	}

	return result, nil
}

type persistResult struct {
	isNew  bool
	assets []models.Asset
}

func (s *CheckService) persistReleaseWithLatest(ctx context.Context, repository models.Repository, githubRelease *githubsvc.Release, matcher *filter.Matcher, isLatest bool) (*persistResult, error) {
	// 检查是否已存在
	var existingRelease models.Release
	err := s.db.WithContext(ctx).
		Where("repository_id = ? AND tag = ?", repository.ID, githubRelease.TagName).
		First(&existingRelease).Error
	if err == nil {
		// 已存在，更新 is_latest 标记
		if existingRelease.IsLatest != isLatest {
			_ = s.db.WithContext(ctx).Model(&existingRelease).Update("is_latest", isLatest).Error
		}
		// 加载已有资产
		var assets []models.Asset
		_ = s.db.WithContext(ctx).Where("release_id = ?", existingRelease.ID).Order("name ASC").Find(&assets).Error
		return &persistResult{isNew: false, assets: assets}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 新 Release，需要持久化
	publishedAt := githubRelease.PublishedAt
	release := models.Release{
		RepositoryID:      repository.ID,
		ProviderReleaseID: githubRelease.ID,
		Tag:               githubRelease.TagName,
		Name:              githubRelease.Name,
		PublishedAt:       &publishedAt,
		Body:              githubRelease.Body,
		HTMLURL:           githubRelease.HTMLURL,
		APIURL:            githubRelease.URL,
		IsLatest:          isLatest,
		SyncStatus:        "checked",
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 先清除其他 Release 的 is_latest 标记
		if isLatest {
			if err := tx.Model(&models.Release{}).
				Where("repository_id = ? AND is_latest = ?", repository.ID, true).
				Update("is_latest", false).Error; err != nil {
				return err
			}
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

		if err := tx.Where("repository_id = ? AND tag = ?", repository.ID, githubRelease.TagName).First(&release).Error; err != nil {
			return err
		}

		for _, githubAsset := range githubRelease.Assets {
			matched, matchErr := matcher.Match(githubAsset.Name)
			if matchErr != nil {
				return matchErr
			}

			status := models.AssetStatusSkipped
			if matched {
				status = models.AssetStatusPending
			}

			asset := models.Asset{
				ReleaseID:          release.ID,
				ProviderAssetID:    githubAsset.ID,
				Name:               githubAsset.Name,
				Size:               githubAsset.Size,
				ContentType:        githubAsset.ContentType,
				DownloadURL:        githubAsset.URL,
				BrowserDownloadURL: githubAsset.BrowserDownloadURL,
				Status:             status,
			}

			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "release_id"},
					{Name: "name"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"provider_asset_id",
					"size",
					"content_type",
					"download_url",
					"browser_download_url",
					"status",
					"updated_at",
				}),
			}).Create(&asset).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	var assets []models.Asset
	_ = s.db.WithContext(ctx).Where("release_id = ?", release.ID).Order("name ASC").Find(&assets).Error
	return &persistResult{isNew: true, assets: assets}, nil
}

func (s *CheckService) persistRelease(ctx context.Context, repository models.Repository, task models.Task, githubRelease *githubsvc.Release, matcher *filter.Matcher) (*CheckResult, error) {
	now := time.Now().UTC()
	result := &CheckResult{}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Release{}).
			Where("repository_id = ?", repository.ID).
			Update("is_latest", false).Error; err != nil {
			return err
		}

		publishedAt := githubRelease.PublishedAt
		release := models.Release{
			RepositoryID:      repository.ID,
			ProviderReleaseID: githubRelease.ID,
			Tag:               githubRelease.TagName,
			Name:              githubRelease.Name,
			PublishedAt:       &publishedAt,
			Body:              githubRelease.Body,
			HTMLURL:           githubRelease.HTMLURL,
			APIURL:            githubRelease.URL,
			IsLatest:          true,
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

		if err := tx.Where("repository_id = ? AND tag = ?", repository.ID, githubRelease.TagName).First(&release).Error; err != nil {
			return err
		}

		assets := make([]models.Asset, 0, len(githubRelease.Assets))
		for _, githubAsset := range githubRelease.Assets {
			matched, err := matcher.Match(githubAsset.Name)
			if err != nil {
				return err
			}

			status := models.AssetStatusSkipped
			if matched {
				status = models.AssetStatusPending
			}

			var existingAsset models.Asset
			existingErr := tx.Where("release_id = ? AND name = ?", release.ID, githubAsset.Name).First(&existingAsset).Error
			if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
				return existingErr
			}
			if existingErr == nil && matched && existingAsset.StoragePath != "" && existingAsset.Status == models.AssetStatusVerified {
				status = existingAsset.Status
			}

			asset := models.Asset{
				ReleaseID:          release.ID,
				ProviderAssetID:    githubAsset.ID,
				Name:               githubAsset.Name,
				Size:               githubAsset.Size,
				ContentType:        githubAsset.ContentType,
				DownloadURL:        githubAsset.URL,
				BrowserDownloadURL: githubAsset.BrowserDownloadURL,
				Status:             status,
			}

			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "release_id"},
					{Name: "name"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"provider_asset_id",
					"size",
					"content_type",
					"download_url",
					"browser_download_url",
					"status",
					"updated_at",
				}),
			}).Create(&asset).Error; err != nil {
				return err
			}
		}

		if err := tx.Where("release_id = ?", release.ID).Order("name ASC").Find(&assets).Error; err != nil {
			return err
		}

		repository.LastCheckAt = &now
		repository.LastReleaseTag = release.Tag
		repository.LastStatus = models.RepositoryStatusHealthy
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

func (s *CheckService) failTask(ctx context.Context, task *models.Task, err error) {
	now := time.Now().UTC()
	task.Status = models.TaskStatusFailed
	task.ErrorMessage = err.Error()
	task.FinishedAt = &now
	_ = s.db.WithContext(ctx).Save(task).Error
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

func ptrTime(t time.Time) *time.Time {
	return &t
}
