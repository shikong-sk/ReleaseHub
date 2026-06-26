package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookNotifier 通过 HTTP POST 发送 JSON 通知
type WebhookNotifier struct {
	serverURL string
	token     string
	client    *http.Client
}

func NewWebhookNotifier(serverURL string, token string) *WebhookNotifier {
	return &WebhookNotifier{
		serverURL: serverURL,
		token:     token,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *WebhookNotifier) Send(ctx context.Context, title string, message string) error {
	payload := map[string]string{
		"title":   title,
		"message": message,
	}
	if w.token != "" {
		payload["token"] = w.token
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 Webhook 载荷失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.serverURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("创建 Webhook 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if w.token != "" {
		req.Header.Set("X-Webhook-Token", w.token)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送 Webhook 通知失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Webhook 返回异常状态: HTTP %d", resp.StatusCode)
	}
	return nil
}
