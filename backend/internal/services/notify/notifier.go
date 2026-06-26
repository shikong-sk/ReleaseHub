package notify

import "context"

// Notifier 通知发送接口
type Notifier interface {
	// Send 发送通知，title 为标题，message 为正文
	Send(ctx context.Context, title string, message string) error
}

// Event 通知事件类型
type Event string

const (
	EventNewRelease  Event = "new_release"
	EventSyncSuccess Event = "sync_success"
	EventSyncFailed  Event = "sync_failed"
	EventDownloadOK  Event = "download_ok"
	EventDownloadErr Event = "download_err"
)
