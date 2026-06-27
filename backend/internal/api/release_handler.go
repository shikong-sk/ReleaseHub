package api

import (
	"net/http"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"
	assetsvc "releasehub/backend/internal/services/asset"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type releaseHandler struct {
	db           *gorm.DB
	assetService *assetsvc.Service
}

func registerReleaseRoutes(router *gin.Engine, db *gorm.DB, storageConfig config.StorageConfig) {
	assetService := assetsvc.NewServiceWithFactory(db, storageConfig)
	handler := &releaseHandler{
		db:           db,
		assetService: assetService,
	}

	group := router.Group("/api/releases")
	group.GET("/:id", handler.get)
	group.GET("/:id/assets", handler.listAssets)
	group.POST("/:id/pin", handler.pinRelease)
	group.POST("/:id/unpin", handler.unpinRelease)

	assetGroup := router.Group("/api/assets")
	assetGroup.POST("/:id/download", handler.downloadAsset)
	assetGroup.POST("/:id/redownload", handler.downloadAsset)
	assetGroup.DELETE("/:id", handler.deleteAsset)
	assetGroup.GET("/:id/file", handler.openAsset)
}

func (h *releaseHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var release models.Release
	if err := h.db.WithContext(c.Request.Context()).First(&release, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(c, http.StatusNotFound, "Release 不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询 Release 失败")
		return
	}

	c.JSON(http.StatusOK, release)
}


// pinRelease 固定指定版本，使其不受保留策略清理
func (h *releaseHandler) pinRelease(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var release models.Release
	if err := h.db.WithContext(c.Request.Context()).First(&release, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(c, http.StatusNotFound, "Release 不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询 Release 失败")
		return
	}

	release.IsPinned = true
	if err := h.db.WithContext(c.Request.Context()).Save(&release).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "固定版本失败")
		return
	}

	c.JSON(http.StatusOK, release)
}

// unpinRelease 取消固定指定版本
func (h *releaseHandler) unpinRelease(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var release models.Release
	if err := h.db.WithContext(c.Request.Context()).First(&release, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeError(c, http.StatusNotFound, "Release 不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询 Release 失败")
		return
	}

	release.IsPinned = false
	if err := h.db.WithContext(c.Request.Context()).Save(&release).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "取消固定版本失败")
		return
	}

	c.JSON(http.StatusOK, release)
}

func (h *releaseHandler) listAssets(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var assets []models.Asset
	if err := h.db.WithContext(c.Request.Context()).
		Where("release_id = ?", id).
		Order("name ASC").
		Find(&assets).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询资产失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": assets,
	})
}

func (h *releaseHandler) downloadAsset(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	result, err := h.assetService.Download(c.Request.Context(), id)
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *releaseHandler) deleteAsset(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.assetService.Delete(c.Request.Context(), id); err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *releaseHandler) openAsset(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	asset, object, file, err := h.assetService.Open(c.Request.Context(), id)
	if err != nil {
		writeError(c, http.StatusNotFound, err.Error())
		return
	}
	defer file.Close()

	c.Header("Content-Disposition", `attachment; filename="`+object.Filename+`"`)
	c.DataFromReader(http.StatusOK, object.Size, asset.ContentType, file, nil)
}
