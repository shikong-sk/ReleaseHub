package api

import (
	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/notify"
)

// CreateNotifier 根据 Notification 模型配置创建对应的通知器
func CreateNotifier(n models.Notification) (notify.Notifier, error) {
	return notify.NewNotifierFromModel(n)
}
