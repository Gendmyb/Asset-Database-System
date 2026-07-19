// Package ldap — 同步服务单测 (mock DirectoryClient, 不依赖真实 AD)
package ldap

import (
	"context"
	"testing"
)

// mockClient 内存模拟 LDAP 目录
type mockClient struct {
	users    []DirUser
	bindFail map[string]bool // username -> bind 应失败
}

func (m *mockClient) SearchUsers(ctx context.Context) ([]DirUser, error) {
	return m.users, nil
}

// SearchOne 模拟单用户精确查询: 按 username 在内存目录中匹配
func (m *mockClient) SearchOne(ctx context.Context, username string) (*DirUser, error) {
	for _, u := range m.users {
		if u.Username == username {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockClient) Bind(ctx context.Context, userDN, password string) error {
	if m.bindFail == nil {
		return nil
	}
	for _, u := range m.users {
		if u.DN == userDN {
			if m.bindFail[u.Username] {
				return errInvalidCreds
			}
			return nil
		}
	}
	return errInvalidCreds
}

func (m *mockClient) Close() error { return nil }

// 简单 sentinel 错误 (避免引入额外类型)
type sentinelErr string

func (s sentinelErr) Error() string { return string(s) }

const errInvalidCreds = sentinelErr("invalid credentials")

// 测 RunSyncOnce 不连真实 DB 时返回错误 (依赖 pool); 此用例验证 client 层契约
func TestRunSyncOnce_NoPool(t *testing.T) {
	s := NewSyncService(&mockClient{users: []DirUser{{Username: "jdoe"}}}, nil)
	_, err := s.RunSyncOnce(context.Background(), "actor", "org")
	if err == nil {
		t.Fatal("expected error when pool is nil")
	}
}

// 测 sanitizeLTree
func TestSanitizeLTree(t *testing.T) {
	cases := map[string]string{
		// 中文按 UTF-8 字节数替换为下划线 (技术部 = 3 字符 × 3 字节 = 9 下划线)
		"技术部":        "_________",
		"Engineering": "Engineering",
		"R&D":         "R_D",
		"":            "dept",
		"Org.1":       "Org_1",
	}
	for in, want := range cases {
		got := sanitizeLTree(in)
		if got != want {
			t.Errorf("sanitizeLTree(%q) = %q, want %q", in, got, want)
		}
	}
}

// 测 mockClient.SearchUsers / Bind 契约
func TestMockClient(t *testing.T) {
	m := &mockClient{
		users: []DirUser{
			{Username: "alice", DN: "cn=alice,dc=corp,dc=local"},
			{Username: "bob", DN: "cn=bob,dc=corp,dc=local"},
		},
		bindFail: map[string]bool{"bob": true},
	}
	ctx := context.Background()
	users, err := m.SearchUsers(ctx)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if err := m.Bind(ctx, "cn=alice,dc=corp,dc=local", "x"); err != nil {
		t.Errorf("alice bind should succeed: %v", err)
	}
	if err := m.Bind(ctx, "cn=bob,dc=corp,dc=local", "x"); err == nil {
		t.Errorf("bob bind should fail")
	}
}
