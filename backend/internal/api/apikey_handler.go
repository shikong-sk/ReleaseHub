package api

import (
	"errors"
	"net/http"

	"releasehub/backend/internal/middleware"
	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type apiKeyHandler struct {
	db *gorm.DB
}

type createAPIKeyInput struct {
	Name  string `json:"name" binding:"required,min=1,max=120"`
	Scope string `json:"scope"`
}

type apiKeyResponse struct {
	ID         uint    `json:"id"`
	Name       string  `json:"name"`
	KeyHint    string  `json:"keyHint"`
	Scope      string  `json:"scope"`
	Enabled    bool    `json:"enabled"`
	LastUsedAt *string `json:"lastUsedAt"`
	CreatedAt  string  `json:"createdAt"`
}

type createAPIKeyResponse struct {
	apiKeyResponse
	Key string `json:"key"`
}

func registerAPIKeyRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &apiKeyHandler{db: db}
	group := router.Group("/api/apikeys")
	group.GET("", handler.list)
	group.POST("", handler.create)
	group.DELETE("/:id", handler.delete)
}

func (h *apiKeyHandler) list(c *gin.Context) {
	var keys []models.APIKey
	if err := h.db.WithContext(c.Request.Context()).Order("created_at DESC").Find(&keys).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询 API Key 列表失败")
		return
	}
	items := make([]apiKeyResponse, 0, len(keys))
	for _, k := range keys {
		items = append(items, toAPIKeyResponse(k))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *apiKeyHandler) create(c *gin.Context) {
	var input createAPIKeyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	key, hint, err := middleware.GenerateAPIKey()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "生成 Key 失败")
		return
	}

	scope := input.Scope
	if scope == "" {
		scope = "*"
	}

	apiKey := models.APIKey{
		Name:    input.Name,
		Key:     key,
		KeyHint: hint,
		Scope:   scope,
		Enabled: true,
	}

	if err := h.db.WithContext(c.Request.Context()).Create(&apiKey).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "创建 API Key 失败")
		return
	}

	resp := createAPIKeyResponse{
		apiKeyResponse: toAPIKeyResponse(apiKey),
		Key:            key,
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *apiKeyHandler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var apiKey models.APIKey
	if err := h.db.WithContext(c.Request.Context()).First(&apiKey, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "API Key 不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询 API Key 失败")
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Delete(&apiKey).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "删除 API Key 失败")
		return
	}

	c.Status(http.StatusNoContent)
}

func toAPIKeyResponse(k models.APIKey) apiKeyResponse {
	var lastUsed *string
	if k.LastUsedAt != nil {
		s := k.LastUsedAt.UTC().Format("2006-01-02T15:04:05Z")
		lastUsed = &s
	}
	return apiKeyResponse{
		ID:         k.ID,
		Name:       k.Name,
		KeyHint:    k.KeyHint,
		Scope:      k.Scope,
		Enabled:    k.Enabled,
		LastUsedAt: lastUsed,
		CreatedAt:  k.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
