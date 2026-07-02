package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/provider"
	"releasehub/backend/internal/services/storage"

	"gorm.io/gorm"
)

const (
	defaultProvider        = "github"
	defaultIntervalSeconds = 1800
	defaultFilterMode      = "glob"
	defaultRetention       = 5
	minIntervalSeconds     = 300
)

var (
	errNotFound       = errors.New("仓库不存在")
	errInvalidInput   = errors.New("仓库参数无效")
	ownerPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type Service struct {
	db       *gorm.DB
	storages *storage.DriverFactory
}

type CreateInput struct {
	Provider             string `json:"provider"`
	Owner                string `json:"owner"`
	Repo                 string `json:"repo"`
	Enabled              *bool  `json:"enabled"`
	GitHubTokenID        *uint  `json:"githubTokenId"`
	StorageID            *uint  `json:"storageId"`     // 兼容旧 API，单存储
	StorageIDs           []uint `json:"storageIds"`    // 多存储（优先）
	ProxyID              *uint  `json:"proxyId"`
	ProviderApiBaseUrl    string `json:"providerApiBaseUrl"`
	IntervalSeconds      int    `json:"intervalSeconds"`
	FilterMode           string `json:"filterMode"`
	AssetIncludePatterns string `json:"assetIncludePatterns"`
	AssetExcludePatterns string `json:"assetExcludePatterns"`
	TagFilterMode        string `json:"tagFilterMode"`
	TagIncludePattern    string `json:"tagIncludePattern"`
	TagExcludePattern    string `json:"tagExcludePattern"`
	RetentionKeepLatest  int    `json:"retentionKeepLatest"`
}

type UpdateInput struct {
	Enabled              *bool   `json:"enabled"`
	GitHubTokenID        *uint   `json:"githubTokenId"`
	StorageID            *uint  `json:"storageId"`
	StorageIDs           []uint  `json:"storageIds"`    // 多存储（优先）
	ProxyID              *uint  `json:"proxyId"`
	ProviderApiBaseUrl    *string `json:"providerApiBaseUrl"`
	IntervalSeconds      *int    `json:"intervalSeconds"`
	FilterMode           *string `json:"filterMode"`
	AssetIncludePatterns *string `json:"assetIncludePatterns"`
	AssetExcludePatterns *string `json:"assetExcludePatterns"`
	TagFilterMode        *string `json:"tagFilterMode"`
	TagIncludePattern    *string `json:"tagIncludePattern"`
	TagExcludePattern    *string `json:"tagExcludePattern"`
	RetentionKeepLatest  *int    `json:"retentionKeepLatest"`
}

func NewService(db *gorm.DB, storageConfig config.StorageConfig) *Service {
	return &Service{
		db:       db,
		storages: storage.NewDriverFactory(db, storageConfig),
	}
}

func (s *Service) DB() *gorm.DB {
	return s.db
}

func IsNotFound(err error) bool {
	return errors.Is(err, errNotFound)
}

func IsInvalidInput(err error) bool {
	return errors.Is(err, errInvalidInput)
}

func (s *Service) List(ctx context.Context) ([]models.Repository, error) {
	var repositories []models.Repository
	err := s.db.WithContext(ctx).
		Order("updated_at DESC").
		Find(&repositories).Error
	if err != nil {
		return nil, err
	}

	// 批量加载存储关联
	repoIDs := make([]uint, 0, len(repositories))
	for _, r := range repositories {
		repoIDs = append(repoIDs, r.ID)
	}
	var allRS []models.RepositoryStorage
	if len(repoIDs) > 0 {
		s.db.WithContext(ctx).Where("repository_id IN ?", repoIDs).Find(&allRS)
	}
	rsMap := make(map[uint][]uint)
	for _, rs := range allRS {
		rsMap[rs.RepositoryID] = append(rsMap[rs.RepositoryID], rs.StorageID)
	}
	for i := range repositories {
		if ids, ok := rsMap[repositories[i].ID]; ok {
			repositories[i].StorageIDs = ids
		}
	}

	// 批量聚合每仓库已下载资产占用的存储大小（仅统计 download_bytes > 0 的已下载资产）
	if len(repoIDs) > 0 {
		type sizeRow struct {
			RepositoryID  uint
			TotalBytes    int64
		}
		var sizeRows []sizeRow
		// 关联 assets → releases → repositories，按 repository_id 分组求和
		// 只统计已下载成功的资产（download_bytes 记录实际下载字节数）
		if err := s.db.WithContext(ctx).
			Model(&models.Asset{}).
			Select("releases.repository_id AS repository_id, COALESCE(SUM(assets.download_bytes), 0) AS total_bytes").
			Joins("JOIN releases ON releases.id = assets.release_id").
			Where("releases.repository_id IN ? AND assets.download_bytes > 0", repoIDs).
			Group("releases.repository_id").
			Scan(&sizeRows).Error; err == nil {
			sizeMap := make(map[uint]int64, len(sizeRows))
			for _, r := range sizeRows {
				sizeMap[r.RepositoryID] = r.TotalBytes
			}
			for i := range repositories {
				repositories[i].TotalStorageBytes = sizeMap[repositories[i].ID]
			}
		}
	}

	return repositories, nil
}

func (s *Service) Get(ctx context.Context, id uint) (*models.Repository, error) {
	var repository models.Repository
	err := s.db.WithContext(ctx).First(&repository, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}

	// 加载存储关联
	s.loadStorageIDs(ctx, &repository)

	return &repository, nil
}

func (s *Service) Create(ctx context.Context, input CreateInput) (*models.Repository, error) {
	repository, err := buildRepository(input)
	if err != nil {
		return nil, err
	}

	if err := s.db.WithContext(ctx).Create(repository).Error; err != nil {
		return nil, err
	}

	// 同步存储关联表
	if err := s.syncRepositoryStorages(ctx, repository.ID, input.StorageIDs, input.StorageID); err != nil {
		return nil, fmt.Errorf("同步存储关联失败: %w", err)
	}

	// 加载存储关联到响应
	s.loadStorageIDs(ctx, repository)

	return repository, nil
}

func (s *Service) Update(ctx context.Context, id uint, input UpdateInput) (*models.Repository, error) {
	repository, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Enabled != nil {
		repository.Enabled = *input.Enabled
	}
	if input.GitHubTokenID != nil {
		repository.GitHubTokenID = input.GitHubTokenID
	}
	// 处理存储配置变更
	storageChanged := false
	if input.StorageIDs != nil {
		// 多存储模式：更新关联表
		if err := s.syncRepositoryStorages(ctx, repository.ID, input.StorageIDs, nil); err != nil {
			return nil, fmt.Errorf("同步存储关联失败: %w", err)
		}
		storageChanged = true
	} else if input.StorageID != nil {
		// 兼容旧 API：单存储模式
		oldStorageID := repository.StorageID
		newStorageID := input.StorageID
		repository.StorageID = newStorageID
		if !storageIDEqual(oldStorageID, newStorageID) {
			if err := s.syncRepositoryStorages(ctx, repository.ID, nil, input.StorageID); err != nil {
				return nil, fmt.Errorf("同步存储关联失败: %w", err)
			}
			storageChanged = true
		}
	}
	if storageChanged {
		// 存储变更后，将不在新存储列表中的资产重置为 pending
		if err := s.resetOrphanedAssets(ctx, repository.ID); err != nil {
			return nil, fmt.Errorf("重置孤立资产失败: %w", err)
		}
	}
	if input.ProxyID != nil {
		repository.ProxyID = input.ProxyID
	}
	if input.ProviderApiBaseUrl != nil {
		repository.ProviderApiBaseUrl = strings.TrimSpace(*input.ProviderApiBaseUrl)
	}
	if input.IntervalSeconds != nil {
		if err := validateInterval(*input.IntervalSeconds); err != nil {
			return nil, err
		}
		repository.IntervalSeconds = *input.IntervalSeconds
	}
	if input.FilterMode != nil {
		filterMode := normalizeFilterMode(*input.FilterMode)
		if err := validateFilterMode(filterMode); err != nil {
			return nil, err
		}
		repository.FilterMode = filterMode
	}
	if input.AssetIncludePatterns != nil {
		repository.AssetIncludePatterns = strings.TrimSpace(*input.AssetIncludePatterns)
	}
	if input.AssetExcludePatterns != nil {
		repository.AssetExcludePatterns = strings.TrimSpace(*input.AssetExcludePatterns)
	}
	if input.TagFilterMode != nil {
		tagFilterMode := normalizeFilterMode(*input.TagFilterMode)
		if tagFilterMode != "" {
			if err := validateFilterMode(tagFilterMode); err != nil {
				return nil, err
			}
		}
		repository.TagFilterMode = tagFilterMode
	}
	if input.TagIncludePattern != nil {
		repository.TagIncludePattern = strings.TrimSpace(*input.TagIncludePattern)
	}
	if input.TagExcludePattern != nil {
		repository.TagExcludePattern = strings.TrimSpace(*input.TagExcludePattern)
	}
	if input.RetentionKeepLatest != nil {
		if err := validateRetention(*input.RetentionKeepLatest); err != nil {
			return nil, err
		}
		repository.RetentionKeepLatest = *input.RetentionKeepLatest
	}

	if err := s.db.WithContext(ctx).Save(repository).Error; err != nil {
		return nil, err
	}

	// 重新加载存储关联（syncRepositoryStorages 可能已更新关联表）
	s.loadStorageIDs(ctx, repository)

	return repository, nil
}

func (s *Service) Delete(ctx context.Context, id uint) error {
	repository, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	// 1. 查找该仓库所有已下载资产的存储路径，用于删除物理文件
	var assets []models.Asset
	if err := s.db.WithContext(ctx).
		Joins("JOIN releases ON releases.id = assets.release_id").
		Where("releases.repository_id = ?", id).
		Where("assets.storage_path != ''").
		Find(&assets).Error; err != nil {
		// 查找失败不阻断删除，但记录错误以便排查孤儿文件
		assets = nil
		_ = err
	}

	// 2. 删除物理文件（尽力而为，不阻断 DB 删除）
	for _, asset := range assets {
		if asset.StoragePath == "" {
			continue
		}
		// ponytail: 用 Asset 的 StorageID 构造临时 Repository 传给 DriverAndStorageID，
		// 复用已有的多存储解析逻辑，不另写 driverForAsset
		tmpRepo := models.Repository{StorageID: asset.StorageID}
		driver, _, err := s.storages.DriverAndStorageID(ctx, tmpRepo)
		if err != nil {
			continue
		}
		_ = driver.Delete(ctx, asset.StoragePath)
	}

	// 3. 事务级联删除 DB 记录
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// TaskLog → Task (by repository_id)
		if err := tx.Where("task_id IN (?)",
			tx.Model(&models.Task{}).Select("id").Where("repository_id = ?", id),
		).Delete(&models.TaskLog{}).Error; err != nil {
			return err
		}

		// Task
		if err := tx.Where("repository_id = ?", id).Delete(&models.Task{}).Error; err != nil {
			return err
		}

		// Asset → Release (by repository_id)
		if err := tx.Where("release_id IN (?)",
			tx.Model(&models.Release{}).Select("id").Where("repository_id = ?", id),
		).Delete(&models.Asset{}).Error; err != nil {
			return err
		}

		// Release
		if err := tx.Where("repository_id = ?", id).Delete(&models.Release{}).Error; err != nil {
			return err
		}

		// RepositoryStorage
		if err := tx.Where("repository_id = ?", id).Delete(&models.RepositoryStorage{}).Error; err != nil {
			return err
		}

		// Repository
		  return tx.Delete(repository).Error
		 })
	}

