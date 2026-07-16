// Package webhook — Webhook 引擎与安全测试
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"testing"
	"time"
)

// ============================================================
// mockSecretStore — 模拟密钥存储
// ============================================================

type mockSecretStore struct {
	secrets map[string][]byte
}

func newMockSecretStore() *mockSecretStore {
	return &mockSecretStore{secrets: make(map[string][]byte)}
}

func (m *mockSecretStore) GetSecret(ctx context.Context, endpointID string) ([]byte, error) {
	secret, ok := m.secrets[endpointID]
	if !ok {
		return nil, &storeError{"secret not found"}
	}
	return secret, nil
}

func (m *mockSecretStore) SetSecret(endpointID string, secret []byte) {
	m.secrets[endpointID] = secret
}

type storeError struct{ msg string }

func (e *storeError) Error() string { return e.msg }

// ============================================================
// TestHMACSignature — 签名生成与验证
// ============================================================

func TestHMACSignature(t *testing.T) {
	secret := []byte("whsec_test_secret_key_32_bytes_long!")
	eventID := "evt_20240716_001"
	deliveredAt, _ := time.Parse(time.RFC3339, "2024-07-16T10:30:00Z")
	body := []byte(`{"event":"asset.created","data":{"id":"123"}}`)

	// 生成签名
	sig := SignPayload(secret, eventID, deliveredAt, body)

	// 验证签名格式
	if len(sig) < 8 || sig[:7] != "sha256=" {
		t.Errorf("invalid signature format: %s", sig)
	}

	// 独立验证签名正确性
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(eventID))
	mac.Write([]byte(deliveredAt.Format(time.RFC3339)))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if sig != expected {
		t.Errorf("signature mismatch:\n  got      %s\n  expected %s", sig, expected)
	}

	// 验证修改任何一个分量都会导致签名不同
	// 修改 eventID
	sig2 := SignPayload(secret, "evt_20240716_002", deliveredAt, body)
	if sig == sig2 {
		t.Error("different eventID should produce different signature")
	}

	// 修改 body
	sig3 := SignPayload(secret, eventID, deliveredAt, []byte(`{"modified":true}`))
	if sig == sig3 {
		t.Error("different body should produce different signature")
	}

	// 修改 secret
	sig4 := SignPayload([]byte("different_secret_key_here!!!!!!!"), eventID, deliveredAt, body)
	if sig == sig4 {
		t.Error("different secret should produce different signature")
	}

	// 修改时间
	later := deliveredAt.Add(1 * time.Second)
	sig5 := SignPayload(secret, eventID, later, body)
	if sig == sig5 {
		t.Error("different time should produce different signature")
	}
}

// ============================================================
// TestHMACSignatureDeterministic — 签名确定性
// ============================================================

func TestHMACSignatureDeterministic(t *testing.T) {
	secret := []byte("whsec_deterministic_test_key_here")
	eventID := "evt_det_001"
	deliveredAt := time.Now().Truncate(time.Second)
	body := []byte(`{"test":true}`)

	sig1 := SignPayload(secret, eventID, deliveredAt, body)
	sig2 := SignPayload(secret, eventID, deliveredAt, body)

	if sig1 != sig2 {
		t.Errorf("signature should be deterministic: %s != %s", sig1, sig2)
	}
}

// ============================================================
// TestReplayProtection — 重放攻击检测 (时间校验)
// ============================================================

func TestReplayProtection(t *testing.T) {
	maxAge := 5 * time.Minute
	skew := 30 * time.Second

	// 当前时间 — 应该有效
	now := time.Now()
	err := ValidateDeliveryTime(now, maxAge, skew)
	if err != nil {
		t.Errorf("current time should be valid: %v", err)
	}

	// 刚刚过去 1 分钟 — 应该有效
	oneMinAgo := now.Add(-1 * time.Minute)
	err = ValidateDeliveryTime(oneMinAgo, maxAge, skew)
	if err != nil {
		t.Errorf("1 minute ago should be valid: %v", err)
	}

	// 刚刚过去 4 分钟 45 秒 — 应该有效 (在 maxAge 内)
	almostExpired := now.Add(-4*time.Minute - 45*time.Second)
	err = ValidateDeliveryTime(almostExpired, maxAge, skew)
	if err != nil {
		t.Errorf("4m45s ago should be valid: %v", err)
	}

	// 过去 6 分钟 — 应该过期
	sixMinAgo := now.Add(-6 * time.Minute)
	err = ValidateDeliveryTime(sixMinAgo, maxAge, skew)
	if err == nil {
		t.Error("6 minutes ago should be expired")
	}

	// 过去 1 小时 — 应该过期
	oneHourAgo := now.Add(-1 * time.Hour)
	err = ValidateDeliveryTime(oneHourAgo, maxAge, skew)
	if err == nil {
		t.Error("1 hour ago should be expired")
	}

	// 未来时间 — 应拒绝
	future := now.Add(1 * time.Minute)
	err = ValidateDeliveryTime(future, maxAge, skew)
	if err == nil {
		t.Error("future time should be rejected")
	}

	// 稍微未来但在 skew 范围内 — 应允许
	slightFuture := now.Add(15 * time.Second)
	err = ValidateDeliveryTime(slightFuture, maxAge, skew)
	if err != nil {
		t.Errorf("slight future within skew should be valid: %v", err)
	}

	// 边界: 超过 maxAge+skew — 应过期
	exactMaxAge := now.Add(-(maxAge + skew + time.Nanosecond))
	err = ValidateDeliveryTime(exactMaxAge, maxAge, skew)
	if err == nil {
		t.Error("past maxAge+skew should be expired")
	}
}

