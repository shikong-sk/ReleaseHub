package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"

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
	)
}
