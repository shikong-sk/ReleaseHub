package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Open(cfg config.DatabaseConfig) (*gorm.DB, error) {
	switch cfg.Driver {
	case "postgres":
		db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
			TranslateError: true,
		})
		if err != nil {
			return nil, fmt.Errorf("连接 PostgreSQL 失败: %w", err)
		}
		return db, nil
	case "mysql":
		db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
			TranslateError: true,
		})
		if err != nil {
			return nil, fmt.Errorf("连接 MySQL 失败: %w", err)
		}
		return db, nil
	case "sqlite", "":
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
	default:
		return nil, fmt.Errorf("不支持的数据库类型: %s", cfg.Driver)
	}
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
		&models.NotificationLog{},
		&models.TaskLog{},
		&models.User{},
		&models.APIKey{},
		&models.AppSetting{},
		&models.OperationLog{},
	)
}

// MigrateAssetUniqueIndex 迁移 Asset 表的唯一索引
// 从 (release_id, name) 改为 (release_id, name, storage_id)
// 使用 GORM Migrator API 实现跨数据库兼容（SQLite/PostgreSQL/MySQL）
func MigrateAssetUniqueIndex(db *gorm.DB) error {
	migrator := db.Migrator()

	// 检查旧索引是否存在，存在则删除
	if migrator.HasIndex(&models.Asset{}, "idx_release_asset_name") {
		if err := migrator.DropIndex(&models.Asset{}, "idx_release_asset_name"); err != nil {
			return fmt.Errorf("删除旧索引失败: %w", err)
		}
	}

	// 检查新索引是否存在，不存在则创建
	if !migrator.HasIndex(&models.Asset{}, "idx_release_asset_storage") {
		// 创建新索引，如果因重复数据失败则先清理再重试
		if err := createAssetUniqueIndex(db); err != nil {
			fmt.Printf("[MigrateAssetUniqueIndex] 创建索引失败，尝试清理重复数据: %v\n", err)
			cleanDuplicateAssets(db)
			if err := createAssetUniqueIndex(db); err != nil {
				return fmt.Errorf("创建新索引失败: %w", err)
			}
		}
	}

	return nil
}

// createAssetUniqueIndex 通过 Migrator 创建 assets 表的多列唯一索引
func createAssetUniqueIndex(db *gorm.DB) error {
	return db.Migrator().CreateIndex(&models.Asset{}, "idx_release_asset_storage")
}

// cleanDuplicateAssets 清理重复的 asset 记录（保留最新的）
func cleanDuplicateAssets(db *gorm.DB) {
	db.Exec(`DELETE FROM assets WHERE id NOT IN (
		SELECT MAX(id) FROM assets
		GROUP BY release_id, name, COALESCE(storage_id, 0)
	)`)
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


// MigrateDropDeletedAt 清理软删除列：删除已被软删除的记录，然后处理旧索引
// 此函数仅在从软删除迁移到硬删除时需要执行一次
// 使用 GORM Migrator API 实现跨数据库兼容（SQLite/PostgreSQL/MySQL）
func MigrateDropDeletedAt(db *gorm.DB) error {
	// 已迁移过的标记，避免每次启动重复执行
	var marker models.AppSetting
	if err := db.Where("key = ?", "migrate.drop_deleted_at_done").First(&marker).Error; err == nil {
		return nil
	}

	migrator := db.Migrator()

	// 跨数据库检测：使用 Migrator.HasColumn 检查 assets 表是否有 deleted_at 列
	if !migrator.HasColumn(&models.Asset{}, "deleted_at") {
		// 没有 deleted_at 列，标记已完成
		db.Save(&models.AppSetting{Key: "migrate.drop_deleted_at_done", Value: "true"})
		return nil
	}

	fmt.Println("[MigrateDropDeletedAt] 检测到 deleted_at 列，开始迁移...")

	// 删除所有已软删除的记录（deleted_at 不为 NULL 的记录）
	// 遍历所有可能存在 deleted_at 列的表，用 Migrator.HasColumn 逐个检测
	tables := []string{"assets", "releases", "repositories", "tasks", "storages",
		"proxies", "notifications", "git_hub_tokens", "users", "api_keys"}

	for _, table := range tables {
		// 检查该表是否有 deleted_at 列（有些表可能从没用过软删除）
		if !migrator.HasTable(table) {
			continue
		}
		if !hasColumnRaw(db, table, "deleted_at") {
			continue
		}
		var count int64
		db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE deleted_at IS NOT NULL", table)).Scan(&count)
		if count > 0 {
			if err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE deleted_at IS NOT NULL", table)).Error; err != nil {
				fmt.Printf("[MigrateDropDeletedAt] 清理 %s 软删除记录失败: %v\n", table, err)
			} else {
				fmt.Printf("[MigrateDropDeletedAt] 清理 %s 中 %d 条软删除记录\n", table, count)
			}
		}
	}

	// 删除旧唯一索引（可能包含 WHERE deleted_at IS NULL 条件），使用 Migrator 跨库删除
	if migrator.HasIndex(&models.Asset{}, "idx_release_asset_storage") {
		_ = migrator.DropIndex(&models.Asset{}, "idx_release_asset_storage")
	}

	// 重建不含 WHERE 条件的唯一索引
	if err := createAssetUniqueIndex(db); err != nil {
		fmt.Printf("[MigrateDropDeletedAt] 创建索引失败，尝试清理重复数据: %v\n", err)
		cleanDuplicateAssets(db)
		if err := createAssetUniqueIndex(db); err != nil {
			return fmt.Errorf("创建新索引失败: %w", err)
		}
	}

	// 标记迁移完成
	db.Save(&models.AppSetting{Key: "migrate.drop_deleted_at_done", Value: "true"})

	fmt.Println("[MigrateDropDeletedAt] 迁移完成")
	return nil
}

// hasColumnRaw 通过 information_schema 或回退方案检测某表是否有某列
// GORM Migrator.HasColumn 需要模型对象，这里用 Raw 查询支持任意表名
func hasColumnRaw(db *gorm.DB, tableName, columnName string) bool {
	switch db.Dialector.Name() {
	case "postgres":
		var count int64
		db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&count)
		return count > 0
	case "mysql":
		var count int64
		db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?`,
			tableName, columnName).Scan(&count)
		return count > 0
	default: // sqlite
		var count int64
		db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name='%s'", tableName, columnName)).Scan(&count)
		return count > 0
	}
}

// MigrateBackfillDownloadBytes 回填存量数据：
// 已下载成功（downloaded/verified）但 download_bytes=0 的资产，用 size 值补上
// 仅执行一次，用 AppSetting 标记
func MigrateBackfillDownloadBytes(db *gorm.DB) error {
	var marker models.AppSetting
	if err := db.Where("key = ?", "migrate.backfill_download_bytes_done").First(&marker).Error; err == nil {
		return nil
	}

	result := db.Model(&models.Asset{}).
		Where("download_bytes = 0 AND size > 0 AND status IN ?", []string{"downloaded", "verified"}).
		Update("download_bytes", gorm.Expr("assets.size"))
	if result.Error != nil {
		return result.Error
	}

	if err := db.Save(&models.AppSetting{Key: "migrate.backfill_download_bytes_done", Value: "true"}).Error; err != nil {
		return fmt.Errorf("标记迁移完成失败: %w", err)
	}
	if result.RowsAffected > 0 {
		fmt.Printf("[MigrateBackfillDownloadBytes] 回填 %d 条资产\n", result.RowsAffected)
	}
	return nil
}
