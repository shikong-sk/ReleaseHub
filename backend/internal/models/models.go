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
	Name      string         `json:"name" gorm:"column:name;size:120;not null"`
	Token     string         `json:"-" gorm:"column:token;size:512;not null"`
	CreatedAt time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt time.Time      `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}

type Storage struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"column:name;size:120;not null"`
	Type      string         `json:"type" gorm:"column:type;size:40;not null;default:local"`
	BasePath  string         `json:"basePath" gorm:"column:base_path;size:1024;not null"`
	IsDefault bool           `json:"isDefault" gorm:"column:is_default;not null;default:false"`
	Endpoint  string         `json:"endpoint" gorm:"column:endpoint;size:1024"`
	Bucket    string         `json:"bucket" gorm:"column:bucket;size:255"`
	Region    string         `json:"region" gorm:"column:region;size:64"`
	AccessKey string         `json:"-" gorm:"column:access_key;size:512"`
	SecretKey string         `json:"-" gorm:"column:secret_key;size:512"`
	Username  string         `json:"username" gorm:"column:username;size:255"`
	Password  string         `json:"-" gorm:"column:password;size:512"`
	RemoteURL string         `json:"remoteUrl" gorm:"column:remote_url;size:1024"`
	CreatedAt time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt time.Time      `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}

type Repository struct {
	ID                   uint             `json:"id" gorm:"primaryKey"`
	Provider             string           `json:"provider" gorm:"column:provider;size:40;not null;default:github;uniqueIndex:idx_provider_owner_repo"`
	Owner                string           `json:"owner" gorm:"column:owner;size:120;not null;uniqueIndex:idx_provider_owner_repo"`
	Repo                 string           `json:"repo" gorm:"column:repo;size:120;not null;uniqueIndex:idx_provider_owner_repo"`
	Enabled              bool             `json:"enabled" gorm:"column:enabled;not null"`
	GitHubTokenID        *uint            `json:"githubTokenId" gorm:"column:github_token_id"`
	StorageID            *uint            `json:"storageId" gorm:"column:storage_id"`
	ProxyID             *uint            `json:"proxyId" gorm:"column:proxy_id"`
	IntervalSeconds      int              `json:"intervalSeconds" gorm:"column:interval_seconds;not null;default:1800"`
	FilterMode           string           `json:"filterMode" gorm:"column:filter_mode;size:20;not null;default:glob"`
	AssetIncludePatterns string           `json:"assetIncludePatterns" gorm:"column:asset_include_patterns;type:text"`
	AssetExcludePatterns string           `json:"assetExcludePatterns" gorm:"column:asset_exclude_patterns;type:text"`
	RetentionKeepLatest  int              `json:"retentionKeepLatest" gorm:"column:retention_keep_latest;not null;default:5"`
	LastCheckAt          *time.Time       `json:"lastCheckAt" gorm:"column:last_check_at"`
	LastReleaseTag       string           `json:"lastReleaseTag" gorm:"column:last_release_tag;size:255"`
	LastStatus           RepositoryStatus `json:"lastStatus" gorm:"column:last_status;size:40;not null;default:unknown"`
	CreatedAt            time.Time        `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt            time.Time        `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt            gorm.DeletedAt   `json:"-" gorm:"column:deleted_at;index"`
}

type Release struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	RepositoryID      uint           `json:"repositoryId" gorm:"column:repository_id;not null;uniqueIndex:idx_repo_tag"`
	ProviderReleaseID int64          `json:"providerReleaseId" gorm:"column:provider_release_id"`
	Tag               string         `json:"tag" gorm:"column:tag;size:255;not null;uniqueIndex:idx_repo_tag"`
	Name              string         `json:"name" gorm:"column:name;size:255"`
	PublishedAt       *time.Time     `json:"publishedAt" gorm:"column:published_at"`
	Body              string         `json:"body" gorm:"column:body;type:text"`
	HTMLURL           string         `json:"htmlUrl" gorm:"column:html_url;size:1024"`
	APIURL            string         `json:"apiUrl" gorm:"column:api_url;size:1024"`
	IsLatest          bool           `json:"isLatest" gorm:"column:is_latest;not null;default:false"`
	SyncStatus        string         `json:"syncStatus" gorm:"column:sync_status;size:40;not null;default:pending"`
	CreatedAt         time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt         time.Time      `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt         gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}

