package api

import (
	"net/http"
	"strings"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/storage"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type reconcileHandler struct {
	db       *gorm.DB
	logger   *zap.Logger
	storages *storage.DriverFactory
}

type reconcileResult struct {
	MissingInStorage []string `json:"missingInStorage"`
	MissingInDB      []string `json:"missingInDB"`
	OrphanedAssets   []uint   `json:"orphanedAssets"`
}

func registerReconcileRoutes(router *gin.Engine, db *gorm.DB, storageConfig config.StorageConfig, logger *zap.Logger) {
	handler := &reconcileHandler{
		db:       db,
		logger:   logger,
		storages: storage.NewDriverFactory(db, storageConfig),
	}
	router.POST("/api/reconcile", handler.reconcile)
}

func (h *reconcileHandler) reconcile(c *gin.Context) {
	result := reconcileResult{
		MissingInStorage: []string{},
		MissingInDB:      []string{},
		OrphanedAssets:   []uint{},
	}

	var assets []models.Asset
	if err := h.db.WithContext(c.Request.Context()).
		Where("status = ? AND storage_path != ?", models.AssetStatusVerified, "").
		Find(&assets).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询资产失败")
		return
	}

	for _, asset := range assets {
		if strings.TrimSpace(asset.StoragePath) == "" {
			continue
		}

		repo, err := h.getRepo(c, asset)
		if err != nil {
			continue
		}

		driver, err := h.getStorageDriver(c, *repo)
		if err != nil {
			continue
		}

		reader, _, err := driver.Open(c.Request.Context(), asset.StoragePath)
		if err != nil {
			result.MissingInStorage = append(result.MissingInStorage, asset.StoragePath)
			result.OrphanedAssets = append(result.OrphanedAssets, asset.ID)
		} else {
			reader.Close()
		}
	}

	c.JSON(http.StatusOK, result)
}

func (h *reconcileHandler) getRepo(c *gin.Context, asset models.Asset) (*models.Repository, error) {
	var release models.Release
	if err := h.db.WithContext(c.Request.Context()).First(&release, asset.ReleaseID).Error; err != nil {
		return nil, err
	}
	var repo models.Repository
	if err := h.db.WithContext(c.Request.Context()).First(&repo, release.RepositoryID).Error; err != nil {
		return nil, err
	}
	return &repo, nil
}

func (h *reconcileHandler) getStorageDriver(c *gin.Context, repo models.Repository) (storage.Driver, error) {
	return h.storages.DriverForRepository(c.Request.Context(), repo)
}
