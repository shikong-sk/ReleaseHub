package api

import (
	"net/http"
	"strconv"
	"time"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type configHandler struct {
	config    *config.Config
	scheduler SchedulerUpdater
	syncer    SyncerUpdater
	db        *gorm.DB
}

// SchedulerUpdater 调度器运行时配置更新接口
type SchedulerUpdater interface {
	UpdateInterval(interval time.Duration)
	UpdateMaxConcurrent(maxConcurrent int)
}

// SyncerUpdater 同步器运行时配置更新接口
type SyncerUpdater interface {
	UpdateMaxConcurrentTasks(n int)
	UpdateMaxConcurrentDownloads(n int)
}

type configResponse struct {
	SchedulerEnabled             bool   `json:"schedulerEnabled"`
	SchedulerTickSeconds         int    `json:"schedulerTickSeconds"`
	SchedulerMaxConcurrent       int    `json:"schedulerMaxConcurrent"`
	StorageDataDir               string `json:"storageDataDir"`
	GitHubAPIBaseURL             string `json:"githubApiBaseUrl"`
	AuthEnabled                  bool   `json:"authEnabled"`
	SyncerMaxConcurrentTasks     int    `json:"syncerMaxConcurrentTasks"`
	SyncerMaxConcurrentDownloads int    `json:"syncerMaxConcurrentDownloads"`
	TaskLogRetentionDays         int    `json:"taskLogRetentionDays"`
	OperationLogRetentionDays    int    `json:"operationLogRetentionDays"`
	DownloadMaxSpeedBytes        int64  `json:"downloadMaxSpeedBytes"`
	Aria2RPC                     string `json:"aria2RPC"`
	Aria2Secret                  string `json:"aria2Secret"`
	Aria2HTTP                    string `json:"aria2HTTP"`
	Aria2Dir                     string `json:"aria2Dir"`
}

func registerConfigRoutes(router *gin.Engine, cfg *config.Config, scheduler SchedulerUpdater, syncer SyncerUpdater, db *gorm.DB) {
	handler := &configHandler{config: cfg, scheduler: scheduler, syncer: syncer, db: db}

	group := router.Group("/api/config")
	group.GET("", handler.get)
	group.PUT("", handler.update)
}

// LoadPersistedSettings 从数据库加载持久化的配置到 Config
//
// 注意：每个查询必须使用独立的局部变量，不能复用同一个 models.AppSetting。
// 因为 AppSetting 的 Key 是 primaryKey，GORM 的 First(&setting) 成功后会把主键
// 填入变量，复用时 GORM 会把残留主键追加为查询条件，导致后续查询永远查不到记录。
func LoadPersistedSettings(db *gorm.DB, cfg *config.Config) {
	loadBool := func(key string) (string, bool) {
		var s models.AppSetting
		err := db.Where("key = ?", key).First(&s).Error
		return s.Value, err == nil
	}
	// 注意：每个查询独立变量，不能复用（见下方注释）
	loadStr := func(key string) (string, bool) {
		var s models.AppSetting
		err := db.Where("key = ?", key).First(&s).Error
		return s.Value, err == nil
	}
	loadInt := func(key string, min int) (int, bool) {
		var s models.AppSetting
		if err := db.Where("key = ?", key).First(&s).Error; err == nil {
			if n, perr := strconv.Atoi(s.Value); perr == nil && n >= min {
				return n, true
			}
		}
		return 0, false
	}

	loadInt64 := func(key string, min int64) (int64, bool) {
		var s models.AppSetting
		if err := db.Where("key = ?", key).First(&s).Error; err == nil {
			if n, perr := strconv.ParseInt(s.Value, 10, 64); perr == nil && n >= min {
				return n, true
			}
		}
		return 0, false
	}

	if v, ok := loadBool("auth.enabled"); ok {
		cfg.Auth.Enabled = v == "true"
	}
	if v, ok := loadInt("syncer.max_concurrent_tasks", 1); ok {
		cfg.Syncer.MaxConcurrentTasks = v
	}
	if v, ok := loadInt("syncer.max_concurrent_downloads", 1); ok {
		cfg.Syncer.MaxConcurrentDownloads = v
	}
	if v, ok := loadInt("tasklog.retention_days", 0); ok {
		cfg.TaskLog.RetentionDays = v
	}
	if v, ok := loadInt("oplog.retention_days", 0); ok {
		cfg.OpLog.RetentionDays = v
	}
	if v, ok := loadBool("scheduler.enabled"); ok {
		cfg.Scheduler.Enabled = v == "true"
	}
	if v, ok := loadInt("scheduler.tick_seconds", 10); ok {
		cfg.Scheduler.TickSeconds = v
	}
	if v, ok := loadInt("scheduler.max_concurrent", 1); ok {
		cfg.Scheduler.MaxConcurrent = v
	}
	if v, ok := loadStr("github.api_base_url"); ok && v != "" {
		cfg.GitHub.APIBaseURL = v
	}
	if v, ok := loadInt64("download.max_speed_bytes", 0); ok {
		cfg.Download.MaxSpeedBytes = v
	}
	if v, ok := loadStr("download.aria2_rpc"); ok && v != "" {
		cfg.Download.Aria2RPC = v
	}
	if v, ok := loadStr("download.aria2_secret"); ok && v != "" {
		cfg.Download.Aria2Secret = v
	}
	if v, ok := loadStr("download.aria2_http"); ok && v != "" {
		cfg.Download.Aria2HTTP = v
	}
	if v, ok := loadStr("download.aria2_dir"); ok && v != "" {
		cfg.Download.Aria2Dir = v
	}
}

func (h *configHandler) get(c *gin.Context) {
	c.JSON(http.StatusOK, configResponse{
		SchedulerEnabled:             h.config.Scheduler.Enabled,
		SchedulerTickSeconds:         h.config.Scheduler.TickSeconds,
		SchedulerMaxConcurrent:       h.config.Scheduler.MaxConcurrent,
		StorageDataDir:               h.config.Storage.DataDir,
		GitHubAPIBaseURL:             h.config.GitHub.APIBaseURL,
		AuthEnabled:                  h.config.Auth.Enabled,
		SyncerMaxConcurrentTasks:     h.config.Syncer.MaxConcurrentTasks,
		SyncerMaxConcurrentDownloads: h.config.Syncer.MaxConcurrentDownloads,
		TaskLogRetentionDays:         h.config.TaskLog.RetentionDays,
		OperationLogRetentionDays:    h.config.OpLog.RetentionDays,
		DownloadMaxSpeedBytes:        h.config.Download.MaxSpeedBytes,
		Aria2RPC:                     h.config.Download.Aria2RPC,
		Aria2Secret:                  h.config.Download.Aria2Secret,
		Aria2HTTP:                    h.config.Download.Aria2HTTP,
		Aria2Dir:                     h.config.Download.Aria2Dir,
	})
}

type configUpdateRequest struct {
	SchedulerEnabled             *bool   `json:"schedulerEnabled,omitempty"`
	SchedulerTickSeconds         *int    `json:"schedulerTickSeconds,omitempty"`
	SchedulerMaxConcurrent       *int    `json:"schedulerMaxConcurrent,omitempty"`
	GitHubAPIBaseURL             *string `json:"githubApiBaseUrl,omitempty"`
	AuthEnabled                  *bool   `json:"authEnabled,omitempty"`
	SyncerMaxConcurrentTasks     *int    `json:"syncerMaxConcurrentTasks,omitempty"`
	SyncerMaxConcurrentDownloads *int    `json:"syncerMaxConcurrentDownloads,omitempty"`
	TaskLogRetentionDays         *int    `json:"taskLogRetentionDays,omitempty"`
	OperationLogRetentionDays    *int `json:"operationLogRetentionDays,omitempty"`
	DownloadMaxSpeedBytes       *int64 `json:"downloadMaxSpeedBytes,omitempty"`
	Aria2RPC                    *string `json:"aria2RPC,omitempty"`
	Aria2Secret                 *string `json:"aria2Secret,omitempty"`
	Aria2HTTP                   *string `json:"aria2HTTP,omitempty"`
	Aria2Dir                    *string `json:"aria2Dir,omitempty"`
}

func (h *configHandler) update(c *gin.Context) {
	var req configUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	update := config.UpdateConfig{
		SchedulerEnabled:             req.SchedulerEnabled,
		SchedulerTickSeconds:         req.SchedulerTickSeconds,
		SchedulerMaxConcurrent:       req.SchedulerMaxConcurrent,
		GitHubAPIBaseURL:             req.GitHubAPIBaseURL,
		AuthEnabled:                  req.AuthEnabled,
		SyncerMaxConcurrentTasks:     req.SyncerMaxConcurrentTasks,
		SyncerMaxConcurrentDownloads: req.SyncerMaxConcurrentDownloads,
		TaskLogRetentionDays:         req.TaskLogRetentionDays,
		OperationLogRetentionDays:    req.OperationLogRetentionDays,
		DownloadMaxSpeedBytes:        req.DownloadMaxSpeedBytes,
		Aria2RPC:                     req.Aria2RPC,
		Aria2Secret:                  req.Aria2Secret,
		Aria2HTTP:                    req.Aria2HTTP,
		Aria2Dir:                     req.Aria2Dir,
	}

	changed, err := h.config.ApplyUpdate(update)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 同步更新调度器运行时参数
	if h.scheduler != nil {
		for _, field := range changed {
			switch field {
			case "schedulerTickSeconds":
				h.scheduler.UpdateInterval(time.Duration(h.config.Scheduler.TickSeconds) * time.Second)
			case "schedulerMaxConcurrent":
				h.scheduler.UpdateMaxConcurrent(h.config.Scheduler.MaxConcurrent)
			}
		}
	}

	// 同步更新同步器运行时参数
	if h.syncer != nil {
		for _, field := range changed {
			switch field {
			case "syncerMaxConcurrentTasks":
				h.syncer.UpdateMaxConcurrentTasks(h.config.Syncer.MaxConcurrentTasks)
			case "syncerMaxConcurrentDownloads":
				h.syncer.UpdateMaxConcurrentDownloads(h.config.Syncer.MaxConcurrentDownloads)
			}
		}
	}

	// 持久化到数据库
	for _, field := range changed {
		var setting models.AppSetting
		switch field {
		case "authEnabled":
			val := "false"
			if h.config.Auth.Enabled {
				val = "true"
			}
			setting = models.AppSetting{Key: "auth.enabled", Value: val}
		case "syncerMaxConcurrentTasks":
			setting = models.AppSetting{Key: "syncer.max_concurrent_tasks", Value: strconv.Itoa(h.config.Syncer.MaxConcurrentTasks)}
		case "syncerMaxConcurrentDownloads":
			setting = models.AppSetting{Key: "syncer.max_concurrent_downloads", Value: strconv.Itoa(h.config.Syncer.MaxConcurrentDownloads)}
		case "taskLogRetentionDays":
			setting = models.AppSetting{Key: "tasklog.retention_days", Value: strconv.Itoa(h.config.TaskLog.RetentionDays)}
		case "operationLogRetentionDays":
			setting = models.AppSetting{Key: "oplog.retention_days", Value: strconv.Itoa(h.config.OpLog.RetentionDays)}
		case "schedulerEnabled":
			val := "false"
			if h.config.Scheduler.Enabled {
				val = "true"
			}
			setting = models.AppSetting{Key: "scheduler.enabled", Value: val}
		case "schedulerTickSeconds":
			setting = models.AppSetting{Key: "scheduler.tick_seconds", Value: strconv.Itoa(h.config.Scheduler.TickSeconds)}
		case "schedulerMaxConcurrent":
			setting = models.AppSetting{Key: "scheduler.max_concurrent", Value: strconv.Itoa(h.config.Scheduler.MaxConcurrent)}
		case "githubApiBaseUrl":
			setting = models.AppSetting{Key: "github.api_base_url", Value: h.config.GitHub.APIBaseURL}
		case "downloadMaxSpeedBytes":
			setting = models.AppSetting{Key: "download.max_speed_bytes", Value: strconv.FormatInt(h.config.Download.MaxSpeedBytes, 10)}
		case "aria2RPC":
			setting = models.AppSetting{Key: "download.aria2_rpc", Value: h.config.Download.Aria2RPC}
		case "aria2Secret":
			setting = models.AppSetting{Key: "download.aria2_secret", Value: h.config.Download.Aria2Secret}
		case "aria2HTTP":
			setting = models.AppSetting{Key: "download.aria2_http", Value: h.config.Download.Aria2HTTP}
		case "aria2Dir":
			setting = models.AppSetting{Key: "download.aria2_dir", Value: h.config.Download.Aria2Dir}
		default:
			continue
		}
		if err := h.db.Save(&setting).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "配置持久化失败: "+err.Error())
			return
		}
	}

	// 返回更新后的完整配置
	c.JSON(http.StatusOK, configResponse{
		SchedulerEnabled:             h.config.Scheduler.Enabled,
		SchedulerTickSeconds:         h.config.Scheduler.TickSeconds,
		SchedulerMaxConcurrent:       h.config.Scheduler.MaxConcurrent,
		StorageDataDir:               h.config.Storage.DataDir,
		GitHubAPIBaseURL:             h.config.GitHub.APIBaseURL,
		AuthEnabled:                  h.config.Auth.Enabled,
		SyncerMaxConcurrentTasks:     h.config.Syncer.MaxConcurrentTasks,
		SyncerMaxConcurrentDownloads: h.config.Syncer.MaxConcurrentDownloads,
		TaskLogRetentionDays:         h.config.TaskLog.RetentionDays,
		OperationLogRetentionDays:    h.config.OpLog.RetentionDays,
		DownloadMaxSpeedBytes:        h.config.Download.MaxSpeedBytes,
		Aria2RPC:                     h.config.Download.Aria2RPC,
		Aria2Secret:                  h.config.Download.Aria2Secret,
		Aria2HTTP:                    h.config.Download.Aria2HTTP,
		Aria2Dir:                     h.config.Download.Aria2Dir,
	})
}
