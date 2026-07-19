// Package notify — 原生通知渠道 (Wave 2 G6)
//
// 职责:
//   - 定义统一 Notifier 接口与 Notification 数据模型
//   - 提供邮件 (SMTP) 与机器人 webhook (钉钉/企微/飞书) 渠道实现
//   - Dispatcher 订阅事件总线, 按 notify_rules 分发, 失败重试 + 投递记录
//
// 安全: 机器人 webhook 复用 internal/webhook 的 SSRF 防护 (禁止发往内网/元数据地址);
// 凭据 (SMTP 密码) 仅从 config 注入, 不入日志/审计。
package notify

import "context"

// Notification 通知内容 (渠道无关)
type Notification struct {
	// EventType 触发此通知的事件类型 (如 "asset.warranty_expiring")
	EventType string
	// To 收件人; email 为地址列表, 机器人渠道忽略 (用全局 webhook)
	To []string
	// Subject 主题 (email 使用; 机器人渠道合并到 body)
	Subject string
	// Body 正文 (纯文本)
	Body string
	// OrgID 所属组织 (用于审计/隔离)
	OrgID string
}

// Notifier 通知渠道接口
type Notifier interface {
	// Send 投递一条通知; 失败返回 error 供 dispatcher 记录/重试
	Send(ctx context.Context, n Notification) error
	// Channel 渠道标识 (email/dingtalk/wecom/feishu)
	Channel() string
	// Available 该渠道是否已配置可用 (凭据/URL 齐全)
	Available() bool
}
