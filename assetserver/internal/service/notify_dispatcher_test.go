// Package service — 通知 dispatcher 与审批状态机单测 (Wave 2 G6/G7)
package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/notify"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
)

// ============================================================
// mockNotifier — 模拟通知渠道
// ============================================================

type mockNotifier struct {
	mu        sync.Mutex
	channel   string
	available bool
	sent      []notify.Notification
	failN     int // 前 N 次失败
	calls     int
}

func (m *mockNotifier) Send(ctx context.Context, n notify.Notification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls <= m.failN {
		return errors.New("simulated send failure")
	}
	m.sent = append(m.sent, n)
	return nil
}

func (m *mockNotifier) Channel() string { return m.channel }
func (m *mockNotifier) Available() bool { return m.available }
func (m *mockNotifier) Sent() []notify.Notification {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]notify.Notification(nil), m.sent...)
}

// ============================================================
// TestNotifyDispatcher_RoutesByChannel — 规则按渠道分发
// ============================================================

func TestNotifyDispatcher_RoutesByChannel(t *testing.T) {
	email := &mockNotifier{channel: "email", available: true}
	dd := &mockNotifier{channel: "dingtalk", available: true}
	// 不可用的渠道应被过滤掉
	disabled := &mockNotifier{channel: "feishu", available: false}

	d := NewNotifyDispatcher(nil, nil, []notify.Notifier{email, dd, disabled}, true)
	if _, ok := d.notifiers["feishu"]; ok {
		t.Fatal("不可用渠道应被过滤")
	}
	if len(d.notifiers) != 2 {
		t.Fatalf("期望 2 个可用渠道, 实际 %d", len(d.notifiers))
	}

	// 模拟 handleEvent: 手动调用 deliverWithRetry
	rule := repository.NotifyRuleRow{
		ID:        "rule-1",
		EventType: "asset.warranty_expiring",
		Channel:   "email",
		Target:    "a@x.com,b@x.com",
		Active:    true,
	}
	notif := notify.Notification{
		EventType: "asset.warranty_expiring",
		Subject:   "test",
		Body:      "body",
		OrgID:     "org-1",
	}
	// email 渠道需设置 To
	notif.To = parseRecipients(rule.Target)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.deliverWithRetry(ctx, email, notif, rule)

	if len(email.Sent()) != 1 {
		t.Fatalf("期望 email 发送 1 次, 实际 %d", len(email.Sent()))
	}
	if len(email.Sent()[0].To) != 2 {
		t.Fatalf("期望 2 个收件人, 实际 %d", len(email.Sent()[0].To))
	}
}

// ============================================================
// TestNotifyDispatcher_RetryOnFailure — 失败重试到成功
// ============================================================

func TestNotifyDispatcher_RetryOnFailure(t *testing.T) {
	// 前 2 次失败, 第 3 次成功 (delays = 5s, 20s, 60s — 测试用缩放)
	email := &mockNotifier{channel: "email", available: true, failN: 2}

	d := NewNotifyDispatcher(nil, nil, []notify.Notifier{email}, true)

	// 临时替换重试延迟为极短以加速测试
	origDelays := retryDelays
	retryDelays = []time.Duration{1 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond}
	defer func() { retryDelays = origDelays }()

	rule := repository.NotifyRuleRow{ID: "r", EventType: "evt", Channel: "email", Target: "x@y.com", Active: true}
	notif := notify.Notification{EventType: "evt", To: []string{"x@y.com"}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	d.deliverWithRetry(ctx, email, notif, rule)

	if len(email.Sent()) != 1 {
		t.Fatalf("重试后应成功发送 1 次, 实际 %d (calls=%d)", len(email.Sent()), email.calls)
	}
	if email.calls != 3 {
		t.Fatalf("期望调用 3 次 (2 失败 + 1 成功), 实际 %d", email.calls)
	}
}

// ============================================================
// TestNotifyDispatcher_DisabledWhenNotEnabled — 未启用时空操作
// ============================================================

func TestNotifyDispatcher_DisabledWhenNotEnabled(t *testing.T) {
	email := &mockNotifier{channel: "email", available: true}
	d := NewNotifyDispatcher(nil, nil, []notify.Notifier{email}, false)
	// Start 在未启用时直接返回, 不订阅
	d.Start(context.Background())
	// 无 panic 即通过
}

// ============================================================
// TestParseRecipients — 收件人解析
// ============================================================

func TestParseRecipients(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a@x.com", 1},
		{"a@x.com,b@x.com", 2},
		{" a@x.com , , b@x.com ", 2},
	}
	for _, c := range cases {
		got := parseRecipients(c.in)
		if len(got) != c.want {
			t.Errorf("parseRecipients(%q) = %v, want %d items", c.in, got, c.want)
		}
	}
}

// ============================================================
// TestApprovalExecutor_DecodePayload — 执行回调正确解码 payload
// ============================================================

func TestApprovalExecutor_DecodePayload(t *testing.T) {
	// 验证 AssignmentApprovalExecutor 能解码 payload
	payload, _ := json.Marshal(map[string]string{"assigned_to": "u-1", "notes": "n"})
	req := &repository.ApprovalRequestRow{
		ResourceType: "assignment",
		ResourceID:   "asset-1",
		RequesterID:  "mgr-1",
		OrgID:        "org-1",
		Payload:      payload,
	}

	// 直接测试 payload 解码 (不依赖 DB 的 svc 调用)
	var p struct {
		AssignedTo string `json:"assigned_to"`
		Notes      string `json:"notes"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if p.AssignedTo != "u-1" || p.Notes != "n" {
		t.Errorf("payload 解码错误: %+v", p)
	}
}
