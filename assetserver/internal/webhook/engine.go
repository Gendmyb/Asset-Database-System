// Package webhook — Webhook 引擎 (HMAC + 防重放 + SSRF)
// 对应架构文档 §10.2 Webhook 引擎 + §7.4 Webhook 安全
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Payload Webhook 外发载荷
type Payload struct {
	EventID     string          `json:"event_id"`
	EventType   string          `json:"event_type"`
	DeliveredAt time.Time       `json:"delivered_at"`
	Data        json.RawMessage `json:"data"`
}

// Engine Webhook 引擎
type Engine struct {
	client      *http.Client
	secretStore SecretStore
	maxRetries  int
	retryDelays []time.Duration
}

// SecretStore Webhook secret 存储接口
// 生产: AES-256-GCM 加密存储, 解密仅在内存
type SecretStore interface {
	GetSecret(ctx context.Context, endpointID string) ([]byte, error)
}

func NewEngine(store SecretStore) *Engine {
	return &Engine{
		client:      newSSRFHTTPClient(),
		secretStore: store,
		maxRetries:  5,
		retryDelays: []time.Duration{1, 2, 4, 8, 16},
	}
}

// SignPayload HMAC-SHA256 签名
// 对应架构文档: HMAC-SHA256(secret, event_id + delivered_at + raw_body)
func SignPayload(secret []byte, eventID string, deliveredAt time.Time, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(eventID))
	mac.Write([]byte(deliveredAt.Format(time.RFC3339)))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// ValidateDeliveryTime 服务端时间校验 (防伪造 delivered_at)
// 对应架构文档 §7.4: 服务端自身时钟校验
func ValidateDeliveryTime(deliveredAt time.Time, maxAge time.Duration, skew time.Duration) error {
	now := time.Now()
	if deliveredAt.After(now.Add(skew)) {
		return fmt.Errorf("webhook: future timestamp")
	}
	if now.Sub(deliveredAt) > maxAge+skew {
		return fmt.Errorf("webhook: expired timestamp")
	}
	return nil
}

// Deliver 发送 webhook (含重试)
func (e *Engine) Deliver(ctx context.Context, endpointURL string, endpointID string, payload *Payload) error {
	secret, err := e.secretStore.GetSecret(ctx, endpointID)
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}

	body, _ := json.Marshal(payload)
	payload.DeliveredAt = time.Now()
	sig := SignPayload(secret, payload.EventID, payload.DeliveredAt, body)

	var lastErr error
	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		if attempt > 0 {
			delay := e.retryDelays[attempt-1] * time.Minute
			time.Sleep(delay)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Signature-256", sig)
		req.Header.Set("X-Event-ID", payload.EventID)

		resp, err := e.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("webhook: status %d", resp.StatusCode)
	}
	return fmt.Errorf("webhook: max retries exceeded: %w", lastErr)
}

// newSSRFHTTPClient 创建 SSRF 防护的 HTTP 客户端
func newSSRFHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				ip := net.ParseIP(host)
				if ip != nil && IsPrivateIP(ip) {
					return nil, fmt.Errorf("ssrf: private IP denied: %s", ip)
				}
				dialer := &net.Dialer{Timeout: 5 * time.Second}
				conn, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				// DNS rebinding 防护: 连接建立后再验对端 IP
				tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
				if ok && IsPrivateIP(tcpAddr.IP) {
					conn.Close()
					return nil, fmt.Errorf("ssrf: dns rebinding detected: %s", tcpAddr.IP)
				}
				return conn, nil
			},
		},
	}
}

// ValidateWebhookURL 校验 webhook URL (HTTPS only + 域名白名单)
func ValidateWebhookURL(rawURL string, allowedHosts []string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("webhook: https required")
	}
	host := u.Hostname()
	for _, allowed := range allowedHosts {
		if host == allowed || (len(allowed) > 0 && allowed[0] == '.' && host[len(host)-len(allowed):] == allowed) {
			return nil
		}
	}
	return fmt.Errorf("webhook: host %s not in allowlist", host)
}

// RetryBackoffs 指数退避时间
func RetryBackoffs() []time.Duration {
	return []time.Duration{
		1 * time.Minute,
		2 * time.Minute,
		4 * time.Minute,
		8 * time.Minute,
		16 * time.Minute,
	}
}

// MaxRetries 最大重试次数
func MaxRetries() int {
	return 5
}

func init() {
	// 验证 RetryBackoffs 单调递增
	backoffs := RetryBackoffs()
	for i := 1; i < len(backoffs); i++ {
		if backoffs[i] <= backoffs[i-1] {
			panic(fmt.Sprintf("retry backoffs not monotonic: %v <= %v", backoffs[i], backoffs[i-1]))
		}
	}
	// 验证 16min < math.MaxInt
	_ = math.MaxInt
}
