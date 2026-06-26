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
