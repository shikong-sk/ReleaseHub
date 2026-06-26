package api

import (
	"errors"
	"net/http"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tokenHandler struct {
	db *gorm.DB
}

type tokenInput struct {
	Name  string `json:"name" binding:"required,min=1,max=120"`
	Token string `json:"token" binding:"required,min=1,max=512"`
}

type tokenResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	TokenHint string `json:"tokenHint"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func registerTokenRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &tokenHandler{db: db}

	group := router.Group("/api/tokens")
	group.GET("", handler.list)
	group.POST("", handler.create)
	group.GET("/:id", handler.get)
	group.DELETE("/:id", handler.delete)
}

func (h *tokenHandler) list(c *gin.Context) {
	var tokens []models.GitHubToken
	if err := h.db.WithContext(c.Request.Context()).Order("created_at DESC").Find(&tokens).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询 Token 列表失败")
		return
	}

	items := make([]tokenResponse, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, toTokenResponse(token))
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *tokenHandler) create(c *gin.Context) {
	var input tokenInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	token := models.GitHubToken{
		Name:  input.Name,
		Token: input.Token,
	}
	if err := h.db.WithContext(c.Request.Context()).Create(&token).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "创建 Token 失败")
		return
	}

	c.JSON(http.StatusCreated, toTokenResponse(token))
}

func (h *tokenHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var token models.GitHubToken
	if err := h.db.WithContext(c.Request.Context()).First(&token, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "Token 不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询 Token 失败")
		return
	}

	c.JSON(http.StatusOK, toTokenResponse(token))
}

func (h *tokenHandler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var token models.GitHubToken
	if err := h.db.WithContext(c.Request.Context()).First(&token, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "Token 不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询 Token 失败")
		return
	}

	// 检查是否有仓库正在使用此 Token
	var count int64
	h.db.WithContext(c.Request.Context()).Model(&models.Repository{}).
		Where("github_token_id = ?", id).Count(&count)
	if count > 0 {
		writeError(c, http.StatusConflict, "该 Token 正在被 %d 个仓库使用，无法删除")
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Delete(&token).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "删除 Token 失败")
		return
	}

	c.Status(http.StatusNoContent)
}

func toTokenResponse(token models.GitHubToken) tokenResponse {
	hint := ""
	if len(token.Token) > 8 {
		hint = token.Token[:4] + "****" + token.Token[len(token.Token)-4:]
	} else {
		hint = "****"
	}

	return tokenResponse{
		ID:        token.ID,
		Name:      token.Name,
		TokenHint: hint,
		CreatedAt: token.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: token.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
