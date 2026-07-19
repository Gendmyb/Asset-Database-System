package notify

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/webhook"
)

// ssrfSafeClient 返回带 SSRF 防护的 HTTP 客户端 (复用 webhook.IsPrivateIP)
// 拒绝连接解析到私有网段/元数据地址的目标, 并防 DNS rebinding。
func ssrfSafeClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				if ip := net.ParseIP(host); ip != nil && webhook.IsPrivateIP(ip) {
					return nil, fmt.Errorf("notify: ssrf private IP denied: %s", ip)
				}
				dialer := &net.Dialer{Timeout: 5 * time.Second}
				conn, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				if tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr); ok && webhook.IsPrivateIP(tcpAddr.IP) {
					conn.Close()
					return nil, fmt.Errorf("notify: ssrf dns rebinding denied: %s", tcpAddr.IP)
				}
				return conn, nil
			},
		},
	}
}

// postRobotJSON 向机器人 webhook 发送 JSON 载荷 (SSRF 防护)
// 仅允许 https — 强制校验 scheme, 防止 http 明文泄露签名/凭据。
func postRobotJSON(ctx context.Context, client *http.Client, url string, body []byte) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("notify: robot webhook must use https, got %q", url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("notify: request failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notify: robot webhook returned status %d", resp.StatusCode)
	}
	return nil
}
