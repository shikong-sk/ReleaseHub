package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	StorageID    *uint  `json:"storageId"`
	StorageName  string `json:"storageName"`
	StorageType  string `json:"storageType"`
}

func registerFileRoutes(router *gin.Engine, db *gorm.DB) {
	handler := &fileHandler{db: db}

	group := router.Group("/api/files")
	group.GET("", handler.list)
	group.GET("/tree", handler.tree)
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

	// 预加载存储配置，用于按存储分组展示
	storagesByID := map[uint]models.Storage{}
	{
		var storages []models.Storage
		if err := h.db.WithContext(c.Request.Context()).Find(&storages).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "查询存储配置失败")
			return
		}
		for _, storage := range storages {
			storagesByID[storage.ID] = storage
		}
	}

	items := make([]FileItem, 0, len(assets))
	for _, asset := range assets {
		release := releasesByID[asset.ReleaseID]
		repository := repositoriesByID[release.RepositoryID]

		storageName := "默认本地存储"
		storageType := "local"
		if repository.StorageID != nil {
			if storage, ok := storagesByID[*repository.StorageID]; ok {
				storageName = storage.Name
				storageType = storage.Type
			}
		}

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
			StorageID:    repository.StorageID,
			StorageName:  storageName,
			StorageType:  storageType,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
	})
}

// tree 返回文件树结构。无参数时返回 存储→仓库 两层（仓库带文件计数）；
// 传 repositoryId 时返回该仓库的 版本→文件 子树。
func (h *fileHandler) tree(c *gin.Context) {
	repoQuery := c.Query("repositoryId")
	if repoQuery != "" {
		h.treeForRepository(c, repoQuery)
		return
	}
	h.treeTopLevel(c)
}

