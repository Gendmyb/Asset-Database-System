// Package service — 通知 dispatcher (Wave 2 G6)
//
// 订阅事件总线, 按 assets.notify_rules 将事件分发到配置的渠道
// (email/dingtalk/wecom/feishu)。失败重试 (指数退避, 最多 3 次),
// 投递结果写入 assets.notify_deliveries; 失败写审计摘要。
package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/notify"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotifyDispatcher 通知分发器
type NotifyDispatcher struct {
	pool      *pgxpool.Pool
	repo      *repository.NotifyRepo
	notifiers map[string]notify.Notifier // channel → notifier
	enabled   bool
}

// NewNotifyDispatcher 创建分发器; notifiers 为空或 enable=false 时为空操作
func NewNotifyDispatcher(pool *pgxpool.Pool, repo *repository.NotifyRepo, notifiers []notify.Notifier, enabled bool) *NotifyDispatcher {
	m := make(map[string]notify.Notifier, len(notifiers))
	for _, n := range notifiers {
		if n.Available() {
			m[n.Channel()] = n
		}
	}
	return &NotifyDispatcher{pool: pool, repo: repo, notifiers: m, enabled: enabled}
}

// notifyEventTypes 需要订阅的事件类型 (覆盖 Wave1 scheduler 与业务事件)
var notifyEventTypes = []string{
	"asset.warranty_expiring",
	"asset.warranty_expired",
	"assignment.overdue",
	event.EventAssetAssigned,
	event.EventAssetReleased,
	event.EventAssetTransferred,
	event.EventAssetBorrowed,
	"asset.retired",
	"maintenance.created",
}

// Start 订阅事件总线并分发; ctx 取消时优雅退出
func (d *NotifyDispatcher) Start(ctx context.Context) {
	if !d.enabled || len(d.notifiers) == 0 {
		slog.Info("notify dispatcher: disabled (not enabled or no channel configured)")
		return
	}
	slog.Info("notify dispatcher: started", "channels", d.channelList())

	for _, et := range notifyEventTypes {
		ch, err := event.DefaultBus.Subscribe(ctx, et)
		if err != nil {
			slog.Error("notify dispatcher: subscribe failed", "event", et, "error", err)
			continue
		}
		go d.consume(ctx, et, ch)
	}
}

func (d *NotifyDispatcher) channelList() []string {
	out := make([]string, 0, len(d.notifiers))
	for k := range d.notifiers {
		out = append(out, k)
	}
	return out
}

func (d *NotifyDispatcher) consume(ctx context.Context, eventType string, ch <-chan *event.Event) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			d.handleEvent(ctx, evt)
		}
	}
}

func (d *NotifyDispatcher) handleEvent(ctx context.Context, evt *event.Event) {
	rules, err := d.repo.ListActiveRules(ctx, d.pool, evt.OrgID, evt.Type)
	if err != nil {
		slog.Error("notify dispatcher: list rules failed", "event", evt.Type, "error", err)
		return
	}
	if len(rules) == 0 {
		return
	}

	n := notify.Notification{
		EventType: evt.Type,
		OrgID:     evt.OrgID,
		Subject:   formatSubject(evt),
		Body:      formatBody(evt),
	}

	for _, rule := range rules {
		notifier, ok := d.notifiers[rule.Channel]
		if !ok {
			continue // 渠道未配置, 跳过
		}
		// email 渠道使用 rule.Target 作为收件人; 机器人渠道忽略 target
		nn := n
		if rule.Channel == "email" {
			nn.To = parseRecipients(rule.Target)
			if len(nn.To) == 0 {
				continue
			}
		}
		rule := rule
		go d.deliverWithRetry(ctx, notifier, nn, rule)
	}
}

// retryDelays 通知投递失败重试延迟 (指数退避: 5s, 20s, 60s)
// 包级变量便于测试覆写加速。
var retryDelays = []time.Duration{5 * time.Second, 20 * time.Second, 60 * time.Second}

