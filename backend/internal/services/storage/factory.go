package storage

import (
	"context"
	"errors"
	"fmt"

	"releasehub/backend/internal/config"
	"releasehub/backend/internal/models"

	"gorm.io/gorm"
)

// DriverFactory 根据仓库配置解析实际使用的存储驱动。
type DriverFactory struct {
	db             *gorm.DB
	defaultDataDir string
}

func NewDriverFactory(db *gorm.DB, storageConfig config.StorageConfig) *DriverFactory {
	return &DriverFactory{
		db:             db,
		defaultDataDir: storageConfig.DataDir,
	}
}

func (f *DriverFactory) DriverForRepository(ctx context.Context, repository models.Repository) (Driver, error) {
	storageModel, ok, err := f.resolveStorage(ctx, repository)
	if err != nil {
		return nil, err
	}
	if !ok {
		return NewLocalStorage(f.defaultDataDir)
	}

	return NewDriverFromModel(storageModel)
}

func (f *DriverFactory) resolveStorage(ctx context.Context, repository models.Repository) (models.Storage, bool, error) {
	var storageModel models.Storage
	if repository.StorageID != nil {
		if err := f.db.WithContext(ctx).First(&storageModel, *repository.StorageID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return storageModel, false, fmt.Errorf("存储配置不存在 (id=%d)", *repository.StorageID)
			}
			return storageModel, false, err
		}
		return storageModel, true, nil
	}

	err := f.db.WithContext(ctx).
		Where("is_default = ?", true).
		Order("updated_at DESC, created_at DESC").
		First(&storageModel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return storageModel, false, nil
	}
	if err != nil {
		return storageModel, false, err
	}
	return storageModel, true, nil
}

func NewDriverFromModel(storageModel models.Storage) (Driver, error) {
	switch storageModel.Type {
	case "local":
		return NewLocalStorage(storageModel.BasePath)
	case "s3":
		return NewS3Storage(S3Config{
			Endpoint:  storageModel.Endpoint,
			Bucket:    storageModel.Bucket,
			Region:    storageModel.Region,
			AccessKey: storageModel.AccessKey,
			SecretKey: storageModel.SecretKey,
			Prefix:    storageModel.BasePath,
		})
	case "webdav":
		return NewWebDAVStorage(WebDAVConfig{
			URL:      storageModel.RemoteURL,
			Username: storageModel.Username,
			Password: storageModel.Password,
			BasePath: storageModel.BasePath,
		})
	default:
		return nil, fmt.Errorf("不支持的存储类型: %s", storageModel.Type)
	}
}
