package api

import (
	"net/http"
	"strconv"
	"time"

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
	repositoryID, _ := strconv.Atoi(c.Query("repositoryId"))
	status := c.Query("status")
	dateFrom := c.Query("dateFrom")
	dateTo := c.Query("dateTo")

	limit := 20
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}

	result := searchResult{}
	ctx := c.Request.Context()

	// 搜索仓库（仅文本查询）
	if query != "" {
		h.db.WithContext(ctx).
			Where("owner LIKE ? OR repo LIKE ?", "%"+query+"%", "%"+query+"%").
			Order("updated_at DESC").
			Limit(limit).
			Find(&result.Repositories)
	}

	// 搜索 Release：文本 + body + 组合筛选
	releaseQ := h.db.WithContext(ctx).Model(&models.Release{})
	if query != "" {
		releaseQ = releaseQ.Where("tag LIKE ? OR name LIKE ? OR body LIKE ?", "%"+query+"%", "%"+query+"%", "%"+query+"%")
	}
	if repositoryID > 0 {
		releaseQ = releaseQ.Where("repository_id = ?", repositoryID)
	}
	if dateFrom != "" {
		if t, err := time.Parse("2006-01-02", dateFrom); err == nil {
			releaseQ = releaseQ.Where("published_at >= ?", t)
		}
	}
	if dateTo != "" {
		if t, err := time.Parse("2006-01-02", dateTo); err == nil {
			releaseQ = releaseQ.Where("published_at < ?", t.AddDate(0, 0, 1))
		}
	}
	releaseQ.Order("published_at DESC").Limit(limit).Find(&result.Releases)

	// 搜索 Asset：文本 + 状态 + 仓库筛选
	assetQ := h.db.WithContext(ctx).Model(&models.Asset{})
	if query != "" {
		assetQ = assetQ.Where("name LIKE ?", "%"+query+"%")
	}
	if status != "" {
		assetQ = assetQ.Where("status = ?", status)
	}
	if repositoryID > 0 {
		assetQ = assetQ.Where("release_id IN (?)",
			h.db.Model(&models.Release{}).Select("id").Where("repository_id = ?", repositoryID))
	}
	assetQ.Order("created_at DESC").Limit(limit).Find(&result.Assets)

	result.Total = int64(len(result.Repositories) + len(result.Releases) + len(result.Assets))

	c.JSON(http.StatusOK, result)
}
