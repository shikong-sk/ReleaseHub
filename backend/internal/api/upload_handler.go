package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type uploadHandler struct {
	db *gorm.DB
}

func registerUploadRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &uploadHandler{db: db}
	router.POST("/api/assets/upload", handler.upload)
}

func (h *uploadHandler) upload(c *gin.Context) {
	repoIDStr := c.PostForm("repository_id")
	releaseIDStr := c.PostForm("release_id")
	repoID := parseUploadUint(repoIDStr)
	releaseID := parseUploadUint(releaseIDStr)

	if repoID == 0 || releaseID == 0 {
		writeError(c, http.StatusBadRequest, "需要提供 repository_id 和 release_id")
		return
	}

	var repo models.Repository
	if err := h.db.WithContext(c.Request.Context()).First(&repo, repoID).Error; err != nil {
		writeError(c, http.StatusNotFound, "仓库不存在")
		return
	}

	var release models.Release
	if err := h.db.WithContext(c.Request.Context()).First(&release, releaseID).Error; err != nil {
		writeError(c, http.StatusNotFound, "Release 不存在")
		return
	}

	driver, err := h.getStorageDriver(c, repo)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		writeError(c, http.StatusBadRequest, "文件上传失败: "+err.Error())
		return
	}
	defer file.Close()

	objectPath := filepath.ToSlash(filepath.Join(
		"github",
		safeUploadSegment(repo.Owner),
		safeUploadSegment(repo.Repo),
		safeUploadSegment(release.Tag),
		filepath.Base(header.Filename),
	))
	storedObject, err := driver.Put(c.Request.Context(), objectPath, file)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "存储文件失败: "+err.Error())
		return
	}

	asset := models.Asset{
		ReleaseID:   release.ID,
		Name:        header.Filename,
		Size:        header.Size,
		Status:      models.AssetStatusVerified,
		StoragePath: storedObject.Path,
	}

	if err := h.db.WithContext(c.Request.Context()).Create(&asset).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "保存资产记录失败")
		return
	}

	c.JSON(http.StatusCreated, asset)
}

func (h *uploadHandler) getStorageDriver(c *gin.Context, repo models.Repository) (storage.Driver, error) {
	if repo.StorageID == nil {
		return storage.NewLocalStorage("data/releases")
	}
	var s models.Storage
	if err := h.db.WithContext(c.Request.Context()).First(&s, *repo.StorageID).Error; err != nil {
		return nil, fmt.Errorf("存储配置不存在")
	}
	return createStorageDriver(s)
}

func safeUploadSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	if value == "" || value == "." || value == ".." {
		return "_"
	}
	return value
}

func parseUploadUint(s string) uint {
	var n uint
	fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n
}
