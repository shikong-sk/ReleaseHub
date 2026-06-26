package api

import (
	"net/http"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type taskHandler struct {
	db *gorm.DB
}

func registerTaskRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &taskHandler{db: db}

	group := router.Group("/api/tasks")
	group.GET("", handler.list)
	group.GET("/:id", handler.get)
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
		if err == gorm.ErrRecordNotFound {
			writeError(c, http.StatusNotFound, "任务不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询任务失败")
		return
	}

	c.JSON(http.StatusOK, task)
}
