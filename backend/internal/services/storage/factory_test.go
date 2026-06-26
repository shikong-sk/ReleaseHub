package storage

import (
	"context"
	"strings"
	"testing"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFactoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Storage{}, &models.Repository{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db
}

func TestDriverFactoryUsesRepositoryStorage(t *testing.T) {
	db := setupFactoryTestDB(t)
	storageRoot := t.TempDir()
	storageModel := models.Storage{
		Name:     "local-a",
		Type:     "local",
		BasePath: storageRoot,
	}
	if err := db.Create(&storageModel).Error; err != nil {
		t.Fatalf("create storage: %v", err)
	}

	repository := models.Repository{
		Provider:  "github",
		Owner:     "owner",
		Repo:      "repo",
		StorageID: &storageModel.ID,
	}
	factory := NewDriverFactory(db, config.StorageConfig{DataDir: t.TempDir()})

	driver, err := factory.DriverForRepository(context.Background(), repository)
	if err != nil {
		t.Fatalf("driver for repository: %v", err)
	}
	object, err := driver.Put(context.Background(), "github/owner/repo/v1/file.txt", strings.NewReader("ok"))
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	if !strings.HasPrefix(object.AbsPath, storageRoot) {
		t.Fatalf("expected object under repository storage %q, got %q", storageRoot, object.AbsPath)
	}
}

func TestDriverFactoryFallsBackToConfigStorage(t *testing.T) {
	db := setupFactoryTestDB(t)
	defaultRoot := t.TempDir()
	factory := NewDriverFactory(db, config.StorageConfig{DataDir: defaultRoot})

	driver, err := factory.DriverForRepository(context.Background(), models.Repository{})
	if err != nil {
		t.Fatalf("driver for repository: %v", err)
	}
	object, err := driver.Put(context.Background(), "github/owner/repo/v1/file.txt", strings.NewReader("ok"))
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	if !strings.HasPrefix(object.AbsPath, defaultRoot) {
		t.Fatalf("expected object under config storage %q, got %q", defaultRoot, object.AbsPath)
	}
}

func TestDriverFactoryReportsMissingRepositoryStorage(t *testing.T) {
	db := setupFactoryTestDB(t)
	missingID := uint(404)
	factory := NewDriverFactory(db, config.StorageConfig{DataDir: t.TempDir()})

	_, err := factory.DriverForRepository(context.Background(), models.Repository{StorageID: &missingID})
	if err == nil || !strings.Contains(err.Error(), "存储配置不存在") {
		t.Fatalf("expected missing storage error, got %v", err)
	}
}
