package tasklog

import (
	"context"
	"time"

	"releasehub/backend/internal/models"

	"gorm.io/gorm"
)

// Service 任务日志服务
type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// Append 追加一条任务日志
func (s *Service) Append(ctx context.Context, taskID uint, level string, message string) error {
	entry := models.TaskLog{
		TaskID:    taskID,
		Level:     level,
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Create(&entry).Error
}

// List 查询任务日志（按时间降序）
func (s *Service) List(ctx context.Context, taskID uint, limit int) ([]models.TaskLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var logs []models.TaskLog
	err := s.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}

// ListRecent 查询最近的任务日志
func (s *Service) ListRecent(ctx context.Context, limit int) ([]models.TaskLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var logs []models.TaskLog
	err := s.db.WithContext(ctx).
		Order("timestamp DESC").
		Limit(limit).
		Find(&logs).Error
	return logs, err
}
