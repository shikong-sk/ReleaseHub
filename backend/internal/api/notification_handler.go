package api

import (
	"errors"
	"net/http"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type notificationHandler struct {
	db *gorm.DB
}

type notificationInput struct {
	Name      string `json:"name" binding:"required,min=1,max=120"`
	Type      string `json:"type" binding:"required,oneof=gotify webhook telegram email"`
	ServerURL string `json:"serverUrl"`
	Token     string `json:"token"`
	Enabled   *bool  `json:"enabled"`
	Events    string `json:"events"`
}

type notificationResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	ServerURL string `json:"serverUrl"`
	TokenHint string `json:"tokenHint"`
	Enabled   bool   `json:"enabled"`
	Events    string `json:"events"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func registerNotificationRoutes(router *gin.Engine, db *gorm.DB) {
	handler := notificationHandler{db: db}
	group := router.Group("/api/notifications")
	group.GET("", handler.list)
	group.POST("", handler.create)
	group.GET("/:id", handler.get)
	group.PATCH("/:id", handler.update)
	group.DELETE("/:id", handler.delete)
	group.POST("/:id/test", handler.testSend)
}

func (h notificationHandler) list(c *gin.Context) {
	var notifications []models.Notification
	if err := h.db.WithContext(c.Request.Context()).Order("created_at DESC").Find(&notifications).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询通知列表失败")
		return
	}
	items := make([]notificationResponse, 0, len(notifications))
	for _, n := range notifications {
		items = append(items, toNotificationResponse(n))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h notificationHandler) create(c *gin.Context) {
	var input notificationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}
	notification := models.Notification{
		Name:      input.Name,
		Type:      input.Type,
		ServerURL: input.ServerURL,
		Token:     input.Token,
		Enabled:   true,
		Events:    input.Events,
	}
	if input.Enabled != nil {
		notification.Enabled = *input.Enabled
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&notification).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "创建通知失败")
		return
	}
	c.JSON(http.StatusCreated, toNotificationResponse(notification))
}

func (h notificationHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var notification models.Notification
	if err := h.db.WithContext(c.Request.Context()).First(&notification, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "通知不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询通知失败")
		return
	}
	c.JSON(http.StatusOK, toNotificationResponse(notification))
}

func (h notificationHandler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var notification models.Notification
	if err := h.db.WithContext(c.Request.Context()).First(&notification, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "通知不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询通知失败")
		return
	}
	var input notificationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}
	notification.Name = input.Name
	notification.Type = input.Type
	notification.ServerURL = input.ServerURL
	notification.Events = input.Events
	if input.Token != "" {
		notification.Token = input.Token
	}
	if input.Enabled != nil {
		notification.Enabled = *input.Enabled
	}
	if err := h.db.WithContext(c.Request.Context()).Save(&notification).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "更新通知失败")
		return
	}
	c.JSON(http.StatusOK, toNotificationResponse(notification))
}

func (h notificationHandler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var notification models.Notification
	if err := h.db.WithContext(c.Request.Context()).First(&notification, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "通知不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询通知失败")
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Delete(&notification).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "删除通知失败")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h notificationHandler) testSend(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var notification models.Notification
	if err := h.db.WithContext(c.Request.Context()).First(&notification, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "通知不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询通知失败")
		return
	}
	notifier, err := CreateNotifier(notification)
	if err != nil {
		writeError(c, http.StatusBadRequest, "通知配置无效: "+err.Error())
		return
	}
	if err := notifier.Send(c.Request.Context(), "ReleaseHub 测试通知", "如果你收到了这条消息，说明通知配置正确。"); err != nil {
		writeError(c, http.StatusBadGateway, "发送测试通知失败: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "测试通知发送成功"})
}

func toNotificationResponse(n models.Notification) notificationResponse {
	hint := ""
	if len(n.Token) > 8 {
		hint = n.Token[:4] + "****" + n.Token[len(n.Token)-4:]
	} else if n.Token != "" {
		hint = "****"
	}
	return notificationResponse{
		ID:        n.ID,
		Name:      n.Name,
		Type:      n.Type,
		ServerURL: n.ServerURL,
		TokenHint: hint,
		Enabled:   n.Enabled,
		Events:    n.Events,
		CreatedAt: n.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: n.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