// ============================================================
// TestSSRFProtection — 私有 IP 拒绝
// ============================================================

func TestSSRFProtection(t *testing.T) {
	privateIPs := []string{
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.0.1",
		"192.168.255.255",
		"127.0.0.1",
		"0.0.0.0",
		"169.254.1.1",
		"100.64.0.1",
		"224.0.0.1",
		"172.17.0.1",  // Docker
		"172.18.0.1",  // Docker
		"::1",         // IPv6 loopback
		"fe80::1",     // IPv6 link-local
		"fc00::1",     // IPv6 unique local
	}

	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34", // example.com
		"203.0.113.1",
		"198.51.100.1",
	}

	for _, ipStr := range privateIPs {
		t.Run("private/"+ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				t.Fatalf("invalid test IP: %s", ipStr)
			}
			if !IsPrivateIP(ip) {
				t.Errorf("expected %s to be private", ipStr)
			}
		})
	}

	for _, ipStr := range publicIPs {
		t.Run("public/"+ipStr, func(t *testing.T) {
			ip := net.ParseIP(ipStr)
			if ip == nil {
				t.Fatalf("invalid test IP: %s", ipStr)
			}
			if IsPrivateIP(ip) {
				t.Errorf("expected %s to be public", ipStr)
			}
		})
	}
}

// ============================================================
// TestSSRFProtectionCIDRBoundaries — CIDR 边界测试
// ============================================================

func TestSSRFProtectionCIDRBoundaries(t *testing.T) {
	// 10.0.0.0/8 边界
	if !IsPrivateIP(net.ParseIP("10.0.0.0")) {
		t.Error("10.0.0.0 should be private")
	}
	if !IsPrivateIP(net.ParseIP("10.255.255.255")) {
		t.Error("10.255.255.255 should be private")
	}
	if IsPrivateIP(net.ParseIP("11.0.0.0")) {
		t.Error("11.0.0.0 should be public")
	}
	if IsPrivateIP(net.ParseIP("9.255.255.255")) {
		t.Error("9.255.255.255 should be public")
	}

	// 192.168.0.0/16 边界
	if !IsPrivateIP(net.ParseIP("192.168.0.0")) {
		t.Error("192.168.0.0 should be private")
	}
	if !IsPrivateIP(net.ParseIP("192.168.255.255")) {
		t.Error("192.168.255.255 should be private")
	}
	if IsPrivateIP(net.ParseIP("192.169.0.0")) {
		t.Error("192.169.0.0 should be public")
	}
	if IsPrivateIP(net.ParseIP("192.167.255.255")) {
		t.Error("192.167.255.255 should be public")
	}

	// 172.16.0.0/12 边界
	if !IsPrivateIP(net.ParseIP("172.16.0.0")) {
		t.Error("172.16.0.0 should be private")
	}
	if !IsPrivateIP(net.ParseIP("172.31.255.255")) {
		t.Error("172.31.255.255 should be private")
	}
	if IsPrivateIP(net.ParseIP("172.15.255.255")) {
		t.Error("172.15.255.255 should be public")
	}
	if IsPrivateIP(net.ParseIP("172.32.0.0")) {
		t.Error("172.32.0.0 should be public")
	}
}

// ============================================================
// TestWebhookURLValidation — URL 校验
// ============================================================

func TestWebhookURLValidation(t *testing.T) {
	allowedHosts := AllowedHosts()

	validURLs := []string{
		"https://hooks.slack.com/services/T00/B00/xxx",
		"https://api.github.com/repos/owner/repo",
	}
	invalidURLs := []string{
		"http://hooks.slack.com/services/T00/B00/xxx", // not https
		"https://evil.com/webhook",                     // not in allowlist
		"https://hooks.slack.com.evil.com/webhook",     // subdomain bypass attempt
		"",                                              // empty
		"not-a-url",                                     // invalid
	}

	for _, url := range validURLs {
		err := ValidateWebhookURL(url, allowedHosts)
		if err != nil {
			t.Errorf("expected valid URL %q: %v", url, err)
		}
	}

	for _, url := range invalidURLs {
		err := ValidateWebhookURL(url, allowedHosts)
		if err == nil {
			t.Errorf("expected invalid URL %q", url)
		}
	}
}

// ============================================================
// TestAllowedHosts — 白名单
// ============================================================

func TestAllowedHosts(t *testing.T) {
	hosts := AllowedHosts()
	if len(hosts) < 2 {
		t.Errorf("expected at least 2 allowed hosts, got %d", len(hosts))
	}
	hasSlack := false
	hasGitHub := false
	for _, h := range hosts {
		if h == "hooks.slack.com" {
			hasSlack = true
		}
		if h == "api.github.com" {
			hasGitHub = true
		}
	}
	if !hasSlack {
		t.Error("hooks.slack.com should be in allowed hosts")
	}
	if !hasGitHub {
		t.Error("api.github.com should be in allowed hosts")
	}
}
