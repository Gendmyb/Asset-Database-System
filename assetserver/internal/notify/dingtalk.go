package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DingTalkNotifier 钉钉机器人 webhook 通知器
// 文档: https://open.dingtalk.com/document/robots/custom-robot-access
type DingTalkNotifier struct {
	webhookURL string
	client     *http.Client
}

func NewDingTalkNotifier(url string) *DingTalkNotifier {
	return &DingTalkNotifier{webhookURL: url, client: ssrfSafeClient()}
}

func (d *DingTalkNotifier) Channel() string { return "dingtalk" }

func (d *DingTalkNotifier) Available() bool { return d.webhookURL != "" }

func (d *DingTalkNotifier) Send(ctx context.Context, n Notification) error {
	if !d.Available() {
		return fmt.Errorf("dingtalk notifier: not configured")
	}
	payload := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]string{
			"content": n.Subject + "\n" + n.Body,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("dingtalk notifier: marshal: %w", err)
	}
	return postRobotJSON(ctx, d.client, d.webhookURL, body)
}
