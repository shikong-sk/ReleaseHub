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

// EventEnabled 检查事件是否在给定的 events 列表中启用
func EventEnabled(events string, event Event) bool {
	if events == "" || events == "*" {
		return true
	}
	// events 格式: "new_release,sync_failed"
	for _, e := range splitEvents(events) {
		if Event(e) == event || e == "*" {
			return true
		}
	}
	return false
}

func splitEvents(events string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(events); i++ {
		if i == len(events) || events[i] == ',' {
			segment := events[start:i]
			if segment != "" {
				result = append(result, segment)
			}
			start = i + 1
		}
	}
	return result
}
