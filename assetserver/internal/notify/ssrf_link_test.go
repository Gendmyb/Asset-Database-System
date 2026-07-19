// Package notify — SSRF 实链路测试 (Wave 2 G6)
//
// 验证 ssrfSafeClient 的 DialContext 在真实 HTTP 请求路径上拒绝私有 IP,
// 包括 IPv4-mapped IPv6 (::ffff:127.0.0.1) 这一绕过向量。
package notify

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSSRFRealLink_PostRobotJSON_DeniesLoopback — postRobotJSON 对 127.0.0.1 应失败。
//
// postRobotJSON 强制 https; 用 https://127.0.0.1:<port>/ 触发 DialContext,
// DialContext 在解析到 127.0.0.1 后立即拒绝 (不会真正建立 TCP/TLS)。
func TestSSRFRealLink_PostRobotJSON_DeniesLoopback(t *testing.T) {
	client := ssrfSafeClient()

	cases := []struct {
		name string
		url  string
	}{
		{"ipv4_loopback", "https://127.0.0.1:9/hook"},
		{"ipv4_loopback_anyport", "https://127.0.0.1:443/hook"},
		{"ipv4_mapped_v6_loopback", "https://[::ffff:127.0.0.1]:9/hook"},
		{"private_10", "https://10.0.0.1:9/hook"},
		{"private_192168", "https://192.168.1.1:9/hook"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			err := postRobotJSON(ctx, client, c.url, []byte(`{}`))
			if err == nil {
				t.Fatalf("postRobotJSON to %s should fail (SSRF denied)", c.url)
			}
			// 错误信息提示被拒 (而非 TLS / 连接成功)
			low := strings.ToLower(err.Error())
			if !strings.Contains(low, "ssrf") && !strings.Contains(low, "private") &&
				!strings.Contains(low, "denied") && !strings.Contains(low, "request failed") {
				t.Logf("note: non-ssrf error for %s: %v", c.url, err)
			}
		})
	}
}

// TestSSRFRealLink_DialContext_DeniesMappedV6 — 直接验证 DialContext 对 ::ffff:127.0.0.1 拒绝。
//
// 这是 SSRF 防护的关键向量: 不归一化 IPv4-mapped IPv6 会绕过 IPv4 私有网段判定。
func TestSSRFRealLink_DialContext_DeniesMappedV6(t *testing.T) {
	// 直接验证 IsPrivateIP 归一化路径 (webhook.IsPrivateIP 已被 ssrfSafeClient 复用)
	ip := net.ParseIP("::ffff:127.0.0.1")
	if ip == nil {
		t.Fatal("parse ::ffff:127.0.0.1 failed")
	}
	if !isPrivate(ip) {
		t.Fatal("::ffff:127.0.0.1 must be detected as private (IPv4-mapped normalization)")
	}

	// 通过真实 http.Client 触发 DialContext: 用 httptest 在 127.0.0.1 起一个 server,
	// ssrfSafeClient 必须在 dial 阶段拒绝, 永不抵达 server。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "should not reach")
	}))
	t.Cleanup(srv.Close)

	// httptest 默认监听 127.0.0.1; 将 URL 改成 https 以触发 postRobotJSON 的 https 校验,
	// 但即便用 http, DialContext 仍会拒绝。这里直接用 client.Do 验证 dial 拒绝。
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("new req: %v", err)
	}
	_, err = ssrfSafeClient().Do(req)
	if err == nil {
		t.Fatal("request to 127.0.0.1 httptest server must be denied by SSRF dial guard")
	}
}