func buildRepository(input CreateInput) (*models.Repository, error) {
	providerName := strings.ToLower(strings.TrimSpace(input.Provider))
	if providerName == "" {
		providerName = defaultProvider
	}
	// 支持所有已注册的 Provider
	if !provider.IsSupported(providerName) {
		return nil, fmt.Errorf("%w: 暂不支持 provider %q，支持: %v", errInvalidInput, providerName, provider.SupportedProviders())
	}

	owner := strings.TrimSpace(input.Owner)
	repo := strings.TrimSpace(input.Repo)
	if err := validateOwnerRepo(owner, repo); err != nil {
		return nil, err
	}

	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	interval := input.IntervalSeconds
	if interval == 0 {
		interval = defaultIntervalSeconds
	}
	if err := validateInterval(interval); err != nil {
		return nil, err
	}

	filterMode := normalizeFilterMode(input.FilterMode)
	if filterMode == "" {
		filterMode = defaultFilterMode
	}
	if err := validateFilterMode(filterMode); err != nil {
		return nil, err
	}

	tagFilterMode := normalizeFilterMode(input.TagFilterMode)
	if tagFilterMode != "" {
		if err := validateFilterMode(tagFilterMode); err != nil {
			return nil, err
		}
	}

	retention := input.RetentionKeepLatest
	if retention == 0 {
		retention = defaultRetention
	}
	if err := validateRetention(retention); err != nil {
		return nil, err
	}

	return &models.Repository{
		Provider:             providerName,
		Owner:                owner,
		Repo:                 repo,
		Enabled:              enabled,
		GitHubTokenID:        input.GitHubTokenID,
		StorageID:            input.StorageID,
		ProxyID:              input.ProxyID,
		IntervalSeconds:      interval,
		FilterMode:           filterMode,
		AssetIncludePatterns: strings.TrimSpace(input.AssetIncludePatterns),
		AssetExcludePatterns: strings.TrimSpace(input.AssetExcludePatterns),
		TagFilterMode:        tagFilterMode,
		TagIncludePattern:    strings.TrimSpace(input.TagIncludePattern),
		TagExcludePattern:    strings.TrimSpace(input.TagExcludePattern),
		RetentionKeepLatest:  retention,
		LastStatus:           models.RepositoryStatusUnknown,
	}, nil
}

