package api

import (
	"fmt"

	"releasehub/backend/internal/models"
	"releasehub/backend/internal/services/notify"
)

// CreateNotifier 根据 Notification 模型配置创建对应的通知器
func CreateNotifier(n models.Notification) (notify.Notifier, error) {
	switch n.Type {
	case "gotify":
		if n.ServerURL == "" {
			return nil, fmt.Errorf("Gotify 需要配置 ServerURL")
		}
		return notify.NewGotifyNotifier(n.ServerURL, n.Token), nil
	case "webhook":
		if n.ServerURL == "" {
			return nil, fmt.Errorf("Webhook 需要配置 ServerURL")
		}
		return notify.NewWebhookNotifier(n.ServerURL, n.Token), nil
	case "telegram":
		notifier, err := notify.NewTelegramNotifierFromServerURL(n.ServerURL)
		if err != nil {
			return nil, fmt.Errorf("Telegram 配置无效: %w", err)
		}
		return notifier, nil
	case "email":
		notifier, err := notify.NewEmailNotifier(n.ServerURL, n.Token)
		if err != nil {
			return nil, fmt.Errorf("邮件配置无效: %w", err)
		}
		return notifier, nil
	default:
		return nil, fmt.Errorf("不支持的通知类型: %s", n.Type)
	}
}
