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
	// 支持按状态过滤，默认排除 deleted/skipped
	statusFilter := c.Query("status")
	
	var assets []models.Asset
	query := h.db.WithContext(c.Request.Context())
	if statusFilter != "" {
		query = query.Where("assets.status = ?", statusFilter)
	} else {
		query = query.Where("assets.status NOT IN ?", []models.AssetStatus{models.AssetStatusDeleted, models.AssetStatusSkipped})
	}
	err := query.
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

		// 优先使用 Asset 自身的 StorageID（文件实际所在存储），回退到仓库配置
		assetStorageID := asset.StorageID
		if assetStorageID == nil {
			assetStorageID = repository.StorageID
		}
		storageName := "默认本地存储"
		storageType := "local"
		if assetStorageID != nil {
			if storage, ok := storagesByID[*assetStorageID]; ok {
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
			StorageID:    assetStorageID,
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
		storageQuery := c.Query("storageId")
		h.treeForRepository(c, repoQuery, storageQuery)
		return
	}
	h.treeTopLevel(c)
}

// treeForStorage 返回指定存储下的仓库列表（用于存储页面的懒加载）
func (h *fileHandler) treeForStorage(c *gin.Context) {
	storageQuery := c.Query("storageId")
	if storageQuery == "" {
		h.treeTopLevel(c)
		return
	}
	storageID, err := strconv.ParseUint(storageQuery, 10, 64)
	if err != nil || storageID == 0 {
		writeError(c, http.StatusBadRequest, "storageId 必须是正整数")
		return
	}
	// 复用 treeTopLevel 的逻辑，但只返回指定存储下的仓库
	h.treeTopLevel(c)
}