func validateOwnerRepo(owner string, repo string) error {
	if !ownerPattern.MatchString(owner) {
		return fmt.Errorf("%w: owner 格式无效", errInvalidInput)
	}
	if !repositoryPattern.MatchString(repo) {
		return fmt.Errorf("%w: repo 格式无效", errInvalidInput)
	}

	return nil
}

func validateInterval(interval int) error {
	if interval < minIntervalSeconds {
		return fmt.Errorf("%w: intervalSeconds 不能小于 %d", errInvalidInput, minIntervalSeconds)
	}

	return nil
}

func validateFilterMode(filterMode string) error {
	if filterMode != "glob" && filterMode != "regex" {
		return fmt.Errorf("%w: filterMode 只能是 glob 或 regex", errInvalidInput)
	}

	return nil
}

func validateRetention(retention int) error {
	if retention < 1 {
		return fmt.Errorf("%w: retentionKeepLatest 不能小于 1", errInvalidInput)
	}

	return nil
}

func normalizeFilterMode(filterMode string) string {
	return strings.ToLower(strings.TrimSpace(filterMode))
}

// storageIDEqual 比较两个 *uint 是否相等（nil 视为 0）
func storageIDEqual(a, b *uint) bool {
	va := uint(0)
	if a != nil {
		va = *a
	}
	vb := uint(0)
	if b != nil {
		vb = *b
	}
	return va == vb
}

