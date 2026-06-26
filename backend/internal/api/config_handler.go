package api

import (
	"net/http"

	"releasehub/backend/internal/config"

	"github.com/gin-gonic/gin"
)

type configHandler struct {
	config *config.Config
}

type configResponse struct {
	SchedulerEnabled       bool   `json:"schedulerEnabled"`
	SchedulerTickSeconds   int    `json:"schedulerTickSeconds"`
	SchedulerMaxConcurrent int    `json:"schedulerMaxConcurrent"`
	StorageDataDir         string `json:"storageDataDir"`
	GitHubAPIBaseURL       string `json:"githubApiBaseUrl"`
	AuthEnabled            bool   `json:"authEnabled"`
}

func registerConfigRoutes(router *gin.Engine, cfg *config.Config) {
	handler := &configHandler{config: cfg}

	group := router.Group("/api/config")
	group.GET("", handler.get)
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
