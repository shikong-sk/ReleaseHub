package api

import (
	"net/http"
	"strconv"

	auditlog "releasehub/backend/internal/services/auditlog"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type operationLogHandler struct {
	svc *auditlog.Service
}

func registerOperationLogRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &operationLogHandler{svc: auditlog.NewService(db)}

	group := router.Group("/api/operation-logs")
	group.GET("", handler.list)
}

type operationLogResponse struct {
	ID        uint   `json:"id"`
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
	Detail    string `json:"detail"`
	Status    string `json:"status"`
	ClientIP  string `json:"clientIp"`
	CreatedAt string `json:"createdAt"`
}

func (h *operationLogHandler) list(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "50"))

	logs, total, err := h.svc.List(c.Request.Context(), auditlog.ListParams{
		Actor:    c.Query("actor"),
		Action:   c.Query("action"),
		Status:   c.Query("status"),
		Keyword:  c.Query("keyword"),
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		writeError(c, http.StatusInternalServerError, "查询操作日志失败")
		return
	}

	items := make([]operationLogResponse, 0, len(logs))
	for _, log := range logs {
		items = append(items, operationLogResponse{
			ID:        log.ID,
			Actor:     log.Actor,
			Action:    log.Action,
			Resource:  log.Resource,
			Detail:    log.Detail,
			Status:    log.Status,
			ClientIP:  log.ClientIP,
			CreatedAt: log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}