// resetAssetsToPending 将指定仓库下所有 verified/downloaded 资产重置为 pending
// 用于仓库存储目标变更后，确保资产被重新下载到新存储
func (s *Service) resetAssetsToPending(ctx context.Context, repositoryID uint) error {
	// 查找该仓库的所有 Release
	var releaseIDs []uint
	if err := s.db.WithContext(ctx).
		Model(&models.Release{}).
		Where("repository_id = ?", repositoryID).
		Pluck("id", &releaseIDs).Error; err != nil {
		return err
	}
	if len(releaseIDs) == 0 {
		return nil
	}

	// 将这些 Release 下 verified/downloaded 资产重置为 pending，并清除 storage_path
	result := s.db.WithContext(ctx).
		Model(&models.Asset{}).
		Where("release_id IN ? AND status IN ?", releaseIDs,
			[]models.AssetStatus{models.AssetStatusVerified, models.AssetStatusDownloaded}).
		Updates(map[string]any{
			"status":       models.AssetStatusPending,
			"storage_path": "",
			"sha256":       "",
			"storage_id":   gorm.Expr("NULL"),
			"downloaded_at": gorm.Expr("NULL"),
		})
	return result.Error
}

// loadStorageIDs 加载仓库的存储关联 ID 到 StorageIDs 字段（非数据库字段，用于 API 响应）
func (s *Service) loadStorageIDs(ctx context.Context, repository *models.Repository) {
	var rss []models.RepositoryStorage
	if err := s.db.WithContext(ctx).Where("repository_id = ?", repository.ID).Find(&rss).Error; err == nil {
		ids := make([]uint, 0, len(rss))
		for _, rs := range rss {
			ids = append(ids, rs.StorageID)
		}
		repository.StorageIDs = ids
	}
}

