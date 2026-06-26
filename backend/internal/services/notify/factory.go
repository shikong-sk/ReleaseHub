package notify

import (
	"fmt"

	"releasehub/backend/internal/models"
)

func NewNotifierFromModel(notification models.Notification) (Notifier, error) {
	switch notification.Type {
	case "gotify":
		if notification.ServerURL == "" {
			return nil, fmt.Errorf("Gotify 需要配置 ServerURL")
		}
		return NewGotifyNotifier(notification.ServerURL, notification.Token), nil
	case "webhook":
		if notification.ServerURL == "" {
			return nil, fmt.Errorf("Webhook 需要配置 ServerURL")
		}
		return NewWebhookNotifier(notification.ServerURL, notification.Token), nil
	case "telegram":
		notifier, err := NewTelegramNotifierFromServerURL(notification.ServerURL)
		if err != nil {
			return nil, fmt.Errorf("Telegram 配置无效: %w", err)
		}
		return notifier, nil
	case "email":
		notifier, err := NewEmailNotifier(notification.ServerURL, notification.Token)
		if err != nil {
			return nil, fmt.Errorf("邮件配置无效: %w", err)
		}
		return notifier, nil
	default:
		return nil, fmt.Errorf("不支持的通知类型: %s", notification.Type)
	}
}
