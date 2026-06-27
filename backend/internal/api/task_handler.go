package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/tasklog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type taskHandler struct {
	db         *gorm.DB
	logService *tasklog.Service
}

type taskResponse struct {
	ID             uint              `json:"id"`
	Type           string            `json:"type"`
	RepositoryID   *uint             `json:"repositoryId"`
	RepositoryName string            `json:"repositoryName"`
	ReleaseID      *uint             `json:"releaseId"`
	ReleaseTag     string            `json:"releaseTag"`
	AssetID        *uint             `json:"assetId"`
	AssetName      string            `json:"assetName"`
	StoragePath    string            `json:"storagePath"`
	Status         models.TaskStatus `json:"status"`
	Priority       int               `json:"priority"`
	Attempt        int               `json:"attempt"`
	MaxAttempts    int               `json:"maxAttempts"`
	ScheduledAt    *time.Time        `json:"scheduledAt"`
	StartedAt      *time.Time        `json:"startedAt"`
	FinishedAt     *time.Time        `json:"finishedAt"`
	ErrorMessage   string            `json:"errorMessage"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}

func registerTaskRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &taskHandler{
		db:         db,
		logService: tasklog.NewService(db),
	}

	group := router.Group("/api/tasks")
	group.GET("", handler.list)
	group.GET("/:id", handler.get)
	group.GET("/:id/logs", handler.logs)
}

func (h *taskHandler) list(c *gin.Context) {
	var tasks []models.Task
	if err := h.db.WithContext(c.Request.Context()).
		Order("created_at DESC").
		Limit(200).
		Find(&tasks).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询任务失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": h.buildTaskResponses(c.Request.Context(), tasks),
	})
}

func (h *taskHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var task models.Task
	if err := h.db.WithContext(c.Request.Context()).First(&task, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "任务不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询任务失败")
		return
	}

	c.JSON(http.StatusOK, h.buildTaskResponse(c.Request.Context(), task))
}

// logs 返回指定任务的日志
func (h *taskHandler) logs(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	limit := 100
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = l
	}

	logs, err := h.logService.List(c.Request.Context(), id, limit)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "查询任务日志失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": logs,
	})
}

func (h *taskHandler) buildTaskResponses(ctx context.Context, tasks []models.Task) []taskResponse {
	items := make([]taskResponse, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, h.buildTaskResponse(ctx, task))
	}
	return items
}

func (h *taskHandler) buildTaskResponse(ctx context.Context, task models.Task) taskResponse {
	resp := taskResponse{
		ID:           task.ID,
		Type:         task.Type,
		RepositoryID: task.RepositoryID,
		ReleaseID:    task.ReleaseID,
		AssetID:      task.AssetID,
		Status:       task.Status,
		Priority:     task.Priority,
		Attempt:      task.Attempt,
		MaxAttempts:  task.MaxAttempts,
		ScheduledAt:  task.ScheduledAt,
		StartedAt:    task.StartedAt,
		FinishedAt:   task.FinishedAt,
		ErrorMessage: task.ErrorMessage,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}

	if task.RepositoryID != nil {
		var repo models.Repository
		if err := h.db.WithContext(ctx).First(&repo, *task.RepositoryID).Error; err == nil {
			resp.RepositoryName = repo.Owner + "/" + repo.Repo
		}
	}
	if task.ReleaseID != nil {
		var release models.Release
		if err := h.db.WithContext(ctx).First(&release, *task.ReleaseID).Error; err == nil {
			resp.ReleaseTag = release.Tag
			if resp.RepositoryName == "" {
				var repo models.Repository
				if err := h.db.WithContext(ctx).First(&repo, release.RepositoryID).Error; err == nil {
					resp.RepositoryName = repo.Owner + "/" + repo.Repo
				}
			}
		}
	}
	if task.AssetID != nil {
		var asset models.Asset
		if err := h.db.WithContext(ctx).First(&asset, *task.AssetID).Error; err == nil {
			resp.AssetName = asset.Name
			resp.StoragePath = asset.StoragePath
			if resp.ReleaseTag == "" {
				var release models.Release
				if err := h.db.WithContext(ctx).First(&release, asset.ReleaseID).Error; err == nil {
					resp.ReleaseTag = release.Tag
					if resp.RepositoryName == "" {
						var repo models.Repository
						if err := h.db.WithContext(ctx).First(&repo, release.RepositoryID).Error; err == nil {
							resp.RepositoryName = repo.Owner + "/" + repo.Repo
						}
					}
				}
			}
		}
	}

	return resp
}