// syncRepositoryStorages 同步仓库的存储关联表
// storageIDs 优先使用（多存储模式），若为空则从 singleStorageID 构建（单存储兼容）
func (s *Service) syncRepositoryStorages(ctx context.Context, repositoryID uint, storageIDs []uint, singleStorageID *uint) error {
	// 确定最终的存储 ID 列表
	var targetIDs []uint
	if len(storageIDs) > 0 {
		targetIDs = storageIDs
	} else if singleStorageID != nil {
		targetIDs = []uint{*singleStorageID}
	} else {
		// 没有指定存储，使用默认存储
		var defaultStorage models.Storage
		if err := s.db.WithContext(ctx).
			Where("is_default = ?", true).
			Order("updated_at DESC, created_at DESC").
			First(&defaultStorage).Error; err == nil {
			targetIDs = []uint{defaultStorage.ID}
		}
	}

	// 删除旧关联
	if err := s.db.WithContext(ctx).
		Where("repository_id = ?", repositoryID).
		Delete(&models.RepositoryStorage{}).Error; err != nil {
		return err
	}

	// 创建新关联
	for _, storageID := range targetIDs {
		rs := models.RepositoryStorage{
			RepositoryID: repositoryID,
			StorageID:    storageID,
		}
		if err := s.db.WithContext(ctx).Create(&rs).Error; err != nil {
			return err
		}
	}

	// 同步更新 Repository.StorageID 为第一个存储（兼容旧代码）
	if len(targetIDs) > 0 {
		s.db.WithContext(ctx).
			Model(&models.Repository{}).
			Where("id = ?", repositoryID).
			Update("storage_id", targetIDs[0])
	}

	return nil
}

// GetRepositoryStorages 获取仓库配置的所有存储 ID
func (s *Service) GetRepositoryStorages(ctx context.Context, repositoryID uint) ([]uint, error) {
	var rss []models.RepositoryStorage
	if err := s.db.WithContext(ctx).
		Where("repository_id = ?", repositoryID).
		Find(&rss).Error; err != nil {
		return nil, err
	}

	// 如果关联表为空，回退到 Repository.StorageID
	if len(rss) == 0 {
		var repo models.Repository
		if err := s.db.WithContext(ctx).First(&repo, repositoryID).Error; err != nil {
			return nil, err
		}
		if repo.StorageID != nil {
			return []uint{*repo.StorageID}, nil
		}
		// 使用默认存储
		var defaultStorage models.Storage
		if err := s.db.WithContext(ctx).
			Where("is_default = ?", true).
			Order("updated_at DESC, created_at DESC").
			First(&defaultStorage).Error; err == nil {
			return []uint{defaultStorage.ID}, nil
		}
		return nil, nil
	}

	ids := make([]uint, 0, len(rss))
	for _, rs := range rss {
		ids = append(ids, rs.StorageID)
	}
	return ids, nil
}

// resetOrphanedAssets 将不在仓库当前存储列表中的资产重置为 pending
// 比如仓库从 [本地, webdav] 改为 [webdav]，则本地存储上的资产变为孤立
func (s *Service) resetOrphanedAssets(ctx context.Context, repositoryID uint) error {
	storageIDs, err := s.GetRepositoryStorages(ctx, repositoryID)
	if err != nil {
		return err
	}

	if len(storageIDs) == 0 {
		return nil
	}

	// 查找该仓库的所有 Release
	var releaseIDs []uint
	if err := s.db.WithContext(ctx).
		Model(&models.Release{}).
		Where("repository_id = ?", repositoryID).
		Pluck("id", &releaseIDs).Error; err != nil {
		return err
	}
	if len(releaseIDs) == 0 {
		return nil
	}

	// 将 storage_id 不在 storageIDs 中的 verified/downloaded 资产重置为 pending
	result := s.db.WithContext(ctx).
		Model(&models.Asset{}).
		Where("release_id IN ? AND status IN ? AND (storage_id IS NULL OR storage_id NOT IN ?)",
			releaseIDs,
			[]models.AssetStatus{models.AssetStatusVerified, models.AssetStatusDownloaded},
			storageIDs).
		Updates(map[string]any{
			"status":        models.AssetStatusPending,
			"storage_path":  "",
			"sha256":        "",
			"storage_id":    gorm.Expr("NULL"),
			"downloaded_at": gorm.Expr("NULL"),
		})
	return result.Error
}
