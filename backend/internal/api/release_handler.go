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
	assetService, assetServiceErr := assetsvc.NewService(db, storageConfig)
	handler := &releaseHandler{
		db:           db,
		assetService: assetService,
	}

	group := router.Group("/api/releases")
	group.GET("/:id", handler.get)
	group.GET("/:id/assets", handler.listAssets)

	assetGroup := router.Group("/api/assets")
	assetGroup.POST("/:id/download", func(c *gin.Context) {
		if assetServiceErr != nil {
			writeError(c, http.StatusInternalServerError, assetServiceErr.Error())
			return
		}
		handler.downloadAsset(c)
	})
	assetGroup.POST("/:id/redownload", func(c *gin.Context) {
		if assetServiceErr != nil {
			writeError(c, http.StatusInternalServerError, assetServiceErr.Error())
			return
		}
		handler.downloadAsset(c)
	})
	assetGroup.DELETE("/:id", func(c *gin.Context) {
		if assetServiceErr != nil {
			writeError(c, http.StatusInternalServerError, assetServiceErr.Error())
			return
		}
		handler.deleteAsset(c)
	})
	assetGroup.GET("/:id/file", func(c *gin.Context) {
		if assetServiceErr != nil {
			writeError(c, http.StatusInternalServerError, assetServiceErr.Error())
			return
		}
		handler.openAsset(c)
	})
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
