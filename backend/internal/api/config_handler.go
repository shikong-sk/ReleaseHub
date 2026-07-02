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
}

func registerConfigRoutes(router *gin.Engine, cfg *config.Config, scheduler SchedulerUpdater, syncer SyncerUpdater, db *gorm.DB) {
	handler := &configHandler{config: cfg, scheduler: scheduler, syncer: syncer, db: db}

	group := router.Group("/api/config")
	group.GET("", handler.get)
	group.PUT("", handler.update)
}

// LoadPersistedSettings 从数据库加载持久化的配置到 Config
func LoadPersistedSettings(db *gorm.DB, cfg *config.Config) {
	var setting models.AppSetting
	if err := db.Where("key = ?", "auth.enabled").First(&setting).Error; err == nil {
		cfg.Auth.Enabled = setting.Value == "true"
	}
	if err := db.Where("key = ?", "syncer.max_concurrent_tasks").First(&setting).Error; err == nil {
		if n, perr := strconv.Atoi(setting.Value); perr == nil && n >= 1 {
			cfg.Syncer.MaxConcurrentTasks = n
		}
	}
	if err := db.Where("key = ?", "syncer.max_concurrent_downloads").First(&setting).Error; err == nil {
		if n, perr := strconv.Atoi(setting.Value); perr == nil && n >= 1 {
			cfg.Syncer.MaxConcurrentDownloads = n
		}
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
		switch field {
		case "authEnabled":
			val := "false"
			if h.config.Auth.Enabled {
				val = "true"
			}
			h.db.Save(&models.AppSetting{Key: "auth.enabled", Value: val})
		case "syncerMaxConcurrentTasks":
			h.db.Save(&models.AppSetting{Key: "syncer.max_concurrent_tasks", Value: strconv.Itoa(h.config.Syncer.MaxConcurrentTasks)})
		case "syncerMaxConcurrentDownloads":
			h.db.Save(&models.AppSetting{Key: "syncer.max_concurrent_downloads", Value: strconv.Itoa(h.config.Syncer.MaxConcurrentDownloads)})
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
	})
}
