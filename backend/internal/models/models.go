package models

import (
	"time"

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
}

type Repository struct {
	ID                   uint             `json:"id" gorm:"primaryKey"`
	Provider             string           `json:"provider" gorm:"column:provider;size:40;not null;default:github;uniqueIndex:idx_provider_owner_repo"`
	Owner                string           `json:"owner" gorm:"column:owner;size:120;not null;uniqueIndex:idx_provider_owner_repo"`
	Repo                 string           `json:"repo" gorm:"column:repo;size:120;not null;uniqueIndex:idx_provider_owner_repo"`
	Enabled              bool             `json:"enabled" gorm:"column:enabled;not null"`
	GitHubTokenID        *uint            `json:"githubTokenId" gorm:"column:github_token_id"`
	StorageID            *uint            `json:"storageId" gorm:"column:storage_id"`
	ProxyID              *uint            `json:"proxyId" gorm:"column:proxy_id"`
	ProviderApiBaseUrl   string           `json:"providerApiBaseUrl" gorm:"column:provider_api_base_url;size:1024"`
	IntervalSeconds      int              `json:"intervalSeconds" gorm:"column:interval_seconds;not null;default:1800"`
	FilterMode           string           `json:"filterMode" gorm:"column:filter_mode;size:20;not null;default:glob"`
	AssetIncludePatterns string           `json:"assetIncludePatterns" gorm:"column:asset_include_patterns;type:text"`
	AssetExcludePatterns string           `json:"assetExcludePatterns" gorm:"column:asset_exclude_patterns;type:text"`
	TagFilterMode        string           `json:"tagFilterMode" gorm:"column:tag_filter_mode;size:20;not null;default:''"`
	TagIncludePattern    string           `json:"tagIncludePattern" gorm:"column:tag_include_pattern;type:text"`
	TagExcludePattern    string           `json:"tagExcludePattern" gorm:"column:tag_exclude_pattern;type:text"`
	RetentionKeepLatest  int              `json:"retentionKeepLatest" gorm:"column:retention_keep_latest;not null;default:5"`
	LastCheckAt          *time.Time       `json:"lastCheckAt" gorm:"column:last_check_at"`
	LastReleaseTag       string           `json:"lastReleaseTag" gorm:"column:last_release_tag;size:255"`
	LastStatus           RepositoryStatus `json:"lastStatus" gorm:"column:last_status;size:40;not null;default:unknown"`
	CreatedAt            time.Time        `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt            time.Time        `json:"updatedAt" gorm:"column:updated_at"`
	// 非数据库字段，API 响应时填充
	StorageIDs           []uint           `json:"storageIds" gorm:"-"`
	TotalStorageBytes    int64            `json:"totalStorageBytes" gorm:"-"`
}

// RepositoryStorage 仓库-存储多对多关联
type RepositoryStorage struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	RepositoryID uint      `json:"repositoryId" gorm:"column:repository_id;not null;uniqueIndex:idx_repo_storage"`
	StorageID    uint      `json:"storageId" gorm:"column:storage_id;not null;uniqueIndex:idx_repo_storage"`
	CreatedAt    time.Time `json:"createdAt" gorm:"column:created_at"`
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
	IsPinned          bool           `json:"isPinned" gorm:"column:is_pinned;not null;default:false"`
	SyncStatus        string         `json:"syncStatus" gorm:"column:sync_status;size:40;not null;default:pending"`
	CreatedAt         time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt         time.Time      `json:"updatedAt" gorm:"column:updated_at"`
}

type Asset struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	ReleaseID          uint           `json:"releaseId" gorm:"column:release_id;not null;uniqueIndex:idx_release_asset_storage"`
	ProviderAssetID    int64          `json:"providerAssetId" gorm:"column:provider_asset_id"`
	Name               string         `json:"name" gorm:"column:name;size:512;not null;uniqueIndex:idx_release_asset_storage"`
	Size               int64          `json:"size" gorm:"column:size"`
	ContentType        string         `json:"contentType" gorm:"column:content_type;size:255"`
	DownloadURL        string         `json:"downloadUrl" gorm:"column:download_url;size:1024"`
	BrowserDownloadURL string         `json:"browserDownloadUrl" gorm:"column:browser_download_url;size:1024"`
	StoragePath        string         `json:"storagePath" gorm:"column:storage_path;size:1024"`
	SHA256             string         `json:"sha256" gorm:"column:sha256;size:64"`
	ExpectedSHA256     string         `json:"expectedSha256" gorm:"column:expected_sha256;size:64"`
	DownloadBytes      int64          `json:"downloadBytes" gorm:"column:download_bytes;not null;default:0"`
	Status             AssetStatus    `json:"status" gorm:"column:status;size:40;not null;default:pending"`
	StorageID          *uint          `json:"storageId" gorm:"column:storage_id;uniqueIndex:idx_release_asset_storage"`
	ErrorMessage       string         `json:"errorMessage" gorm:"column:error_message;type:text"`
	DownloadedAt       *time.Time     `json:"downloadedAt" gorm:"column:downloaded_at"`
	CreatedAt          time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt          time.Time      `json:"updatedAt" gorm:"column:updated_at"`
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
}

type Proxy struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	Name              string         `json:"name" gorm:"column:name;size:120;not null"`
	Type              string         `json:"type" gorm:"column:type;size:20;not null;default:http"`
	Host              string         `json:"host" gorm:"column:host;size:512;not null"`
	Port              int            `json:"port" gorm:"column:port;not null"`
	Username          string         `json:"username" gorm:"column:username;size:255"`
	Password          string         `json:"-" gorm:"column:password;size:512"`
	TestURL           string         `json:"testUrl" gorm:"column:test_url;size:1024"`
	LastTestStatus    string         `json:"lastTestStatus" gorm:"column:last_test_status;size:40"`
	LastTestMessage   string         `json:"lastTestMessage" gorm:"column:last_test_message;type:text"`
	LastTestLatencyMs int64          `json:"lastTestLatencyMs" gorm:"column:last_test_latency_ms"`
	LastTestedAt      *time.Time     `json:"lastTestedAt" gorm:"column:last_tested_at"`
	CreatedAt         time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt         time.Time      `json:"updatedAt" gorm:"column:updated_at"`
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
}

type APIKey struct {
	ID         uint           `json:"id" gorm:"primaryKey"`
	Name       string         `json:"name" gorm:"column:name;size:120;not null"`
	Key        string         `json:"-" gorm:"column:key;size:128;not null;uniqueIndex"`
	KeyHint    string         `json:"keyHint" gorm:"column:key_hint;size:32"`
	Scope      string         `json:"scope" gorm:"column:scope;size:255;not null;default:*"`
	UserID     *uint          `json:"userId" gorm:"column:user_id"`
	Enabled    bool           `json:"enabled" gorm:"column:enabled;not null;default:true"`
	LastUsedAt *time.Time     `json:"lastUsedAt" gorm:"column:last_used_at"`
	CreatedAt  time.Time      `json:"createdAt" gorm:"column:created_at"`
	UpdatedAt  time.Time      `json:"updatedAt" gorm:"column:updated_at"`
}

// NotificationLog 通知推送历史记录
type NotificationLog struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	NotificationID uint      `json:"notificationId" gorm:"column:notification_id;not null;index"`
	NotificationName string  `json:"notificationName" gorm:"column:notification_name;size:120"`
	Event          string    `json:"event" gorm:"column:event;size:40;not null"`
	Title          string    `json:"title" gorm:"column:title;size:512"`
	Message        string    `json:"message" gorm:"column:message;type:text"`
	Success        bool      `json:"success" gorm:"column:success;not null;default:false"`
	Error          string    `json:"error" gorm:"column:error;type:text"`
	CreatedAt      time.Time `json:"createdAt" gorm:"column:created_at;index"`
}

// AppSetting 应用配置键值表（持久化运行时可修改的配置）
type AppSetting struct {
	Key   string `json:"key" gorm:"primaryKey;size:64"`
	Value string `json:"value" gorm:"column:value;size:512;not null;default:''"`
}
