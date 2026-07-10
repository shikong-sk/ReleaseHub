package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"releasehub/backend/internal/models"
	assetsvc "releasehub/backend/internal/services/asset"
	"releasehub/backend/internal/services/tasklog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SyncerProgressProvider 同步器进度查询接口（供 task list API 注入实时下载进度）
type SyncerProgressProvider interface {
	ActiveProgresses() map[uint]assetsvc.DownloadProgress
}

type taskHandler struct {
	db          *gorm.DB
	logService  *tasklog.Service
	progress    SyncerProgressProvider // 可空，无下载进行时为 nil
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
	// 下载进度（仅 download_asset / sync_release 类型且正在下载时有值）
	DownloadedBytes int64 `json:"downloadedBytes"`
	TotalBytes      int64 `json:"totalBytes"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func registerTaskRoutes(router *gin.Engine, db *gorm.DB, progress SyncerProgressProvider) {
	handler := &taskHandler{
		db:         db,
		logService: tasklog.NewService(db),
		progress:   progress,
	}

	group := router.Group("/api/tasks")
	group.GET("", handler.list)
	group.GET("/:id", handler.get)
	group.GET("/:id/logs", handler.logs)
}

func (h *taskHandler) list(c *gin.Context) {
	ctx := c.Request.Context()
	query := h.db.WithContext(ctx).Model(&models.Task{})

	// 状态筛选
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}
	// 类型筛选
	if taskType := c.Query("type"); taskType != "" {
		query = query.Where("type = ?", taskType)
	}
	// 仓库筛选
	if repoID := c.Query("repositoryId"); repoID != "" {
		if id, err := strconv.ParseUint(repoID, 10, 64); err == nil && id > 0 {
			query = query.Where("repository_id = ?", id)
		}
	}
	// 关键字搜索（仓库名 / 资产名 / 错误信息）。仓库名需关联 Repository 表子查询匹配
	if keyword := c.Query("keyword"); keyword != "" {
		like := "%" + keyword + "%"
		var repoIDs []uint
		h.db.WithContext(ctx).Model(&models.Repository{}).
			Where("owner LIKE ? OR repo LIKE ?", like, like).
			Pluck("id", &repoIDs)
		var conds []string
		var params []any
		// 错误信息直接在 tasks 表
		conds = append(conds, "error_message LIKE ?")
		params = append(params, like)
		if len(repoIDs) > 0 {
			conds = append(conds, "repository_id IN ?")
			params = append(params, repoIDs)
		}
		query = query.Where(strings.Join(conds, " OR "), params...)
	}

	// 分页：默认 200，上限 500
	pageSize := 200
	if ps := c.Query("pageSize"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 && n <= 500 {
			pageSize = n
		}
	}
	page := 1
	if pg := c.Query("page"); pg != "" {
		if n, err := strconv.Atoi(pg); err == nil && n > 0 {
			page = n
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询任务总数失败")
		return
	}

	var tasks []models.Task
	if err := query.
		Order("created_at DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&tasks).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询任务失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":    h.buildTaskResponsesWithProgress(ctx, tasks),
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
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

// buildTaskResponsesWithProgress 构建任务响应并注入实时下载进度
// 仅对 assetID 不为空的任务查进度表，未在下载中则 downloaded/total 为 0
func (h *taskHandler) buildTaskResponsesWithProgress(ctx context.Context, tasks []models.Task) []taskResponse {
	items := h.buildTaskResponses(ctx, tasks)
	if h.progress == nil || len(items) == 0 {
		return items
	}
	progressMap := h.progress.ActiveProgresses()
	if len(progressMap) == 0 {
		return items
	}
	for i := range items {
		if items[i].AssetID == nil {
			continue
		}
		assetID := *items[i].AssetID
		if p, ok := progressMap[assetID]; ok {
			items[i].DownloadedBytes = p.Downloaded
			items[i].TotalBytes = p.Total
		}
	}
	return items
}