type treeNode struct {
	Key      string     `json:"key"`
	Label    string     `json:"label"`
	IsLeaf   bool       `json:"isLeaf"`
	Children []treeNode `json:"children,omitempty"`
	Prefix   string     `json:"prefix,omitempty"`

	// 存储层附加字段（存储节点和文件节点均使用）
	StorageID    uint   `json:"storageId"`

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
	ctx := c.Request.Context()

	// 查询所有 enabled 的仓库
	var repos []models.Repository
	if err := h.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("owner, repo").
		Find(&repos).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询仓库列表失败")
		return
	}

	// 预加载存储配置
	storagesByID := map[uint]models.Storage{}
	var defaultStorageID uint
	{
		var storages []models.Storage
		if err := h.db.WithContext(ctx).Find(&storages).Error; err != nil {
			writeError(c, http.StatusInternalServerError, "查询存储配置失败")
			return
		}
		for _, s := range storages {
			storagesByID[s.ID] = s
			if s.IsDefault && defaultStorageID == 0 {
				defaultStorageID = s.ID
			}
		}
	}

	// 构建 repository_id → storage_id 映射（仓库当前配置）
	repoStorageMap := make(map[uint]uint, len(repos))
	for _, repo := range repos {
		if repo.StorageID != nil {
			repoStorageMap[repo.ID] = *repo.StorageID
		} else {
			repoStorageMap[repo.ID] = defaultStorageID
		}
	}

	// 查询所有非 deleted/skipped 的资产，按 release_id 找到 repository_id，
	// 再按 asset.storage_id 或 repository.storage_id 确定实际存储归属
	// 使用 COALESCE 在 SQL 层完成回填
	type assetCountByStorageRepo struct {
		EffectiveStorageID uint
		RepositoryID       uint
		FileCount          int
	}
	var counts []assetCountByStorageRepo
	// 用 Sprintf 预构建 COALESCE 表达式，因为 GORM 的 Select/Group 不支持多参数占位
	coalesceExpr := fmt.Sprintf("COALESCE(assets.storage_id, repositories.storage_id, %d)", defaultStorageID)
	_ = h.db.WithContext(ctx).
		Model(&models.Asset{}).
		Select(fmt.Sprintf("%s as effective_storage_id, repositories.id as repository_id, COUNT(*) as file_count", coalesceExpr)).
		Joins("JOIN releases ON releases.id = assets.release_id").
		Joins("JOIN repositories ON repositories.id = releases.repository_id").
		Where("assets.status NOT IN ?", []models.AssetStatus{models.AssetStatusDeleted, models.AssetStatusSkipped}).
		Where("repositories.enabled = ?", true).
		Group(fmt.Sprintf("%s, repositories.id", coalesceExpr)).
		Find(&counts).Error

	// 构建 (storageID, repoID) → fileCount 映射
	countMap := make(map[[2]uint]int, len(counts))
	for _, rc := range counts {
		countMap[[2]uint{rc.EffectiveStorageID, rc.RepositoryID}] = rc.FileCount
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

	// 初始化所有存储组
	for _, s := range storagesByID {
		storageMap[s.ID] = &storageGroup{ID: s.ID, Name: s.Name, Type: s.Type, Children: make([]treeNode, 0)}
		storageOrder = append(storageOrder, s.ID)
	}

	// 如果没有存储记录但存在默认本地存储（配置中的 DataDir），添加虚拟的默认存储节点
	if len(storagesByID) == 0 {
		virtualID := uint(0)
		storageMap[virtualID] = &storageGroup{ID: virtualID, Name: "默认本地存储", Type: "local", Children: make([]treeNode, 0)}
		storageOrder = append(storageOrder, virtualID)
	}

	for _, repo := range repos {
		// 该仓库的文件可能分布在多个存储上（历史原因）
		// 首先看仓库当前配置的存储
		primaryStorageID := repoStorageMap[repo.ID]

		// 检查该仓库在其他存储上是否也有文件（asset.storage_id 与 repo.storage_id 不同时）
		var otherStorageIDs []uint
		for _, rc := range counts {
			if rc.RepositoryID == repo.ID && rc.EffectiveStorageID != primaryStorageID {
				otherStorageIDs = append(otherStorageIDs, rc.EffectiveStorageID)
			}
		}

		// 在仓库当前配置的存储下显示该仓库节点
		fc := countMap[[2]uint{primaryStorageID, repo.ID}]
		repoNode := treeNode{
			Key:          fmt.Sprintf("repo-%d", repo.ID),
			Label:        fmt.Sprintf("%s/%s", repo.Owner, repo.Repo),
			IsLeaf:       false,
			Prefix:       "📁",
			StorageID:    primaryStorageID,
			RepositoryID: repo.ID,
			FileCount:    fc,
		}
		if _, ok := storageMap[primaryStorageID]; ok {
			storageMap[primaryStorageID].Children = append(storageMap[primaryStorageID].Children, repoNode)
		}

		// 如果仓库在其他存储上也有文件，也在对应存储下显示
		for _, otherID := range otherStorageIDs {
			otherFC := countMap[[2]uint{otherID, repo.ID}]
			otherNode := treeNode{
				Key:          fmt.Sprintf("repo-%d-s%d", repo.ID, otherID),
				Label:        fmt.Sprintf("%s/%s", repo.Owner, repo.Repo),
				IsLeaf:       false,
				Prefix:       "📁",
				StorageID:    otherID,
				RepositoryID: repo.ID,
				FileCount:    otherFC,
			}
			if _, ok := storageMap[otherID]; ok {
				storageMap[otherID].Children = append(storageMap[otherID].Children, otherNode)
			}
		}
	}

	// 组装顶层节点
	nodes := make([]treeNode, 0)
	for _, id := range storageOrder {
		group := storageMap[id]
		totalFiles := 0
		for _, child := range group.Children {
			totalFiles += child.FileCount
		}
		label := fmt.Sprintf("%s (%s)", group.Name, strings.ToUpper(group.Type))
		if totalFiles > 0 {
			label += fmt.Sprintf(" — %d 文件", totalFiles)
		} else {
			label += " — 暂无文件"
		}
		children := group.Children
		if children == nil {
			children = make([]treeNode, 0)
		}
		nodes = append(nodes, treeNode{
			Key:       fmt.Sprintf("storage-%d", id),
			Label:     label,
			IsLeaf:    false,
			Prefix:    "💾",
			StorageID: id,
			Children:  children,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tree": nodes})
}

// treeForRepository 返回指定仓库的 版本→文件 子树
// storageFilter 参数可选，指定后只返回该存储上的文件
func (h *fileHandler) treeForRepository(c *gin.Context, repoQuery string, storageFilter string) {
	repoID, err := strconv.ParseUint(repoQuery, 10, 64)
	if err != nil || repoID == 0 {
		writeError(c, http.StatusBadRequest, "repositoryId 必须是正整数")
		return
	}

	ctx := c.Request.Context()

	// 查询该仓库下所有 Release（按 tag 倒序）
	var releases []models.Release
	if err := h.db.WithContext(ctx).
		Where("repository_id = ? ", repoID).
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

	// 解析可选的 storageId 过滤参数
	var filterStorageID *uint
	if storageFilter != "" {
		if sid, err := strconv.ParseUint(storageFilter, 10, 64); err == nil && sid > 0 {
			uid := uint(sid)
			filterStorageID = &uid
		}
	}

	// 查询这些 Release 下的所有资产（不限状态，排除 deleted 和 skipped）
	assetQuery := h.db.WithContext(ctx).
		Where("release_id IN ? AND status NOT IN ?", releaseIDs,
			[]models.AssetStatus{models.AssetStatusDeleted, models.AssetStatusSkipped})
	if filterStorageID != nil {
		// 查找默认存储 ID，用于处理 storage_id IS NULL 的资产
		var defaultSID uint
		_ = h.db.WithContext(ctx).
			Model(&models.Storage{}).
			Where("is_default = ?", true).
			Order("updated_at DESC, created_at DESC").
			Limit(1).
			Pluck("id", &defaultSID).Error
		if *filterStorageID == defaultSID {
			// 请求的是默认存储：包含 storage_id = defaultSID 或 storage_id IS NULL 的资产
			assetQuery = assetQuery.Where("(storage_id = ? OR storage_id IS NULL)", *filterStorageID)
		} else {
			assetQuery = assetQuery.Where("storage_id = ?", *filterStorageID)
		}
	}
	var assets []models.Asset
	if err := assetQuery.
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

		// 按 storageId 过滤后，某些版本可能没有资产，跳过这些空版本
		if len(releaseAssets) == 0 && filterStorageID != nil {
			continue
		}

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
				StorageID:    uintPtrToUint(a.StorageID),
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

// uintPtrToUint 将 *uint 转为 uint，nil 返回 0
func uintPtrToUint(v *uint) uint {
	if v == nil {
		return 0
	}
	return *v
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