type Asset struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	ReleaseID          uint           `json:"releaseId" gorm:"column:release_id;not null;index;uniqueIndex:idx_release_asset_name"`
	ProviderAssetID    int64          `json:"providerAssetId" gorm:"column:provider_asset_id"`
	Name               string         `json:"name" gorm:"column:name;size:512;not null;uniqueIndex:idx_release_asset_name"`
	Size               int64          `json:"size" gorm:"column:size"`
	ContentType        string         `json:"contentType" gorm:"column:content_type;size:255"`
	DownloadURL        string         `json:"downloadUrl" gorm:"column:download_url;size:1024"`
	BrowserDownloadURL string         `json:"browserDownloadUrl" gorm:"column:browser_download_url;size:1024"`
	StoragePath        string         `json:"storagePath" gorm:"column:storage_path;size:1024"`
	SHA256             string         `json:"sha256" gorm:"column:sha256;size:64"`
	Status             AssetStatus    `json:"status" gorm:"column:status;size:40;not null;default:pending"`
	ErrorMessage       string         `json:"errorMessage" gorm:"column:error_message;type:text"`
	DownloadedAt       *time.Time     `json:"downloadedAt" gorm:"column:downloaded_at"`
	CreatedAt          time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt          time.Time      `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}

type Task struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	Type         string         `json:"type" gorm:"column:type;size:80;not null"`
	RepositoryID *uint          `json:"repositoryId" gorm:"column:repository_id;index"`
	ReleaseID    *uint          `json:"releaseId" gorm:"column:release_id;index"`
	AssetID      *uint          `json:"assetId" gorm:"column:asset_id;index"`
	Status       TaskStatus     `json:"status" gorm:"column:status;size:40;not null;default:pending"`
	Priority     int            `json:"priority" gorm:"column:priority;not null;default:100"`
	Attempt      int            `json:"attempt" gorm:"column:attempt;not null;default:0"`
	MaxAttempts  int            `json:"maxAttempts" gorm:"column:max_attempts;not null;default:3"`
	ScheduledAt  *time.Time     `json:"scheduledAt" gorm:"column:scheduled_at"`
	StartedAt    *time.Time     `json:"startedAt" gorm:"column:started_at"`
	FinishedAt   *time.Time     `json:"finishedAt" gorm:"column:finished_at"`
	ErrorMessage string         `json:"errorMessage" gorm:"column:error_message;type:text"`
	CreatedAt    time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt    time.Time      `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}

type Proxy struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"column:name;size:120;not null"`
	Type      string         `json:"type" gorm:"column:type;size:20;not null;default:http"`
	Host      string         `json:"host" gorm:"column:host;size:512;not null"`
	Port      int            `json:"port" gorm:"column:port;not null"`
	Username  string         `json:"username" gorm:"column:username;size:255"`
	Password  string         `json:"-" gorm:"column:password;size:512"`
	CreatedAt time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt time.Time      `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}

type Notification struct {
	ID        uint           `json:"id" gorm:"primaryKey"`
	Name      string         `json:"name" gorm:"column:name;size:120;not null"`
	Type      string         `json:"type" gorm:"column:type;size:20;not null"`
	ServerURL string         `json:"serverUrl" gorm:"column:server_url;size:1024"`
	Token     string         `json:"-" gorm:"column:token;size:512"`
	Enabled   bool           `json:"enabled" gorm:"column:enabled;not null;default:true"`
	Events    string         `json:"events" gorm:"column:events;type:text"`
	CreatedAt time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt time.Time      `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}

type TaskLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TaskID    uint      `json:"taskId" gorm:"column:task_id;not null;index"`
	Level     string    `json:"level" gorm:"column:level;size:20;not null;default:info"`
	Message   string    `json:"message" gorm:"column:message;type:text;not null"`
	Timestamp time.Time `json:"timestamp" gorm:"column:timestamp;not null;index"`
}

type User struct {
	ID           uint           `json:"id" gorm:"primaryKey"`
	Username     string         `json:"username" gorm:"column:username;size:120;not null;uniqueIndex"`
	PasswordHash string         `json:"-" gorm:"column:password_hash;size:512;not null"`
	Role         string         `json:"role" gorm:"column:role;size:40;not null;default:viewer"`
	Enabled      bool           `json:"enabled" gorm:"column:enabled;not null;default:true"`
	LastLoginAt  *time.Time     `json:"lastLoginAt" gorm:"column:last_login_at"`
	CreatedAt    time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt    time.Time      `json:"updatedAt" gorm:"column:updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"column:deleted_at;index"`
}
