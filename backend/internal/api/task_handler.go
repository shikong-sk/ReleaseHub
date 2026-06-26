package api

import (
	"errors"
	"net/http"
	"strconv"

	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/tasklog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type taskHandler struct {
	db        *gorm.DB
	logService *tasklog.Service
}

func registerTaskRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &taskHandler{
		db:        db,
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
		"items": tasks,
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

	c.JSON(http.StatusOK, task)
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
