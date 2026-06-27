package api

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

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

type reconcileRequest struct {
	DryRun bool `json:"dryRun"`
}

type reconcileItem struct {
	StorageName string `json:"storageName"`
	StorageType string `json:"storageType"`
	Path        string `json:"path"`
	Owner       string `json:"owner,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Tag         string `json:"tag,omitempty"`
	Filename    string `json:"filename,omitempty"`
	Size        int64  `json:"size,omitempty"`
	AssetID     uint   `json:"assetId,omitempty"`
}

type reconcileResult struct {
	DryRun            bool            `json:"dryRun"`
	MissingInStorage  []reconcileItem `json:"missingInStorage"`
	MissingInDB       []reconcileItem `json:"missingInDB"`
	RepairedInDB      []reconcileItem `json:"repairedInDB"`
	ResetToPending    []reconcileItem `json:"resetToPending"`
	StorageScanErrors []string        `json:"storageScanErrors"`
	TotalStorageFiles int             `json:"totalStorageFiles"`
	TotalDBAssets     int             `json:"totalDBAssets"`
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
	var req reconcileRequest
	req.DryRun = true // 默认安全预检模式
	// 尝试解析请求体，成功则覆盖默认值
	_ = c.ShouldBindJSON(&req)

	result := reconcileResult{
		DryRun:            req.DryRun,
		MissingInStorage:  []reconcileItem{},
		MissingInDB:       []reconcileItem{},
		RepairedInDB:      []reconcileItem{},
		ResetToPending:    []reconcileItem{},
		StorageScanErrors: []string{},
	}
	ctx := c.Request.Context()

	// 加载所有存储配置
	var storages []models.Storage
	if err := h.db.WithContext(ctx).Find(&storages).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询存储配置失败")
		return
	}

	// 构建数据库资产索引：storagePath -> Asset
	var dbAssets []models.Asset
	if err := h.db.WithContext(ctx).
		Where("status NOT IN ?", []models.AssetStatus{models.AssetStatusDeleted, models.AssetStatusSkipped}).
		Find(&dbAssets).Error; err != nil {
		writeError(c, http.StatusInternalServerError, "查询资产失败")
		return
	}
	result.TotalDBAssets = len(dbAssets)

	// 按 (存储ID, 路径) 建索引，区分不同存储上的同名文件
	type dbKey struct {
		StorageID uint
		Path      string
	}
	dbPathIndex := make(map[dbKey]*models.Asset, len(dbAssets))
	for i := range dbAssets {
		// 优先使用 Asset 自身的 StorageID（文件实际所在存储），回退到 Repository 配置
		var assetStorageID uint
		if dbAssets[i].StorageID != nil && *dbAssets[i].StorageID > 0 {
			assetStorageID = *dbAssets[i].StorageID
		} else {
			// 回退：通过 Release→Repository 链推断
			var release models.Release
			if err := h.db.WithContext(ctx).First(&release, dbAssets[i].ReleaseID).Error; err == nil {
				var repo models.Repository
			if err := h.db.WithContext(ctx).First(&repo, release.RepositoryID).Error; err == nil {
					if repo.StorageID != nil {
						assetStorageID = *repo.StorageID
					} else {
						// 使用默认存储
						for _, s := range storages {
							if s.IsDefault {
								assetStorageID = s.ID
								break
							}
						}
					}
				}
			}
		}
		if dbAssets[i].StoragePath != "" {
			dbPathIndex[dbKey{StorageID: assetStorageID, Path: dbAssets[i].StoragePath}] = &dbAssets[i]
		}
	}

	// 用于反向检测：收集每个存储实际存在的文件路径集合
	storageFileSets := make(map[uint]map[string]bool)

	// ====== 阶段一：存储→DB 检测（遍历实际存储文件） ======
	for _, storageModel := range storages {
		driver, err := storage.NewDriverFromModel(storageModel)
		if err != nil {
			result.StorageScanErrors = append(result.StorageScanErrors,
				fmt.Sprintf("存储 %s (%s): 创建驱动失败 - %v", storageModel.Name, storageModel.Type, err))
			continue
		}

		lister, ok := driver.(storage.Lister)
		if !ok {
			h.logger.Info("存储驱动不支持 List，跳过存储→DB 检测",
				zap.String("storage", storageModel.Name),
				zap.String("type", storageModel.Type))
			continue
		}

		// 只扫描 ReleaseHub 管理的 github/ 子目录，避免误报非管理文件
		storageFiles, err := lister.List(ctx, "github")
		if err != nil {
			result.StorageScanErrors = append(result.StorageScanErrors,
				fmt.Sprintf("存储 %s (%s): 扫描失败 - %v", storageModel.Name, storageModel.Type, err))
			continue
		}
		result.TotalStorageFiles += len(storageFiles)

		fileSet := make(map[string]bool, len(storageFiles))
		for _, sf := range storageFiles {
			fileSet[sf.Path] = true

			// 只处理 github/ 前缀下的文件，其他目录的文件不属于 ReleaseHub 管理
			if !strings.HasPrefix(sf.Path, "github/") {
				continue
			}

			// 检查 DB 中是否存在该存储+路径的对应记录
			key := dbKey{StorageID: storageModel.ID, Path: sf.Path}
			if _, exists := dbPathIndex[key]; exists {
				continue // 该存储上有对应的 DB 记录，一致
			}

			// 存储中有文件但 DB 中没有 → MissingInDB
			parsed, parseErr := parseStoragePath(sf.Path)
			item := reconcileItem{
				StorageName: storageModel.Name,
				StorageType: storageModel.Type,
				Path:        sf.Path,
				Size:        sf.Size,
			}
			if parseErr == nil {
				item.Owner = parsed.owner
				item.Repo = parsed.repo
				item.Tag = parsed.tag
				item.Filename = parsed.filename
			}

			result.MissingInDB = append(result.MissingInDB, item)

			// 非 dryRun 模式下修复 DB
			if !req.DryRun && parseErr == nil {
				if repaired, repairErr := h.repairDB(ctx, storageModel, parsed, sf); repairErr != nil {
					h.logger.Error("修复 DB 记录失败",
						zap.String("path", sf.Path),
						zap.Error(repairErr))
				} else {
					result.RepairedInDB = append(result.RepairedInDB, *repaired)
				}
			}
		}
		storageFileSets[storageModel.ID] = fileSet
	}

	// ====== 阶段二：DB→存储 检测（检查数据库记录对应的文件是否实际存在） ======
	for i := range dbAssets {
		asset := dbAssets[i]
		if strings.TrimSpace(asset.StoragePath) == "" {
			continue
		}

		// 优先使用 Asset 自身的 StorageID 确定存储归属
		var assetStorageID uint
		if asset.StorageID != nil && *asset.StorageID > 0 {
			assetStorageID = *asset.StorageID
		} else {
			// 回退：通过 Release→Repository 链推断
			var release models.Release
			if err := h.db.WithContext(ctx).First(&release, asset.ReleaseID).Error; err != nil {
				continue
			}
			var repo models.Repository
			if err := h.db.WithContext(ctx).First(&repo, release.RepositoryID).Error; err != nil {
				continue
			}
			if repo.StorageID != nil {
				assetStorageID = *repo.StorageID
			} else {
				// 使用默认存储
				for _, s := range storages {
					if s.IsDefault {
						assetStorageID = s.ID
						break
					}
				}
			}
		}

		// 用 storageID 变量统一后续代码
		storageID := &assetStorageID

		// 检查文件是否在存储文件集合中
		fileSet, ok := storageFileSets[*storageID]
		if !ok {
			// 该存储未被扫描（可能不支持 List），用 Open 尝试检测
			// 根据 assetStorageID 直接创建存储驱动
			var storageModel models.Storage
			if err := h.db.WithContext(ctx).First(&storageModel, assetStorageID).Error; err != nil {
				continue
			}
			driver, err := storage.NewDriverFromModel(storageModel)
			if err != nil {
				continue
			}
			reader, _, err := driver.Open(ctx, asset.StoragePath)
			if err != nil {
				item := reconcileItem{
					StorageName: "未知",
					Path:        asset.StoragePath,
					AssetID:     asset.ID,
				}
				result.MissingInStorage = append(result.MissingInStorage, item)

				// 非 dryRun 模式下重置状态为 pending 以便重新下载
				if !req.DryRun {
					savedPath := asset.StoragePath
					asset.Status = models.AssetStatusPending
					asset.StoragePath = ""
					asset.ErrorMessage = "对账检测：存储文件丢失，已重置为待下载"
					asset.DownloadedAt = nil
					if err := h.db.WithContext(ctx).Save(&asset).Error; err != nil {
						h.logger.Error("重置资产状态失败", zap.Uint("assetId", asset.ID), zap.Error(err))
					} else {
						result.ResetToPending = append(result.ResetToPending, reconcileItem{
							StorageName: "未知",
							Path:        savedPath,
							AssetID:     asset.ID,
						})
					}
				}
			} else {
				reader.Close()
			}
			continue
		}

		// 使用文件集合检测
		if !fileSet[asset.StoragePath] {
			storageName := "未知"
			var storageModel models.Storage
			if err := h.db.WithContext(ctx).First(&storageModel, *storageID).Error; err == nil {
				storageName = storageModel.Name
			}

			item := reconcileItem{
				StorageName: storageName,
				Path:        asset.StoragePath,
				AssetID:     asset.ID,
			}
			result.MissingInStorage = append(result.MissingInStorage, item)

			// 非 dryRun 模式下重置状态
			if !req.DryRun {
				savedPath := asset.StoragePath
				asset.Status = models.AssetStatusPending
				asset.StoragePath = ""
				asset.ErrorMessage = "对账检测：存储文件丢失，已重置为待下载"
				asset.DownloadedAt = nil
				if err := h.db.WithContext(ctx).Save(&asset).Error; err != nil {
					h.logger.Error("重置资产状态失败", zap.Uint("assetId", asset.ID), zap.Error(err))
				} else {
					result.ResetToPending = append(result.ResetToPending, reconcileItem{
						StorageName: storageName,
						Path:        savedPath,
						AssetID:     asset.ID,
					})
				}
			}
		}
	}

	c.JSON(http.StatusOK, result)
}

// parsedStoragePath 解析后的存储路径组件
type parsedStoragePath struct {
	provider string
	owner    string
	repo     string
	tag      string
	filename string
}

// parseStoragePath 将存储路径解析为组件
// 路径格式：github/owner/repo/tag/filename
func parseStoragePath(path string) (*parsedStoragePath, error) {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	if len(parts) < 5 {
		return nil, fmt.Errorf("路径格式不正确，期望 provider/owner/repo/tag/filename: %s", path)
	}
	return &parsedStoragePath{
		provider: parts[0],
		owner:    parts[1],
		repo:     parts[2],
		tag:      parts[3],
		filename: strings.Join(parts[4:], "/"),
	}, nil
}

// repairDB 在 DB 中创建缺失的 Repository/Release/Asset 记录
func (h *reconcileHandler) repairDB(ctx context.Context, storageModel models.Storage, parsed *parsedStoragePath, fileResult storage.ListResult) (*reconcileItem, error) {
	// 查找或创建 Repository
	var repo models.Repository
	provider := "github"
	if parsed.provider != "" {
		provider = parsed.provider
	}
	err := h.db.WithContext(ctx).
		Where("provider = ? AND owner = ? AND repo = ?", provider, parsed.owner, parsed.repo).
		First(&repo).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			repo = models.Repository{
				Provider:        provider,
				Owner:           parsed.owner,
				Repo:            parsed.repo,
				Enabled:         true,
				StorageID:       &storageModel.ID,
				IntervalSeconds: 1800,
				LastStatus:      models.RepositoryStatusHealthy,
			}
			if err := h.db.WithContext(ctx).Create(&repo).Error; err != nil {
				return nil, fmt.Errorf("创建仓库记录失败: %w", err)
			}
		} else {
			return nil, fmt.Errorf("查询仓库失败: %w", err)
		}
	}

	// 查找或创建 Release
	var release models.Release
	err = h.db.WithContext(ctx).
		Where("repository_id = ? AND tag = ?", repo.ID, parsed.tag).
		First(&release).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			now := time.Now().UTC()
			release = models.Release{
				RepositoryID: repo.ID,
				Tag:          parsed.tag,
				Name:         parsed.tag,
				PublishedAt:  &now,
				SyncStatus:   "synced",
			}
			if err := h.db.WithContext(ctx).Create(&release).Error; err != nil {
				return nil, fmt.Errorf("创建 Release 记录失败: %w", err)
			}
		} else {
			return nil, fmt.Errorf("查询 Release 失败: %w", err)
		}
	}

	// 查找或创建 Asset（按 release_id + name + storage_id 查找，区分不同存储上的记录）
	var asset models.Asset
	err = h.db.WithContext(ctx).
		Where("release_id = ? AND name = ? AND storage_id = ?", release.ID, parsed.filename, storageModel.ID).
		First(&asset).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			now := time.Now().UTC()
			asset = models.Asset{
				ReleaseID:    release.ID,
				Name:         parsed.filename,
				Size:         fileResult.Size,
				StoragePath:  fileResult.Path,
				Status:       models.AssetStatusVerified,
				StorageID:    &storageModel.ID,
				DownloadedAt: &now,
			}
			if err := h.db.WithContext(ctx).Create(&asset).Error; err != nil {
				return nil, fmt.Errorf("创建 Asset 记录失败: %w", err)
			}
		} else {
			return nil, fmt.Errorf("查询 Asset 失败: %w", err)
		}
	} else {
		// Asset 已存在但 storage_path 或 status 可能不正确（预检已确认该存储文件不在 dbPathIndex 中）
		// 强制修正为 verified 并设置正确的 StorageID 和 StoragePath
		now := time.Now().UTC()
		asset.StoragePath = fileResult.Path
		asset.Status = models.AssetStatusVerified
		asset.StorageID = &storageModel.ID
		asset.Size = fileResult.Size
		asset.ErrorMessage = ""
		asset.DownloadedAt = &now
		if err := h.db.WithContext(ctx).Save(&asset).Error; err != nil {
			return nil, fmt.Errorf("更新 Asset 记录失败: %w", err)
		}
		// 返回修复结果，让上层统计修复数量
		return &reconcileItem{
			StorageName: storageModel.Name,
			StorageType: storageModel.Type,
			Path:        fileResult.Path,
			Owner:       parsed.owner,
			Repo:        parsed.repo,
			Tag:         parsed.tag,
			Filename:    parsed.filename,
			Size:        fileResult.Size,
			AssetID:     asset.ID,
		}, nil
	}

	// 更新仓库的最新版本信息
	if repo.LastReleaseTag == "" || repo.LastReleaseTag < parsed.tag {
		repo.LastReleaseTag = parsed.tag
		repo.LastStatus = models.RepositoryStatusHealthy
		now := time.Now().UTC()
		repo.LastCheckAt = &now
		if err := h.db.WithContext(ctx).Save(&repo).Error; err != nil {
			h.logger.Error("更新仓库最新版本失败", zap.Error(err))
		}
	}

	return &reconcileItem{
		StorageName: storageModel.Name,
		StorageType: storageModel.Type,
		Path:        fileResult.Path,
		Owner:       parsed.owner,
		Repo:        parsed.repo,
		Tag:         parsed.tag,
		Filename:    parsed.filename,
		Size:        fileResult.Size,
		AssetID:     asset.ID,
	}, nil
}