// deliverWithRetry 指数退避重试, 最多发送 3 次 (首次 + 2 次重试, 重试间隔 5s/20s;
// retryDelays 末项保留为扩展位, 末次失败后不再 sleep)。投递结果写入 notify_deliveries;
// 失败写审计摘要。
func (d *NotifyDispatcher) deliverWithRetry(ctx context.Context, n notify.Notifier, notif notify.Notification, rule repository.NotifyRuleRow) {
	delays := retryDelays
	var lastErr error
	attempts := 0
	for attempt := 0; attempt < len(delays); attempt++ {
		attempts = attempt + 1
		if err := n.Send(ctx, notif); err != nil {
			lastErr = err
			if attempt < len(delays)-1 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delays[attempt]):
				}
				continue
			}
			break
		}
		lastErr = nil
		break
	}

	status := "success"
	var errStr *string
	if lastErr != nil {
		status = "failed"
		e := lastErr.Error()
		errStr = &e
		slog.Warn("notify dispatcher: delivery failed",
			"channel", rule.Channel, "event", notif.EventType, "error", lastErr)
		// 写审计摘要 (独立事务, 不含凭据)
		d.writeFailureAudit(ctx, notif, rule, lastErr)
	}

	target := rule.Target
	if d.pool != nil && d.repo != nil {
		if _, err := d.repo.RecordDelivery(ctx, d.pool, rule.ID, rule.OrgID, notif.EventType, rule.Channel, target, status, attempts, errStr); err != nil {
			slog.Error("notify dispatcher: record delivery failed", "error", err)
		}
	}
}

// writeFailureAudit 投递失败写审计摘要 (独立事务, 参照 ldap writeAuditSeparately 模式)
func (d *NotifyDispatcher) writeFailureAudit(ctx context.Context, notif notify.Notification, rule repository.NotifyRuleRow, err error) {
	if d.pool == nil {
		return
	}
	summary, _ := json.Marshal(map[string]interface{}{
		"event":   notif.EventType,
		"channel": rule.Channel,
		"error":   err.Error(),
	})
	// 截断 ≤3500 字节, 留余量给 Entry 包装字段 (audit_log CHECK 4096)
	if len(summary) > 3500 {
		summary = []byte(`{"truncated":true}`)
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "notify_deliveries",
		RecordID:  rule.ID,
		Action:    "notify_failed",
		OrgID:     notif.OrgID,
		NewValues: summary,
	}); err != nil {
		slog.Warn("notify dispatcher: write audit failed", "error", err)
		return
	}
	_ = tx.Commit(ctx)
}

// formatSubject 根据事件类型生成通知主题
func formatSubject(evt *event.Event) string {
	switch evt.Type {
	case "asset.warranty_expiring":
		return "资产质保即将到期提醒"
	case "asset.warranty_expired":
		return "资产质保已过期提醒"
	case "assignment.overdue":
		return "资产领用逾期未还提醒"
	case event.EventAssetAssigned:
		return "资产领用通知"
	case event.EventAssetReleased:
		return "资产归还通知"
	case event.EventAssetTransferred:
		return "资产转移通知"
	case event.EventAssetBorrowed:
		return "资产借用通知"
	case "asset.retired":
		return "资产报废通知"
	case "maintenance.created":
		return "维修工单创建通知"
	default:
		return "资产系统通知"
	}
}

// formatBody 生成通知正文 (含事件关键信息)
func formatBody(evt *event.Event) string {
	var sb strings.Builder
	sb.WriteString("事件类型: " + evt.Type + "\n")
	if evt.AssetID != "" {
		sb.WriteString("资产 ID: " + evt.AssetID + "\n")
	}
	if evt.OrgID != "" {
		sb.WriteString("组织 ID: " + evt.OrgID + "\n")
	}
	if evt.UserID != "" {
		sb.WriteString("操作用户: " + evt.UserID + "\n")
	}
	sb.WriteString("时间: " + evt.Timestamp.Format("2006-01-02 15:04:05"))
	if len(evt.Data) > 0 {
		sb.WriteString("\n附加数据: " + string(evt.Data))
	}
	return sb.String()
}

// parseRecipients 解析逗号分隔的收件人列表
func parseRecipients(target string) []string {
	if target == "" {
		return nil
	}
	parts := strings.Split(target, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
