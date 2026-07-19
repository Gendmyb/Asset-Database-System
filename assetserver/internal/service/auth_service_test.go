// Package service — 认证服务集成测试 (Wave 1 G1)
//
// 覆盖 LDAP 登录回退时序:
//   - 本地 admin 登录成功 (source=local + bcrypt 校验)
//   - 即使注入了 LDAP 认证器, 本地用户命中时 LDAP 路径不被触发 (本地优先)
//   - AD 未配置 (ldap==nil) 时, 未知用户登录返回统一错误, 不 panic
//
// 这是一个 DB-backed 集成测试: 需要可用的 PostgreSQL。
// 通过 WAVE1_PG_TEST_DSN 环境变量指定 DSN; 未设置或连不通时 Skip。
package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultTestDSN 与仓库部署手册中的开发库一致; 可被 WAVE1_PG_TEST_DSN 覆盖。
const defaultTestDSN = "postgres://app_user:app_pass@localhost:5432/assetdb?sslmode=disable&search_path=assets"

// testPool 尝试连接 PG; 不可用则 t.Skip。
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("WAVE1_PG_TEST_DSN")
	if dsn == "" {
		dsn = defaultTestDSN
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip: pg connect failed: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skip: pg ping failed: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// recordingLDAP 记录 Authenticate 调用; 若被调用则标记 called=true。
type recordingLDAP struct {
	called bool
}

func (l *recordingLDAP) Authenticate(ctx context.Context, username, password string) (*LDAPAuthResult, error) {
	l.called = true
	return nil, errors.New("ldap should not be reached")
}

func (l *recordingLDAP) EnsureUserRow(ctx context.Context, r *LDAPAuthResult, defaultOrgID string) (string, string, string, error) {
	return "", "", "", errors.New("ensure should not be reached")
}

// TestLogin_LocalAdminSucceeds_LDAPNotTriggered 本地 admin 登录成功且 LDAP 不被触发。
// 前置: DB 中存在 source=local 的 admin 账号 (部署 seed)。
func TestLogin_LocalAdminSucceeds_LDAPNotTriggered(t *testing.T) {
	pool := testPool(t)
	km, err := crypto.NewKeyManager("")
	if err != nil {
		t.Fatalf("NewKeyManager: %v", err)
	}
	svc := NewAuthService(pool, km)

	// 注入一个会"自爆"的 LDAP 认证器: 一旦被调用即失败。
	rec := &recordingLDAP{}
	svc.SetLDAPAuthenticator(rec)

	// admin/admin123 是 seed 账号 (source=local, status=active)
	res, err := svc.Login(context.Background(), "admin", "admin123")
	if err != nil {
		t.Fatalf("local admin login: %v", err)
	}
	if res == nil || res.AccessToken == "" {
		t.Fatal("empty access token")
	}
	if res.User.Username != "admin" {
		t.Errorf("username = %q, want admin", res.User.Username)
	}
	if rec.called {
		t.Fatal("LDAP Authenticate was triggered for local user (local-first violated)")
	}
}

// TestLogin_NoLDAP_UnknownUserReturnsUnifiedError AD 未配置时未知用户返回统一错误。
func TestLogin_NoLDAP_UnknownUserReturnsUnifiedError(t *testing.T) {
	pool := testPool(t)
	km, err := crypto.NewKeyManager("")
	if err != nil {
		t.Fatalf("NewKeyManager: %v", err)
	}
	svc := NewAuthService(pool, km)
	// 不注入 LDAP -> s.ldap == nil (AD 未配置)

	_, err = svc.Login(context.Background(), "definitely_no_such_user_xyz", "whatever")
	if err == nil {
		t.Fatal("expected error for unknown user, got nil")
	}
	// 统一错误不区分用户是否存在
	if !containsSubstr(err.Error(), "用户名或密码错误") {
		t.Errorf("err = %q, want contains 用户名或密码错误", err.Error())
	}
}

// TestLogin_LocalWrongPassword_LDAPFallbackNotConfigured 本地密码错且 LDAP 未配置 -> 统一错误。
// 保证 AD 未配置时不会因走到 LDAP 兜底而 panic。
func TestLogin_LocalWrongPassword_LDAPFallbackNotConfigured(t *testing.T) {
	pool := testPool(t)
	km, err := crypto.NewKeyManager("")
	if err != nil {
		t.Fatalf("NewKeyManager: %v", err)
	}
	svc := NewAuthService(pool, km)
	// 不注入 LDAP

	_, err = svc.Login(context.Background(), "admin", "this-is-wrong-password")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
	if !containsSubstr(err.Error(), "用户名或密码错误") {
		t.Errorf("err = %q, want contains 用户名或密码错误", err.Error())
	}
}

func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
