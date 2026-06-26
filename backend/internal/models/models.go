package models

import (
	"time"

	"gorm.io/gorm"
)

type RepositoryStatus string

const (
	RepositoryStatusUnknown RepositoryStatus = "unknown"
	RepositoryStatusHealthy RepositoryStatus = "healthy"
	RepositoryStatusFailed  RepositoryStatus = "failed"
)

type AssetStatus string

const (
	AssetStatusPending     AssetStatus = "pending"
	AssetStatusSkipped     AssetStatus = "skipped"
	AssetStatusDownloading AssetStatus = "downloading"
	AssetStatusDownloaded  AssetStatus = "downloaded"
	AssetStatusVerified    AssetStatus = "verified"
	AssetStatusFailed      AssetStatus = "failed"
	AssetStatusDeleted     AssetStatus = "deleted"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCanceled  TaskStatus = "canceled"
)

type GitHubToken struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"size:120;not null"`
	Token     string         `json:"-" gorm:"size:512;not null"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

type Storage struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"size:120;not null"`
	Type      string         `json:"type" gorm:"size:40;not null;default:local"`
	BasePath  string         `json:"basePath" gorm:"size:1024;not null"`
	IsDefault bool           `json:"isDefault" gorm:"not null;default:false"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

type Repository struct {
	ID                   uint             `json:"id" gorm:"primaryKey"`
	Provider             string           `json:"provider" gorm:"size:40;not null;default:github;uniqueIndex:idx_provider_owner_repo"`
	Owner                string           `json:"owner" gorm:"size:120;not null;uniqueIndex:idx_provider_owner_repo"`
	Repo                 string           `json:"repo" gorm:"size:120;not null;uniqueIndex:idx_provider_owner_repo"`
	Enabled              bool             `json:"enabled" gorm:"not null"`
	GitHubTokenID        *uint            `json:"githubTokenId"`
	StorageID            *uint            `json:"storageId"`
	IntervalSeconds      int              `json:"intervalSeconds" gorm:"not null;default:1800"`
	FilterMode           string           `json:"filterMode" gorm:"size:20;not null;default:glob"`
	AssetIncludePatterns string           `json:"assetIncludePatterns" gorm:"type:text"`
	AssetExcludePatterns string           `json:"assetExcludePatterns" gorm:"type:text"`
	RetentionKeepLatest  int              `json:"retentionKeepLatest" gorm:"not null;default:5"`
	LastCheckAt          *time.Time       `json:"lastCheckAt"`
	LastReleaseTag       string           `json:"lastReleaseTag" gorm:"size:255"`
	LastStatus           RepositoryStatus `json:"lastStatus" gorm:"size:40;not null;default:unknown"`
	CreatedAt            time.Time        `json:"createdAt"`
	UpdatedAt            time.Time        `json:"updatedAt"`
	DeletedAt            gorm.DeletedAt   `json:"-" gorm:"index"`
}

type Release struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	RepositoryID      uint           `json:"repositoryId" gorm:"not null;uniqueIndex:idx_repo_tag"`
	ProviderReleaseID int64          `json:"providerReleaseId"`
	Tag               string         `json:"tag" gorm:"size:255;not null;uniqueIndex:idx_repo_tag"`
	Name              string         `json:"name" gorm:"size:255"`
	PublishedAt       *time.Time     `json:"publishedAt"`
	Body              string         `json:"body" gorm:"type:text"`
	HTMLURL           string         `json:"htmlUrl" gorm:"size:1024"`
	APIURL            string         `json:"apiUrl" gorm:"size:1024"`
	IsLatest          bool           `json:"isLatest" gorm:"not null;default:false"`
	SyncStatus        string         `json:"syncStatus" gorm:"size:40;not null;default:pending"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"index"`
}

type Asset struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	ReleaseID          uint           `json:"releaseId" gorm:"not null;index;uniqueIndex:idx_release_asset_name"`
	ProviderAssetID    int64          `json:"providerAssetId"`
	Name               string         `json:"name" gorm:"size:512;not null;uniqueIndex:idx_release_asset_name"`
	Size               int64          `json:"size"`
	ContentType        string         `json:"contentType" gorm:"size:255"`
	DownloadURL        string         `json:"downloadUrl" gorm:"size:1024"`
	BrowserDownloadURL string         `json:"browserDownloadUrl" gorm:"size:1024"`
	StoragePath        string         `json:"storagePath" gorm:"size:1024"`
	SHA256             string         `json:"sha256" gorm:"size:64"`
	Status             AssetStatus    `json:"status" gorm:"size:40;not null;default:pending"`
	ErrorMessage       string         `json:"errorMessage" gorm:"type:text"`
	DownloadedAt       *time.Time     `json:"downloadedAt"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`
}

type Task struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	Type         string         `json:"type" gorm:"size:80;not null"`
	RepositoryID *uint          `json:"repositoryId" gorm:"index"`
	ReleaseID    *uint          `json:"releaseId" gorm:"index"`
	AssetID      *uint          `json:"assetId" gorm:"index"`
	Status       TaskStatus     `json:"status" gorm:"size:40;not null;default:pending"`
	Priority     int            `json:"priority" gorm:"not null;default:100"`
	Attempt      int            `json:"attempt" gorm:"not null;default:0"`
	MaxAttempts  int            `json:"maxAttempts" gorm:"not null;default:3"`
	ScheduledAt  *time.Time     `json:"scheduledAt"`
	StartedAt    *time.Time     `json:"startedAt"`
	FinishedAt   *time.Time     `json:"finishedAt"`
	ErrorMessage string         `json:"errorMessage" gorm:"type:text"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}
