package notify

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
)

// EmailNotifier SMTP 邮件通知器
// 使用 stdlib net/smtp, 不引入额外依赖。
type EmailNotifier struct {
	cfg config.SMTPConfig
}

// NewEmailNotifier 创建邮件通知器; 凭据从 config 注入。
func NewEmailNotifier(cfg config.SMTPConfig) *EmailNotifier {
	return &EmailNotifier{cfg: cfg}
}

func (e *EmailNotifier) Channel() string { return "email" }

func (e *EmailNotifier) Available() bool {
	return e.cfg.Host != "" && e.cfg.From != "" && len(e.cfg.User) > 0
}

// Send 发送邮件 (PLAIN auth, 兼容内网 SMTP)
// 凭据不写入错误信息 (避免密码泄漏到日志/审计)。
func (e *EmailNotifier) Send(ctx context.Context, n Notification) error {
	if !e.Available() {
		return fmt.Errorf("email notifier: not configured")
	}
	if len(n.To) == 0 {
		return fmt.Errorf("email notifier: no recipients")
	}

	addr := fmt.Sprintf("%s:%d", e.cfg.Host, e.cfg.Port)
	auth := smtp.PlainAuth("", e.cfg.User, e.cfg.Password, e.cfg.Host)

	// RFC 5322 简单消息体
	msg := buildRFC5322(e.cfg.From, n.To, n.Subject, n.Body)

	// smtp.SendMail 不接受 context, 但调用通常较快; ctx 仅用于取消信号检查
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := smtp.SendMail(addr, auth, e.cfg.From, n.To, msg); err != nil {
		// 不回显原始 err 中的凭据片段 (PlainAuth 不会带密码入 error, 但保守起见剥离)
		return fmt.Errorf("email notifier: send failed")
	}
	return nil
}

// buildRFC5322 构造简单的 RFC 5322 邮件字节
func buildRFC5322(from string, to []string, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
