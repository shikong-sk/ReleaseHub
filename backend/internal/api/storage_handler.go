package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type storageHandler struct {
	db *gorm.DB
}

type storageInput struct {
	Name      string `json:"name" binding:"required,min=1,max=120"`
	Type      string `json:"type" binding:"required,oneof=local s3 webdav"`
	BasePath  string `json:"basePath"`
	IsDefault *bool  `json:"isDefault"`
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	RemoteURL string `json:"remoteUrl"`
}

type storageResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	BasePath  string `json:"basePath"`
	IsDefault bool   `json:"isDefault"`
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	AccessKey string `json:"accessKeyHint"`
	Username  string `json:"username"`
	RemoteURL string `json:"remoteUrl"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func registerStorageRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &storageHandler{db: db}

	group := router.Group("/api/storages")
	group.GET("", handler.list)
	group.POST("", handler.create)
	group.GET("/:id", handler.get)
	group.PATCH("/:id", handler.update)
	group.DELETE("/:id", handler.delete)
	group.POST("/:id/test", handler.testConnection)
}

func (h *storageHandler) list(c *gin.Context) {
	var storages []models.Storage
	if err := h.db.WithContext(c.Request.Context()).Order("created_at DESC").Find(&storages).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询存储列表失败")
		return
	}

	items := make([]storageResponse, 0, len(storages))
	for _, s := range storages {
		items = append(items, toStorageResponse(s))
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *storageHandler) create(c *gin.Context) {
	var input storageInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	s := models.Storage{
		Name:      input.Name,
		Type:      input.Type,
		BasePath:  input.BasePath,
		Endpoint:  input.Endpoint,
		Bucket:    input.Bucket,
		Region:    input.Region,
		AccessKey: input.AccessKey,
		SecretKey: input.SecretKey,
		Username:  input.Username,
		Password:  input.Password,
		RemoteURL: input.RemoteURL,
	}
	if input.IsDefault != nil {
		s.IsDefault = *input.IsDefault
	}

	// 填充默认值
	if strings.TrimSpace(s.BasePath) == "" && s.Type == "local" {
		s.BasePath = "data/releases"
	}

	if err := h.db.WithContext(c.Request.Context()).Create(&s).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "创建存储失败")
		return
	}

	c.JSON(http.StatusCreated, toStorageResponse(s))
}

func (h *storageHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var s models.Storage
	if err := h.db.WithContext(c.Request.Context()).First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "存储不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询存储失败")
		return
	}

	c.JSON(http.StatusOK, toStorageResponse(s))
}

func (h *storageHandler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var s models.Storage
	if err := h.db.WithContext(c.Request.Context()).First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "存储不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询存储失败")
		return
	}

	var input storageInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体无效")
		return
	}

	s.Name = input.Name
	s.Type = input.Type
	s.BasePath = input.BasePath
	s.Endpoint = input.Endpoint
	s.Bucket = input.Bucket
	s.Region = input.Region
	s.Username = input.Username
	s.RemoteURL = input.RemoteURL
	if input.IsDefault != nil {
		s.IsDefault = *input.IsDefault
	}
	if input.AccessKey != "" {
		s.AccessKey = input.AccessKey
	}
	if input.SecretKey != "" {
		s.SecretKey = input.SecretKey
	}
	if input.Password != "" {
		s.Password = input.Password
	}

	if err := h.db.WithContext(c.Request.Context()).Save(&s).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "更新存储失败")
		return
	}

	c.JSON(http.StatusOK, toStorageResponse(s))
}

func (h *storageHandler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var s models.Storage
	if err := h.db.WithContext(c.Request.Context()).First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "存储不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询存储失败")
		return
	}

	// 检查是否有仓库在使用
	var count int64
	h.db.WithContext(c.Request.Context()).Model(&models.Repository{}).
		Where("storage_id = ? AND storage_id IS NOT NULL", id).Count(&count)
	// 同时检查多存储关联表
	var rsCount int64
	h.db.WithContext(c.Request.Context()).Model(&models.RepositoryStorage{}).
		Where("storage_id = ?", id).Count(&rsCount)
	count += rsCount
	if count > 0 {
		writeError(c, http.StatusConflict, "该存储正在被仓库使用，无法删除")
		return
	}

	if err := h.db.WithContext(c.Request.Context()).Delete(&s).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "删除存储失败")
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *storageHandler) testConnection(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var s models.Storage
	if err := h.db.WithContext(c.Request.Context()).First(&s, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "存储不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询存储失败")
		return
	}

	driver, err := createStorageDriver(s)
	if err != nil {
		writeError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 尝试测试连接
	if tester, ok := driver.(interface {
		TestConnection(ctx context.Context) error
	}); ok {
		if err := tester.TestConnection(c.Request.Context()); err != nil {
			writeError(c, http.StatusBadGateway, "连接测试失败: "+err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "连接成功"})
		return
	}

	// Local 存储无法测试远程连接，直接返回成功
	c.JSON(http.StatusOK, gin.H{"status": "ok", "message": "本地存储就绪"})
}

// createStorageDriver 根据存储配置创建对应的驱动
func createStorageDriver(s models.Storage) (storage.Driver, error) {
	return storage.NewDriverFromModel(s)
}

func toStorageResponse(s models.Storage) storageResponse {
	accessKeyHint := ""
	if len(s.AccessKey) > 4 {
		accessKeyHint = s.AccessKey[:2] + "****" + s.AccessKey[len(s.AccessKey)-2:]
	} else if s.AccessKey != "" {
		accessKeyHint = "****"
	}

	return storageResponse{
		ID:        s.ID,
		Name:      s.Name,
		Type:      s.Type,
		BasePath:  s.BasePath,
		IsDefault: s.IsDefault,
		Endpoint:  s.Endpoint,
		Bucket:    s.Bucket,
		Region:    s.Region,
		AccessKey: accessKeyHint,
		Username:  s.Username,
		RemoteURL: s.RemoteURL,
		CreatedAt: s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: s.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}
