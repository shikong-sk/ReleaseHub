package notify

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"releasehub/backend/internal/models"

	"gorm.io/gorm"
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
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
			errs = append(errs, fmt.Errorf("%s: %w", notification.Name, err))
			continue
		}
		if err := notifier.Send(ctx, title, message); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", notification.Name, err))
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
