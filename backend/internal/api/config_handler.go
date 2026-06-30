package api

import (
	"net/http"
	"time"

	"releasehub/backend/internal/config"

	"github.com/gin-gonic/gin"
)

type configHandler struct {
	config    *config.Config
	scheduler SchedulerUpdater
}

// SchedulerUpdater 调度器运行时配置更新接口
type SchedulerUpdater interface {
	UpdateInterval(interval time.Duration)
	UpdateMaxConcurrent(maxConcurrent int)
}

type configResponse struct {
	SchedulerEnabled       bool   `json:"schedulerEnabled"`
	SchedulerTickSeconds   int    `json:"schedulerTickSeconds"`
	SchedulerMaxConcurrent int    `json:"schedulerMaxConcurrent"`
	StorageDataDir         string `json:"storageDataDir"`
	GitHubAPIBaseURL       string `json:"githubApiBaseUrl"`
	AuthEnabled            bool   `json:"authEnabled"`
}

func registerConfigRoutes(router *gin.Engine, cfg *config.Config, scheduler SchedulerUpdater) {
	handler := &configHandler{config: cfg, scheduler: scheduler}

	group := router.Group("/api/config")
	group.GET("", handler.get)
	group.PUT("", handler.update)
}

func (h *configHandler) get(c *gin.Context) {
	c.JSON(http.StatusOK, configResponse{
		SchedulerEnabled:       h.config.Scheduler.Enabled,
		SchedulerTickSeconds:   h.config.Scheduler.TickSeconds,
		SchedulerMaxConcurrent: h.config.Scheduler.MaxConcurrent,
		StorageDataDir:         h.config.Storage.DataDir,
		GitHubAPIBaseURL:       h.config.GitHub.APIBaseURL,
		AuthEnabled:            h.config.Auth.Enabled,
	})
}

type configUpdateRequest struct {
	SchedulerEnabled       *bool   `json:"schedulerEnabled,omitempty"`
	SchedulerTickSeconds   *int    `json:"schedulerTickSeconds,omitempty"`
	SchedulerMaxConcurrent *int    `json:"schedulerMaxConcurrent,omitempty"`
	GitHubAPIBaseURL       *string `json:"githubApiBaseUrl,omitempty"`
	AuthEnabled            *bool   `json:"authEnabled,omitempty"`
}

func (h *configHandler) update(c *gin.Context) {
	var req configUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数无效: " + err.Error()})
		return
	}

	update := config.UpdateConfig{
		SchedulerEnabled:       req.SchedulerEnabled,
		SchedulerTickSeconds:   req.SchedulerTickSeconds,
		SchedulerMaxConcurrent: req.SchedulerMaxConcurrent,
		GitHubAPIBaseURL:       req.GitHubAPIBaseURL,
		AuthEnabled:            req.AuthEnabled,
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

	// 返回更新后的完整配置
	c.JSON(http.StatusOK, configResponse{
		SchedulerEnabled:       h.config.Scheduler.Enabled,
		SchedulerTickSeconds:   h.config.Scheduler.TickSeconds,
		SchedulerMaxConcurrent: h.config.Scheduler.MaxConcurrent,
		StorageDataDir:         h.config.Storage.DataDir,
		GitHubAPIBaseURL:       h.config.GitHub.APIBaseURL,
		AuthEnabled:            h.config.Auth.Enabled,
	})
}
