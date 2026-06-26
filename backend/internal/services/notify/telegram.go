package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TelegramNotifier 通过 Telegram Bot API 发送通知
type TelegramNotifier struct {
	botToken string
	chatID   string
	client   *http.Client
}

func NewTelegramNotifier(botToken string, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// NewTelegramNotifierFromServerURL 从 serverURL (格式: botToken:chatID) 解析创建
func NewTelegramNotifierFromServerURL(serverURL string) (*TelegramNotifier, error) {
	parts := strings.SplitN(serverURL, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("Telegram serverURL 格式应为 botToken:chatID")
	}
	return NewTelegramNotifier(parts[0], parts[1]), nil
}

func (t *TelegramNotifier) Send(ctx context.Context, title string, message string) error {
	text := fmt.Sprintf("*%s*\n%s", escapeMarkdown(title), escapeMarkdown(message))
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.botToken)

	form := url.Values{}
	form.Set("chat_id", t.chatID)
	form.Set("text", text)
	form.Set("parse_mode", "MarkdownV2")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("创建 Telegram 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送 Telegram 通知失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Telegram API 返回异常状态: HTTP %d", resp.StatusCode)
	}
	return nil
}

// escapeMarkdown 转义 MarkdownV2 特殊字符
func escapeMarkdown(text string) string {
	special := []string{"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+", "-", "=", "|", "{", "}", ".", "!"}
	result := text
	for _, ch := range special {
		replacement := "\\" + ch
		result = strings.ReplaceAll(result, ch, replacement)
	}
	return result
}
