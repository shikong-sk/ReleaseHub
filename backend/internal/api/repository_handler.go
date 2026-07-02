package api

import (
	"errors"
	"net/http"
	"fmt"
	"strconv"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"
	githubsvc "releasehub/backend/internal/services/github"
	releasesvc "releasehub/backend/internal/services/release"
	providersvc "releasehub/backend/internal/services/provider"
	repositorysvc "releasehub/backend/internal/services/repository"
	retentionsvc "releasehub/backend/internal/services/retention"
	syncersvc "releasehub/backend/internal/services/syncer"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type repositoryHandler struct {
	service         *repositorysvc.Service
	checkService    *releasesvc.CheckService
	syncService     *syncersvc.Service
	syncServiceErr  error
	githubClientErr error
	retentionSvc    *retentionsvc.Service
}

func registerRepositoryRoutes(router *gin.Engine, db *gorm.DB, storageConfig config.StorageConfig, githubAPIBaseURL string, githubClient *githubsvc.Client, githubClientErr error, sharedSyncer *syncersvc.Service) {
	providerRegistry := providersvc.NewRegistry(githubAPIBaseURL)
	checkService := releasesvc.NewCheckService(db, githubClient).
		WithGitHubFactory(githubsvc.NewClientFactory(githubAPIBaseURL, db)).
		WithProviderRegistry(providerRegistry)
	var retentionService *retentionsvc.Service
	if rs, err := retentionsvc.NewService(db, storageConfig); err == nil {
		retentionService = rs
		checkService.WithRetention(rs)
	}
	// 复用全局共享的 syncer 实例（与 scheduler / config_handler 同一实例），
	// 确保运行时并发配置调整对手动同步入口同样生效
	syncService := sharedSyncer
	syncServiceErr := error(nil)
	if syncService == nil {
		// scheduler 未启用时降级：独立创建实例，使用默认并发数
		if s, err := syncersvc.NewService(db, checkService, storageConfig); err == nil {
			syncService = s
		} else {
			syncServiceErr = err
		}
	}

	handler := &repositoryHandler{
		service:         repositorysvc.NewService(db, storageConfig),
		checkService:    checkService,
		syncService:     syncService,
		syncServiceErr:  syncServiceErr,
		githubClientErr: githubClientErr,
		retentionSvc:    retentionService,
	}

	group := router.Group("/api/repositories")
	group.GET("", handler.list)
	group.POST("", handler.create)
	group.GET("/:id", handler.get)
	group.PATCH("/:id", handler.update)
	group.DELETE("/:id", handler.delete)
	group.POST("/:id/check", handler.checkLatest)
	group.POST("/:id/check-all", handler.checkAll)
	group.POST("/:id/sync", handler.syncLatest)
	group.POST("/:id/sync-tag", handler.syncByTag)
	group.GET("/:id/releases", handler.listReleases)
	group.GET("/:id/remote-tags", handler.remoteTags)
	group.GET("/:id/retention-preview", handler.retentionPreview)
	group.POST("/:id/cleanup", handler.cleanup)
}

func (h *repositoryHandler) list(c *gin.Context) {
	repositories, err := h.service.List(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "查询仓库失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": repositories,
	})
}

func (h *repositoryHandler) create(c *gin.Context) {
	var input repositorysvc.CreateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体不是有效 JSON")
		return
	}

	repository, err := h.service.Create(c.Request.Context(), input)
	if err != nil {
		writeServiceError(c, err, "创建仓库失败")
		return
	}

	c.JSON(http.StatusCreated, repository)
}

func (h *repositoryHandler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	repository, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err, "查询仓库失败")
		return
	}

	c.JSON(http.StatusOK, repository)
}

func (h *repositoryHandler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var input repositorysvc.UpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "请求体不是有效 JSON")
		return
	}

	repository, err := h.service.Update(c.Request.Context(), id, input)
	if err != nil {
		writeServiceError(c, err, "更新仓库失败")
		return
	}

	c.JSON(http.StatusOK, repository)
}

