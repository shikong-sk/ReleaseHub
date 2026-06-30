package notify

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"releasehub/backend/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Service struct {
	db     *gorm.DB
	logger *zap.Logger

	// ponytail: sync.Map 内存去重，单实例足够；多实例部署时换 Redis TTL key
	dedup   sync.Map
	dedupMu sync.Mutex
}

const dedupTTL = 5 * time.Minute

type dedupEntry struct {
	sentAt time.Time
}

func NewService(db *gorm.DB) *Service {
	s := &Service{db: db, logger: zap.NewNop()}
	go s.cleanDedup()
	return s
}

// NewServiceWithLogger 创建带日志的通知服务
func NewServiceWithLogger(db *gorm.DB, logger *zap.Logger) *Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	s := &Service{db: db, logger: logger}
	go s.cleanDedup()
	return s
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

		// 去重：同一渠道+事件+标题 5分钟内不重复推送
		dedupKey := fmt.Sprintf("%d:%s:%x", notification.ID, event, sha256.Sum256([]byte(title)))
		if v, ok := s.dedup.Load(dedupKey); ok {
			if entry := v.(*dedupEntry); time.Since(entry.sentAt) < dedupTTL {
				s.logger.Debug("通知去重跳过",
					zap.String("channel", notification.Name),
					zap.String("event", string(event)))
				continue
			}
		}

		notifier, err := NewNotifierFromModel(notification)
		if err != nil {
			createErr := fmt.Errorf("%s: %w", notification.Name, err)
			errs = append(errs, createErr)
			s.logger.Warn("创建通知渠道失败",
				zap.String("channel", notification.Name),
				zap.Error(createErr))
			s.logPush(ctx, notification.ID, notification.Name, event, title, message, false, createErr.Error())
			continue
		}
		if err := notifier.Send(ctx, title, message); err != nil {
			sendErr := fmt.Errorf("%s: %w", notification.Name, err)
			errs = append(errs, sendErr)
			s.logger.Warn("通知发送失败",
				zap.String("channel", notification.Name),
				zap.String("event", string(event)),
				zap.Error(sendErr))
			s.logPush(ctx, notification.ID, notification.Name, event, title, message, false, err.Error())
		} else {
			s.dedup.Store(dedupKey, &dedupEntry{sentAt: time.Now()})
			s.logPush(ctx, notification.ID, notification.Name, event, title, message, true, "")
		}
	}

	return errors.Join(errs...)
}

func (s *Service) logPush(ctx context.Context, notificationID uint, name string, event Event, title, message string, success bool, errMsg string) {
	log := models.NotificationLog{
		NotificationID:   notificationID,
		NotificationName: name,
		Event:            string(event),
		Title:            title,
		Message:          message,
		Success:          success,
		Error:            errMsg,
	}
	if err := s.db.WithContext(ctx).Create(&log).Error; err != nil {
		s.logger.Warn("写入通知推送日志失败", zap.Error(err))
	}
}

// cleanDedup 每分钟清理过期的去重条目
func (s *Service) cleanDedup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.dedup.Range(func(key, value any) bool {
			if entry, ok := value.(*dedupEntry); ok && now.Sub(entry.sentAt) > dedupTTL {
				s.dedup.Delete(key)
			}
			return true
		})
	}
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
