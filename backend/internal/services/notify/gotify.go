package notify

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GotifyNotifier 通过 Gotify 服务发送通知
type GotifyNotifier struct {
	serverURL string
	token     string
	client    *http.Client
}

func NewGotifyNotifier(serverURL string, token string) *GotifyNotifier {
	return &GotifyNotifier{
		serverURL: strings.TrimRight(serverURL, "/"),
		token:     token,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *GotifyNotifier) Send(ctx context.Context, title string, message string) error {
	endpoint := g.serverURL + "/message?token=" + url.QueryEscape(g.token)
	form := url.Values{}
	form.Set("title", title)
	form.Set("message", message)
	form.Set("priority", "5")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("创建 Gotify 请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("发送 Gotify 通知失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Gotify 返回异常状态: HTTP %d", resp.StatusCode)
	}
	return nil
}
