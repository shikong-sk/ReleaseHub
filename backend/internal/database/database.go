package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	if cfg.Driver != "sqlite" {
		return nil, fmt.Errorf("暂不支持数据库类型: %s", cfg.Driver)
	}

	dsn := filepath.Clean(cfg.DSN)
	if shouldCreateSQLiteDir(dsn) {
		if err := os.MkdirAll(filepath.Dir(dsn), 0o755); err != nil {
			return nil, err
		}
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, err
	}

	return db, nil
}

func shouldCreateSQLiteDir(dsn string) bool {
	if dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return false
	}

	dir := filepath.Dir(dsn)
	return dir != "." && dir != ""
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.GitHubToken{},
		&models.Storage{},
		&models.Repository{},
		&models.RepositoryStorage{},
		&models.Release{},
		&models.Asset{},
		&models.Task{},
		&models.Proxy{},
		&models.Notification{},
		&models.TaskLog{},
		&models.User{},
		&models.APIKey{},
	)
}

// MigrateAssetUniqueIndex 迁移 Asset 表的唯一索引
// 从 (release_id, name) 改为 (release_id, name, storage_id)
// SQLite 不支持 ALTER INDEX，需要手动处理
func MigrateAssetUniqueIndex(db *gorm.DB) error {
	// 检查旧索引是否存在
	var count int64
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_release_asset_name'").Scan(&count)
	if count > 0 {
		// 删除旧索引
		if err := db.Exec("DROP INDEX IF EXISTS idx_release_asset_name").Error; err != nil {
			return fmt.Errorf("删除旧索引失败: %w", err)
		}
	}

	// 检查新索引是否已存在
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_release_asset_storage'").Scan(&count)
	if count == 0 {
		// 创建新索引
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_release_asset_storage ON assets(release_id, name, storage_id) WHERE deleted_at IS NULL").Error; err != nil {
			// 如果有重复数据导致索引创建失败，先清理重复记录
			fmt.Printf("[MigrateAssetUniqueIndex] 创建索引失败，尝试清理重复数据: %v\n", err)
			cleanDuplicateAssets(db)
			if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_release_asset_storage ON assets(release_id, name, storage_id) WHERE deleted_at IS NULL").Error; err != nil {
				return fmt.Errorf("创建新索引失败: %w", err)
			}
		}
	}

	return nil
}

// cleanDuplicateAssets 清理重复的 asset 记录（保留最新的）
func cleanDuplicateAssets(db *gorm.DB) {
	db.Exec(`DELETE FROM assets WHERE id NOT IN (
		SELECT MAX(id) FROM assets WHERE deleted_at IS NULL
		GROUP BY release_id, name, COALESCE(storage_id, 0)
	) AND deleted_at IS NULL`)
}

// SeedDefaultAdmin 确保数据库中至少存在一个管理员账户。
// 仅在 User 表为空时创建默认管理员，不会覆盖已有数据。
// username/password 为空时使用 "admin"/"admin"。
func SeedDefaultAdmin(db *gorm.DB, username string, password string) error {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("查询用户数量失败: %w", err)
	}
	if count > 0 {
		return nil
	}

	if username == "" {
		username = "admin"
	}
	if password == "" {
		password = "admin"
	}

	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("生成默认密码哈希失败: %w", err)
	}

	admin := models.User{
		Username:     username,
		PasswordHash: hash,
		Role:         "admin",
		Enabled:      true,
	}
	if err := db.Create(&admin).Error; err != nil {
		return fmt.Errorf("创建默认管理员失败: %w", err)
	}

	return nil
}

// hashPassword 使用 bcrypt 生成密码哈希
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}


// SeedDefaultStorage 确保数据库中存在一条默认本地存储记录。
// 如果没有 is_default=true 的存储配置，自动创建一条指向 data/releases 的本地存储。
func SeedDefaultStorage(db *gorm.DB, dataDir string) error {
	var count int64
	if err := db.Model(&models.Storage{}).Where("is_default = ?", true).Count(&count).Error; err != nil {
		return fmt.Errorf("查询默认存储失败: %w", err)
	}
	if count > 0 {
		return nil
	}

	basePath := dataDir
	if basePath == "" {
		basePath = "./data/releases"
	}

	defaultStorage := models.Storage{
		Name:      "默认本地存储",
		Type:      "local",
		BasePath:  basePath,
		IsDefault: true,
	}
	if err := db.Create(&defaultStorage).Error; err != nil {
		return fmt.Errorf("创建默认本地存储失败: %w", err)
	}

	return nil
}

// BackfillAssetStorageID 为已有资产回填 StorageID 字段
// 资产的 StorageID 记录文件实际所在的存储，新建资产在下载时已自动设置。
// 此函数处理存量数据：StorageID 为 NULL 的资产，按其仓库配置回填。
func BackfillAssetStorageID(db *gorm.DB) error {
	// 查找默认存储 ID
	var defaultStorage models.Storage
	hasDefault := true
	if err := db.Where("is_default = ?", true).Order("updated_at DESC, created_at DESC").First(&defaultStorage).Error; err != nil {
		hasDefault = false
	}

	// 查询所有 StorageID 为 NULL 且有 storage_path 的资产
	var assets []models.Asset
	if err := db.Where("storage_id IS NULL AND storage_path != '' AND status IN ?",
		[]models.AssetStatus{models.AssetStatusVerified, models.AssetStatusDownloaded}).
		Find(&assets).Error; err != nil {
		return fmt.Errorf("查询需要回填的资产失败: %w", err)
	}

	if len(assets) == 0 {
		return nil
	}

	// 预加载仓库映射: release_id -> repository
	releaseIDs := make([]uint, 0, len(assets))
	for _, a := range assets {
		releaseIDs = append(releaseIDs, a.ReleaseID)
	}

	var releases []models.Release
	if err := db.Where("id IN ?", releaseIDs).Find(&releases).Error; err != nil {
		return fmt.Errorf("查询 Release 失败: %w", err)
	}
	releaseMap := make(map[uint]models.Release, len(releases))
	for _, r := range releases {
		releaseMap[r.ID] = r
	}

	// 查询所有仓库
	repoIDs := make([]uint, 0)
	for _, r := range releases {
		repoIDs = append(repoIDs, r.RepositoryID)
	}
	var repos []models.Repository
	if err := db.Where("id IN ?", repoIDs).Find(&repos).Error; err != nil {
		return fmt.Errorf("查询仓库失败: %w", err)
	}
	repoMap := make(map[uint]models.Repository, len(repos))
	for _, r := range repos {
		repoMap[r.ID] = r
	}

	updated := 0
	for _, asset := range assets {
		release, ok := releaseMap[asset.ReleaseID]
		if !ok {
			continue
		}
		repo, ok := repoMap[release.RepositoryID]
		if !ok {
			continue
		}

		var storageID uint
		if repo.StorageID != nil {
			storageID = *repo.StorageID
		} else if hasDefault {
			storageID = defaultStorage.ID
		} else {
			continue
		}

		if err := db.Model(&models.Asset{}).Where("id = ?", asset.ID).
			Update("storage_id", storageID).Error; err != nil {
			return fmt.Errorf("更新资产 %d 的 StorageID 失败: %w", asset.ID, err)
		}
		updated++
	}

	if updated > 0 {
		fmt.Printf("[BackfillAssetStorageID] 已回填 %d 条资产的 StorageID\n", updated)
	}

	return nil
}