func (h *repositoryHandler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		writeServiceError(c, err, "删除仓库失败")
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *repositoryHandler) retentionPreview(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if h.retentionSvc == nil {
		writeError(c, http.StatusServiceUnavailable, "保留策略服务未初始化")
		return
	}

	repository, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err, "查询仓库失败")
		return
	}

	result, err := h.retentionSvc.Preview(c.Request.Context(), *repository)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "预览保留策略失败")
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *repositoryHandler) cleanup(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	if h.retentionSvc == nil {
		writeError(c, http.StatusServiceUnavailable, "保留策略服务未初始化")
		return
	}

	repository, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		writeServiceError(c, err, "查询仓库失败")
		return
	}

	result, err := h.retentionSvc.Cleanup(c.Request.Context(), *repository)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "清理失败")
		return
	}

	c.JSON(http.StatusOK, result)
}
func (h *repositoryHandler) checkLatest(c *gin.Context) {
	if h.githubClientErr != nil {
		writeError(c, http.StatusInternalServerError, h.githubClientErr.Error())
		return
	}

	id, ok := parseID(c)
	if !ok {
		return
	}

	result, err := h.checkService.CheckLatest(c.Request.Context(), id)
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *repositoryHandler) checkAll(c *gin.Context) {
	if h.githubClientErr != nil {
		writeError(c, http.StatusInternalServerError, h.githubClientErr.Error())
		return
	}

	id, ok := parseID(c)
	if !ok {
		return
	}

	result, err := h.checkService.CheckAll(c.Request.Context(), id)
	if err != nil {
		writeError(c, http.StatusBadGateway, err.Error())
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *repositoryHandler) syncLatest(c *gin.Context) {
	if h.githubClientErr != nil {
		writeError(c, http.StatusInternalServerError, h.githubClientErr.Error())
		return
	}
	if h.syncServiceErr != nil {
		writeError(c, http.StatusInternalServerError, h.syncServiceErr.Error())
		return
	}

	id, ok := parseID(c)
	if !ok {
		return
	}

	result, err := h.syncService.EnqueueSyncRepository(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  err.Error(),
			"result": result,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}


// syncByTag 同步指定 tag 的 Release
func (h *repositoryHandler) syncByTag(c *gin.Context) {
	if h.githubClientErr != nil {
		writeError(c, http.StatusInternalServerError, h.githubClientErr.Error())
		return
	}
	if h.syncServiceErr != nil {
		writeError(c, http.StatusInternalServerError, h.syncServiceErr.Error())
		return
	}

	id, ok := parseID(c)
	if !ok {
		return
	}

	var input struct {
		Tag string `json:"tag" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "tag 参数必填")
		return
	}

	result, err := h.syncService.EnqueueSyncByTag(c.Request.Context(), id, input.Tag)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":  err.Error(),
			"result": result,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}
func (h *repositoryHandler) listReleases(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}

	var releases []models.Release
	if err := h.service.DB().WithContext(c.Request.Context()).
		Where("repository_id = ?", id).
		Order("published_at DESC, created_at DESC").
		Find(&releases).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询 Release 失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": releases,
	})
}

func parseID(c *gin.Context) (uint, bool) {
	rawID := c.Param("id")
	parsedID, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || parsedID == 0 {
		writeError(c, http.StatusBadRequest, "id 必须是正整数")
		return 0, false
	}

	return uint(parsedID), true
}

func writeServiceError(c *gin.Context, err error, fallback string) {
	switch {
	case repositorysvc.IsNotFound(err):
		writeError(c, http.StatusNotFound, err.Error())
	case repositorysvc.IsInvalidInput(err):
		writeError(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, gorm.ErrDuplicatedKey):
		writeError(c, http.StatusConflict, "仓库已存在")
	default:
		writeError(c, http.StatusInternalServerError, fallback)
	}
}

func writeError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": message,
	})
}

// remoteTags 返回远程仓库的 Release tag 列表（不持久化，仅查询）
func (h *repositoryHandler) remoteTags(c *gin.Context) {
	if h.githubClientErr != nil {
		writeError(c, http.StatusInternalServerError, h.githubClientErr.Error())
		return
	}

	id, ok := parseID(c)
	if !ok {
		return
	}

	var repository models.Repository
	if err := h.service.DB().WithContext(c.Request.Context()).First(&repository, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(c, http.StatusNotFound, "仓库不存在")
			return
		}
		writeError(c, http.StatusInternalServerError, "查询仓库失败")
		return
	}

	token, err := h.checkService.TokenForRepository(c.Request.Context(), &repository)
	if err != nil {
		writeError(c, http.StatusBadGateway, "获取 Token 失败")
		return
	}

	releaseProvider, err := h.checkService.ProviderForRepository(c.Request.Context(), &repository)
	if err != nil {
		writeError(c, http.StatusBadGateway, "创建 Provider 失败")
		return
	}

	providerReleases, err := releaseProvider.ListAllReleases(c.Request.Context(), repository.Owner, repository.Repo, token, 5)
	if err != nil {
		writeError(c, http.StatusBadGateway, fmt.Sprintf("查询远程 Release 失败: %s", err.Error()))
		return
	}

	tags := make([]string, 0, len(providerReleases))
	for _, r := range providerReleases {
		tags = append(tags, r.TagName)
	}

	c.JSON(http.StatusOK, gin.H{"tags": tags})
}