type treeNode struct {
	Key      string     `json:"key"`
	Label    string     `json:"label"`
	IsLeaf   bool       `json:"isLeaf"`
	Children []treeNode `json:"children,omitempty"`
	Prefix   string     `json:"prefix,omitempty"`

	// 仓库层附加字段
	RepositoryID uint `json:"repositoryId,omitempty"`
	FileCount    int  `json:"fileCount,omitempty"`

	// 版本层附加字段
	ReleaseID uint `json:"releaseId,omitempty"`

	// 文件叶节点附加字段
	AssetID      uint   `json:"assetId,omitempty"`
	Size         int64  `json:"size,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	StoragePath  string `json:"storagePath,omitempty"`
	DownloadedAt string `json:"downloadedAt,omitempty"`
}

// treeTopLevel 返回 存储→仓库 两层，仓库节点标记为非叶节点并携带文件计数
func (h *fileHandler) treeTopLevel(c *gin.Context) {
	// 查询所有 verified 资产，按 repository_id 分组计数
	type repoCount struct {
		RepositoryID uint
		StorageID    *uint
		Owner        string
		Repo         string
		FileCount    int
	}
	var counts []repoCount
	err := h.db.WithContext(c.Request.Context()).
		Model(&models.Asset{}).
		Select("repositories.id as repository_id, repositories.storage_id, repositories.owner, repositories.repo, COUNT(*) as file_count").
		Joins("JOIN releases ON releases.id = assets.release_id").
		Joins("JOIN repositories ON repositories.id = releases.repository_id").
		Where("assets.status = ? AND assets.storage_path <> ''", models.AssetStatusVerified).
		Group("repositories.id, repositories.storage_id, repositories.owner, repositories.repo").
		Order("repositories.owner, repositories.repo").
		Find(&counts).Error
	if err != nil {
		writeError(c, http.StatusInternalServerError, "查询文件树失败")
		return
	}

	// 预加载存储名称
	storagesByID := map[uint]models.Storage{}
	{
		var storages []models.Storage
		if err := h.db.WithContext(c.Request.Context()).Find(&storages).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "查询存储配置失败")
			return
		}
		for _, s := range storages {
			storagesByID[s.ID] = s
		}
	}

	// 按 storage 分组构建树
	type storageGroup struct {
		ID       uint
		Name     string
		Type     string
		Children []treeNode
	}
	storageMap := map[uint]*storageGroup{}
	var storageOrder []uint

	// 默认本地存储用 0 作为 key
	defaultStorage := &storageGroup{ID: 0, Name: "默认本地存储", Type: "local"}
	storageMap[0] = defaultStorage
	storageOrder = append(storageOrder, 0)

	for _, rc := range counts {
		var groupID uint
		var groupName string
		var groupType string

		if rc.StorageID != nil {
			if s, ok := storagesByID[*rc.StorageID]; ok {
				groupID = s.ID
				groupName = s.Name
				groupType = s.Type
			}
		}

		if groupID == 0 {
			groupID = 0
			groupName = "默认本地存储"
			groupType = "local"
		}

		if _, exists := storageMap[groupID]; !exists {
			storageMap[groupID] = &storageGroup{ID: groupID, Name: groupName, Type: groupType}
			storageOrder = append(storageOrder, groupID)
		}

		repoNode := treeNode{
			Key:          fmt.Sprintf("repo-%d", rc.RepositoryID),
			Label:        fmt.Sprintf("%s/%s", rc.Owner, rc.Repo),
			IsLeaf:       false,
			Prefix:       "📁",
			RepositoryID: rc.RepositoryID,
			FileCount:    rc.FileCount,
		}
		storageMap[groupID].Children = append(storageMap[groupID].Children, repoNode)
	}

	// 组装顶层节点
	var nodes []treeNode
	for _, id := range storageOrder {
		group := storageMap[id]
		if len(group.Children) == 0 {
			continue
		}
		totalFiles := 0
		for _, child := range group.Children {
			totalFiles += child.FileCount
		}
		nodes = append(nodes, treeNode{
			Key:      fmt.Sprintf("storage-%d", id),
			Label:    fmt.Sprintf("%s (%s) — %d 文件", group.Name, strings.ToUpper(group.Type), totalFiles),
			IsLeaf:   false,
			Prefix:   "💾",
			Children: group.Children,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tree": nodes})
}

// treeForRepository 返回指定仓库的 版本→文件 子树
func (h *fileHandler) treeForRepository(c *gin.Context, repoQuery string) {
	repoID, err := strconv.ParseUint(repoQuery, 10, 64)
	if err != nil || repoID == 0 {
		writeError(c, http.StatusBadRequest, "repositoryId 必须是正整数")
		return
	}

	// 查询该仓库下所有 verified 资产
	var assets []models.Asset
	err = h.db.WithContext(c.Request.Context()).
		Where("assets.status = ? AND assets.storage_path <> ''", models.AssetStatusVerified).
		Joins("JOIN releases ON releases.id = assets.release_id").
		Where("releases.repository_id = ?", repoID).
		Order("releases.tag DESC, assets.name").
		Find(&assets).Error
	if err != nil {
		writeError(c, http.StatusInternalServerError, "查询仓库文件失败")
		return
	}

	// 按 release 分组
	type releaseGroup struct {
		ReleaseID uint
		Tag       string
		Assets    []treeNode
	}
	releaseMap := map[uint]*releaseGroup{}
	var releaseOrder []uint

	for _, asset := range assets {
		if _, exists := releaseMap[asset.ReleaseID]; !exists {
			var release models.Release
			if err := h.db.WithContext(c.Request.Context()).First(&release, asset.ReleaseID).Error; err != nil {
				continue
			}
			releaseMap[asset.ReleaseID] = &releaseGroup{
				ReleaseID: release.ID,
				Tag:       release.Tag,
			}
			releaseOrder = append(releaseOrder, asset.ReleaseID)
		}

		releaseMap[asset.ReleaseID].Assets = append(releaseMap[asset.ReleaseID].Assets, treeNode{
			Key:          fmt.Sprintf("asset-%d", asset.ID),
			Label:        asset.Name,
			IsLeaf:       true,
			Prefix:       "📄",
			AssetID:      asset.ID,
			Size:         asset.Size,
			SHA256:       asset.SHA256,
			StoragePath:  asset.StoragePath,
			DownloadedAt: formatTime(asset.DownloadedAt),
		})
	}

	var nodes []treeNode
	for _, id := range releaseOrder {
		rg := releaseMap[id]
		nodes = append(nodes, treeNode{
			Key:       fmt.Sprintf("release-%d", rg.ReleaseID),
			Label:     rg.Tag,
			IsLeaf:    false,
			Prefix:    "🏷️",
			ReleaseID: rg.ReleaseID,
			Children:  rg.Assets,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tree": nodes})
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
