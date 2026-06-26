package api

import (
	"net/http"
	"strconv"
	"time"

	"releasehub/backend/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type fileHandler struct {
	db *gorm.DB
}

type FileItem struct {
	AssetID      uint   `json:"assetId"`
	ReleaseID    uint   `json:"releaseId"`
	RepositoryID uint   `json:"repositoryId"`
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	Tag          string `json:"tag"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	StoragePath  string `json:"storagePath"`
	DownloadedAt string `json:"downloadedAt"`
}

func registerFileRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &fileHandler{db: db}

	group := router.Group("/api/files")
	group.GET("", handler.list)
	group.GET("/download", handler.download)
}

func (h *fileHandler) list(c *gin.Context) {
	var assets []models.Asset
	err := h.db.WithContext(c.Request.Context()).
		Where("assets.status = ? AND assets.storage_path <> ''", models.AssetStatusVerified).
		Order("assets.downloaded_at DESC, assets.updated_at DESC").
		Limit(500).
		Find(&assets).Error
	if err != nil {
		writeError(c, http.StatusInternalServerError, "查询文件失败")
		return
	}

	releaseIDs := make([]uint, 0, len(assets))
	for _, asset := range assets {
		releaseIDs = append(releaseIDs, asset.ReleaseID)
	}

	releasesByID := map[uint]models.Release{}
	repositoriesByID := map[uint]models.Repository{}
	if len(releaseIDs) > 0 {
		var releases []models.Release
		if err := h.db.WithContext(c.Request.Context()).Where("id IN ?", releaseIDs).Find(&releases).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "查询文件 Release 失败")
			return
		}

		repositoryIDs := make([]uint, 0, len(releases))
		for _, release := range releases {
			releasesByID[release.ID] = release
			repositoryIDs = append(repositoryIDs, release.RepositoryID)
		}

		var repositories []models.Repository
		if err := h.db.WithContext(c.Request.Context()).Where("id IN ?", repositoryIDs).Find(&repositories).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "查询文件仓库失败")
			return
		}
		for _, repository := range repositories {
			repositoriesByID[repository.ID] = repository
		}
	}

	items := make([]FileItem, 0, len(assets))
	for _, asset := range assets {
		release := releasesByID[asset.ReleaseID]
		repository := repositoriesByID[release.RepositoryID]

		items = append(items, FileItem{
			AssetID:      asset.ID,
			ReleaseID:    release.ID,
			RepositoryID: repository.ID,
			Owner:        repository.Owner,
			Repo:         repository.Repo,
			Tag:          release.Tag,
			Name:         asset.Name,
			Size:         asset.Size,
			SHA256:       asset.SHA256,
			StoragePath:  asset.StoragePath,
			DownloadedAt: formatTime(asset.DownloadedAt),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
	})
}

func (h *fileHandler) download(c *gin.Context) {
	rawAssetID := c.Query("assetId")
	assetID, err := strconv.ParseUint(rawAssetID, 10, 64)
	if err != nil || assetID == 0 {
		writeError(c, http.StatusBadRequest, "assetId 必须是正整数")
		return
	}

	c.Redirect(http.StatusFound, "/api/assets/"+strconv.FormatUint(assetID, 10)+"/file")
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
