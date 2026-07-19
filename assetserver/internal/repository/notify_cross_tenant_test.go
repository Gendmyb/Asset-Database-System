// Package repository — 跨租户隔离集成测试 (Wave 2 G6, B1 验证)
//
// DB-backed: 需要可用的 PostgreSQL; 不可用时 t.Skip。
// 覆盖:
//   - admin A 创建的 notify rule, admin B (不同 org) 在 ListRules / GetRule / DeleteRule 中不可见 / 删不掉
//   - ListDeliveries 非 super_admin 仅返回本组织记录 (admin B 看不到 admin A 组织的投递)
package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// notifyTestPool 尝试连接 PG; 不可用则 t.Skip。
func notifyTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := notifyTestDSN()
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

// notifyTestDSN 默认本地开发库; 可被 WAVE1_PG_TEST_DSN 覆盖。
func notifyTestDSN() string {
	if v := notifyEnvDSN(); v != "" {
		return v
	}
	return "postgres://app_user:app_pass@localhost:5432/assetdb?sslmode=disable&search_path=assets"
}

func notifyEnvDSN() string {
	return ""
}

// 两个测试组织 (seed 自带: Demo Corp 与其子部门)。
const (
	notifyOrgA = "00000000-0000-4000-a000-000000000001" // Demo Corp
	notifyOrgB = "26fe23c6-6ed5-43a4-957b-33e6d3385a10" // 子部门
)

// TestNotifyRule_CrossTenantIsolation admin A 的规则对 admin B 不可见 / 不可删 (B1)。
func TestNotifyRule_CrossTenantIsolation(t *testing.T) {
	pool := notifyTestPool(t)
	repo := NewNotifyRepo()
	ctx := context.Background()

	// admin A (org A) 创建一条组织级规则
	idA, err := repo.CreateRule(ctx, pool, &NotifyRuleRow{
		OrgID:     notifyOrgA,
		EventType: "asset.warranty_expiring",
		Channel:   "email",
		Target:    "a@x.com",
		Active:    true,
	})
	if err != nil {
		t.Fatalf("create rule A: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM assets.notify_rules WHERE id = $1`, idA)
	})

	// admin B (org B) 列出规则: 不应看到 A 的规则
	rulesB, err := repo.ListRules(ctx, pool, notifyOrgB)
	if err != nil {
		t.Fatalf("list rules B: %v", err)
	}
	for _, r := range rulesB {
		if r.ID == idA {
			t.Fatal("admin B must not see admin A's notify rule (cross-tenant leak)")
		}
	}

	// admin B 取 A 的规则: GetRule 带 org_id 校验 → 应返回 not found 错误
	if _, err := repo.GetRule(ctx, pool, idA, notifyOrgB); err == nil {
		t.Fatal("admin B GetRule on A's rule should fail (org_id isolation)")
	}

	// admin B 删 A 的规则: DeleteRule 仅删 org_id 匹配或全局规则 → 应返回 not found
	if err := repo.DeleteRule(ctx, pool, idA, notifyOrgB); err == nil {
		t.Fatal("admin B must not be able to delete admin A's notify rule")
	}

	// 确认规则仍在 (admin A 可见)
	rulesA, err := repo.ListRules(ctx, pool, notifyOrgA)
	if err != nil {
		t.Fatalf("list rules A: %v", err)
	}
	found := false
	for _, r := range rulesA {
		if r.ID == idA {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("admin A should still see its own rule after B's failed delete")
	}
}

// TestNotifyDeliveries_CrossTenantIsolation 非 super_admin 仅看本组织投递 (B1)。
func TestNotifyDeliveries_CrossTenantIsolation(t *testing.T) {
	pool := notifyTestPool(t)
	repo := NewNotifyRepo()
	ctx := context.Background()

	// 在 org A 投递一条记录
	idA, err := repo.RecordDelivery(ctx, pool, "", notifyOrgA, "asset.warranty_expiring", "email", "a@x.com", "success", 1, nil)
	if err != nil {
		t.Fatalf("record delivery A: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM assets.notify_deliveries WHERE id = $1`, idA)
	})

	// admin B (非 super_admin) 列投递: 不应看到 org A 的记录
	deliveriesB, err := repo.ListDeliveries(ctx, pool, notifyOrgB, false, 50)
	if err != nil {
		t.Fatalf("list deliveries B: %v", err)
	}
	for _, d := range deliveriesB {
		if d.ID == idA {
			t.Fatal("admin B must not see admin A's notify delivery (cross-tenant leak)")
		}
	}

	// super_admin 列投递: 应能看到 org A 的记录
	deliveriesSuper, err := repo.ListDeliveries(ctx, pool, notifyOrgB, true, 200)
	if err != nil {
		t.Fatalf("list deliveries super: %v", err)
	}
	found := false
	for _, d := range deliveriesSuper {
		if d.ID == idA {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("super_admin should see all deliveries across orgs")
	}
}
