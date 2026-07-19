package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// FeishuNotifier 飞书机器人 webhook 通知器
// 文档: https://open.feishu.cn/document/client-docs/bot-v3/add-custom-bot
type FeishuNotifier struct {
	webhookURL string
	client     *http.Client
}

func NewFeishuNotifier(url string) *FeishuNotifier {
	return &FeishuNotifier{webhookURL: url, client: ssrfSafeClient()}
}

func (f *FeishuNotifier) Channel() string { return "feishu" }

func (f *FeishuNotifier) Available() bool { return f.webhookURL != "" }

func (f *FeishuNotifier) Send(ctx context.Context, n Notification) error {
	if !f.Available() {
		return fmt.Errorf("feishu notifier: not configured")
	}
	payload := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": n.Subject + "\n" + n.Body,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("feishu notifier: marshal: %w", err)
	}
	return postRobotJSON(ctx, f.client, f.webhookURL, body)
}
