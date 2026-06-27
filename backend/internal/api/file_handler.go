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
	Status       string `json:"status,omitempty"`
	AssetID      uint   `json:"assetId,omitempty"`
	Size         int64  `json:"size,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	StoragePath  string `json:"storagePath,omitempty"`
	DownloadedAt string `json:"downloadedAt,omitempty"`
}

// treeTopLevel 返回 存储→仓库 两层，仓库节点标记为非叶节点并携带文件计数
func (h *fileHandler) treeTopLevel(c *gin.Context) {
	// 查询所有 enabled 的仓库，无论是否有 verified 资产
	var repos []models.Repository
	if err := h.db.WithContext(c.Request.Context()).
		Where("enabled = ?", true).
		Order("owner, repo").
		Find(&repos).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询仓库列表失败")
		return
	}

	// 查询 verified 资产计数，按 repository 分组
	type repoCount struct {
		RepositoryID uint
		FileCount    int
	}
	var counts []repoCount
	_ = h.db.WithContext(c.Request.Context()).
		Model(&models.Asset{}).
		Select("repositories.id as repository_id, COUNT(*) as file_count").
		Joins("JOIN releases ON releases.id = assets.release_id").
		Joins("JOIN repositories ON repositories.id = releases.repository_id").
		Where("assets.status = ? AND assets.storage_path <> ''", models.AssetStatusVerified).
		Group("repositories.id").
		Find(&counts).Error

	// 构建 repository_id → file_count 映射
	countMap := make(map[uint]int, len(counts))
	for _, rc := range counts {
		countMap[rc.RepositoryID] = rc.FileCount
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

	for _, repo := range repos {
		var groupID uint
		var groupName string
		var groupType string

		if repo.StorageID != nil {
			if s, ok := storagesByID[*repo.StorageID]; ok {
				groupID = s.ID
				groupName = s.Name
				groupType = s.Type
			}
		}

		if groupID == 0 {
			groupName = "默认本地存储"
			groupType = "local"
		}

		if _, exists := storageMap[groupID]; !exists {
			storageMap[groupID] = &storageGroup{ID: groupID, Name: groupName, Type: groupType}
			storageOrder = append(storageOrder, groupID)
		}

		fc := countMap[repo.ID]
		repoNode := treeNode{
			Key:          fmt.Sprintf("repo-%d", repo.ID),
			Label:        fmt.Sprintf("%s/%s", repo.Owner, repo.Repo),
			IsLeaf:       false,
			Prefix:       "📁",
			RepositoryID: repo.ID,
			FileCount:    fc,
		}
		storageMap[groupID].Children = append(storageMap[groupID].Children, repoNode)
	}

	// 组装顶层节点
	nodes := make([]treeNode, 0)
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
// 显示所有 Release（包括正在同步的），资产状态为 pending/downloading/failed 的也展示
func (h *fileHandler) treeForRepository(c *gin.Context, repoQuery string) {
	repoID, err := strconv.ParseUint(repoQuery, 10, 64)
	if err != nil || repoID == 0 {
		writeError(c, http.StatusBadRequest, "repositoryId 必须是正整数")
		return
	}

	ctx := c.Request.Context()

	// 查询该仓库下所有 Release（按 tag 倒序）
	var releases []models.Release
	if err := h.db.WithContext(ctx).
		Where("repository_id = ? AND deleted_at IS NULL", repoID).
		Order("tag DESC").
		Find(&releases).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询 Release 列表失败")
		return
	}

	if len(releases) == 0 {
		c.JSON(http.StatusOK, gin.H{"tree": make([]treeNode, 0)})
		return
	}

	releaseIDs := make([]uint, 0, len(releases))
	for _, r := range releases {
		releaseIDs = append(releaseIDs, r.ID)
	}

	// 查询这些 Release 下的所有资产（不限状态，排除 deleted 和 skipped）
	var assets []models.Asset
	if err := h.db.WithContext(ctx).
		Where("release_id IN ? AND status NOT IN ?", releaseIDs,
			[]models.AssetStatus{models.AssetStatusDeleted, models.AssetStatusSkipped}).
		Order("release_id, name").
		Find(&assets).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询仓库文件失败")
		return
	}

	// 按 release_id 分组
	assetsByRelease := make(map[uint][]models.Asset, len(releases))
	for _, a := range assets {
		assetsByRelease[a.ReleaseID] = append(assetsByRelease[a.ReleaseID], a)
	}

	nodes := make([]treeNode, 0, len(releases))
	for _, release := range releases {
		releaseAssets := assetsByRelease[release.ID]

		// 检查该版本是否有正在进行的任务
		var syncing bool
		for _, a := range releaseAssets {
			if a.Status == models.AssetStatusPending || a.Status == models.AssetStatusDownloading {
				syncing = true
				break
			}
		}

		label := release.Tag
		if syncing {
			label = release.Tag + " (同步中)"
		}

		children := make([]treeNode, 0, len(releaseAssets))
		for _, a := range releaseAssets {
			assetLabel := a.Name
			switch a.Status {
			case models.AssetStatusPending:
				assetLabel = a.Name + " (待下载)"
			case models.AssetStatusDownloading:
				assetLabel = a.Name + " (下载中)"
			case models.AssetStatusFailed:
				assetLabel = a.Name + " (失败)"
			}
			children = append(children, treeNode{
				Key:          fmt.Sprintf("asset-%d", a.ID),
				Label:        assetLabel,
				IsLeaf:       true,
				Prefix:       assetPrefix(a.Status),
				Status:       string(a.Status),
				AssetID:      a.ID,
				Size:         a.Size,
				SHA256:       a.SHA256,
				StoragePath:  a.StoragePath,
				DownloadedAt: formatTime(a.DownloadedAt),
			})
		}

		nodes = append(nodes, treeNode{
			Key:       fmt.Sprintf("release-%d", release.ID),
			Label:     label,
			IsLeaf:    false,
			Prefix:    "🏷️",
			ReleaseID: release.ID,
			Children:  children,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tree": nodes})
}

// assetPrefix 根据资产状态返回图标前缀
func assetPrefix(status models.AssetStatus) string {
	switch status {
	case models.AssetStatusVerified, models.AssetStatusDownloaded:
		return "📄"
	case models.AssetStatusDownloading:
		return "⬇️"
	case models.AssetStatusPending:
		return "⏳"
	case models.AssetStatusFailed:
		return "❌"
	default:
		return "📄"
	}
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
