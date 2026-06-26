package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"releasehub/backend/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	db     *gorm.DB
	logger *zap.Logger
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, logger: zap.NewNop()}
}

// NewServiceWithLogger 创建带日志的通知服务
func NewServiceWithLogger(db *gorm.DB, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{db: db, logger: logger}
}

func (s *Service) Notify(ctx context.Context, event Event, title string, message string) error {
	var notifications []models.Notification
	if err := s.db.WithContext(ctx).
		Where("enabled = ?", true).
		Order("created_at ASC").
		Find(&notifications).Error; err != nil {
		return err
	}

	errs := make([]error, 0)
	for _, notification := range notifications {
		if !EventEnabled(notification.Events, event) {
			continue
		}

		notifier, err := NewNotifierFromModel(notification)
		if err != nil {
			createErr := fmt.Errorf("%s: %w", notification.Name, err)
			errs = append(errs, createErr)
			s.logger.Warn("创建通知渠道失败",
				zap.String("channel", notification.Name),
				zap.Error(createErr))
			continue
		}
		if err := notifier.Send(ctx, title, message); err != nil {
			sendErr := fmt.Errorf("%s: %w", notification.Name, err)
			errs = append(errs, sendErr)
			s.logger.Warn("通知发送失败",
				zap.String("channel", notification.Name),
				zap.String("event", string(event)),
				zap.Error(sendErr))
		}
	}

	return errors.Join(errs...)
}

func EventEnabled(events string, event Event) bool {
	if strings.TrimSpace(events) == "" || strings.TrimSpace(events) == "*" {
		return true
	}
	for _, e := range splitEvents(events) {
		if Event(e) == event || e == "*" {
			return true
		}
	}
	return false
}

func splitEvents(events string) []string {
	var result []string
	for _, segment := range strings.Split(events, ",") {
		segment = strings.TrimSpace(segment)
		if segment != "" {
			result = append(result, segment)
		}
	}
	return result
}
