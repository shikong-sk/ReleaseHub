package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/provider"

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
	db *gorm.DB
}

type CreateInput struct {
	Provider             string `json:"provider"`
	Owner                string `json:"owner"`
	Repo                 string `json:"repo"`
	Enabled              *bool  `json:"enabled"`
	GitHubTokenID        *uint  `json:"githubTokenId"`
	StorageID            *uint  `json:"storageId"`
	ProxyID              *uint  `json:"proxyId"`
	ProviderApiBaseUrl    string `json:"providerApiBaseUrl"`
	IntervalSeconds      int    `json:"intervalSeconds"`
	FilterMode           string `json:"filterMode"`
	AssetIncludePatterns string `json:"assetIncludePatterns"`
	AssetExcludePatterns string `json:"assetExcludePatterns"`
	RetentionKeepLatest  int    `json:"retentionKeepLatest"`
}

type UpdateInput struct {
	Enabled              *bool   `json:"enabled"`
	GitHubTokenID        *uint   `json:"githubTokenId"`
	StorageID            *uint  `json:"storageId"`
	ProxyID              *uint  `json:"proxyId"`
	ProviderApiBaseUrl    *string `json:"providerApiBaseUrl"`
	IntervalSeconds      *int    `json:"intervalSeconds"`
	FilterMode           *string `json:"filterMode"`
	AssetIncludePatterns *string `json:"assetIncludePatterns"`
	AssetExcludePatterns *string `json:"assetExcludePatterns"`
	RetentionKeepLatest  *int    `json:"retentionKeepLatest"`
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
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
	return repositories, err
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
	if input.StorageID != nil {
		repository.StorageID = input.StorageID
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
	if input.RetentionKeepLatest != nil {
		if err := validateRetention(*input.RetentionKeepLatest); err != nil {
			return nil, err
		}
		repository.RetentionKeepLatest = *input.RetentionKeepLatest
	}

	if err := s.db.WithContext(ctx).Save(repository).Error; err != nil {
		return nil, err
	}

	return repository, nil
}

func (s *Service) Delete(ctx context.Context, id uint) error {
	repository, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Delete(repository).Error
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
