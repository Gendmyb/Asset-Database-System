package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// WeComNotifier 企业微信机器人 webhook 通知器
// 文档: https://developer.work.weixin.qq.com/document/path/91770
type WeComNotifier struct {
	webhookURL string
	client     *http.Client
}

func NewWeComNotifier(url string) *WeComNotifier {
	return &WeComNotifier{webhookURL: url, client: ssrfSafeClient()}
}

func (w *WeComNotifier) Channel() string { return "wecom" }

func (w *WeComNotifier) Available() bool { return w.webhookURL != "" }

func (w *WeComNotifier) Send(ctx context.Context, n Notification) error {
	if !w.Available() {
		return fmt.Errorf("wecom notifier: not configured")
	}
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": n.Subject + "\n" + n.Body,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("wecom notifier: marshal: %w", err)
	}
	return postRobotJSON(ctx, w.client, w.webhookURL, body)
}
