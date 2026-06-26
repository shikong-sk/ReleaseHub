package api

import (
	"net/http"
	"strconv"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type searchHandler struct {
	db *gorm.DB
}

type searchResult struct {
	Repositories []models.Repository `json:"repositories"`
	Releases     []models.Release    `json:"releases"`
	Assets       []models.Asset      `json:"assets"`
	Total        int64               `json:"total"`
}

func registerSearchRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &searchHandler{db: db}
	router.GET("/api/search", handler.search)
}

func (h *searchHandler) search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, searchResult{})
		return
	}

	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	result := searchResult{}

	// 搜索仓库
	h.db.WithContext(c.Request.Context()).
		Where("owner LIKE ? OR repo LIKE ?", "%"+query+"%", "%"+query+"%").
		Order("updated_at DESC").
		Limit(limit).
		Find(&result.Repositories)

	// 搜索 Release
	h.db.WithContext(c.Request.Context()).
		Where("tag LIKE ? OR name LIKE ?", "%"+query+"%", "%"+query+"%").
		Order("published_at DESC").
		Limit(limit).
		Find(&result.Releases)

	// 搜索 Asset
	h.db.WithContext(c.Request.Context()).
		Where("name LIKE ?", "%"+query+"%").
		Order("created_at DESC").
		Limit(limit).
		Find(&result.Assets)

	result.Total = int64(len(result.Repositories) + len(result.Releases) + len(result.Assets))

	c.JSON(http.StatusOK, result)
}
