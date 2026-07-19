// Package service — 审批状态机集成测试 (Wave 2 G7, B1/B2/A2 影子测试)
//
// DB-backed: 需要可用的 PostgreSQL; 不可用时 t.Skip。
// 覆盖:
//   - 真实审批状态机: pending → approved/rejected, 终态再操作拒绝 (A2 影子)
//   - 并发 approve: 两 goroutine 同时 approve 同一 pending → 只有一个成功, executor 只执行一次
//   - 审批执行失败: executor 返回错误 → 审批 approved + 审计 exec_error, 业务不发生
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// approvalTestOrg 测试用的组织 ID (Demo Corp, seed 自带)。
const approvalTestOrg = "00000000-0000-4000-a000-000000000001"

// countingExecutor 计数执行次数; 可控制返回错误。
type countingExecutor struct {
	calls int32
	err   error
}

func (e *countingExecutor) Execute(ctx context.Context, pool *pgxpool.Pool, req *repository.ApprovalRequestRow) (interface{}, error) {
	atomic.AddInt32(&e.calls, 1)
	if e.err != nil {
		return nil, e.err
	}
	return map[string]string{"ok": "1"}, nil
}

// seedApproval 直接通过 repo 创建一条 pending 审批 (绕过 service.Create 的 HasPending 检查),
// 返回 id; t.Cleanup 负责删除。
func seedApproval(t *testing.T, pool *pgxpool.Pool, resourceType, resourceID string) string {
	t.Helper()
	repo := repository.NewApprovalRepo()
	id, err := repo.Create(context.Background(), pool, &repository.ApprovalRequestRow{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		RequesterID:  "00000000-0000-4000-a000-000000000010",
		OrgID:        approvalTestOrg,
		Payload:      []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM assets.approval_requests WHERE id = $1`, id)
	})
	return id
}

// auditSummaryForApproval 读取该审批最近的 audit 摘要 (按 audit_log.asset_id + action 过滤,
// 解析 metadata.new_values)。audit.Record 将整个 Entry 序列化进 metadata 列。
func auditSummaryForApproval(t *testing.T, pool *pgxpool.Pool, approvalID, action string) map[string]interface{} {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(context.Background(),
		`SELECT metadata FROM assets.audit_log
		 WHERE asset_id = $1 AND action = $2
		 ORDER BY created_at DESC, id DESC LIMIT 1`,
		approvalID, action,
	).Scan(&raw)
	if err != nil {
		t.Fatalf("read audit %s for %s: %v", action, approvalID, err)
	}
	var entry struct {
		NewValues map[string]interface{} `json:"new_values"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("unmarshal audit metadata: %v", err)
	}
	if entry.NewValues == nil {
		t.Fatalf("audit metadata has no new_values: %s", raw)
	}
	return entry.NewValues
}

// ============================================================
// TestApprovalStateMachine_Transitions — pending → approved/rejected, 终态拒绝 (A2 影子)
// ============================================================
func TestApprovalStateMachine_Transitions(t *testing.T) {
	pool := testPool(t)
	repo := repository.NewApprovalRepo()
	svc := NewApprovalService(repo, pool)
	exec := &countingExecutor{}
	svc.RegisterExecutor(ApprovalMaintenance, exec)

	// 场景 1: pending → approved; 再 approve/reject 应拒绝
	id1 := seedApproval(t, pool, "maintenance", "smoke-asset-approve-"+randomSuffix())
	if err := svc.Approve(context.Background(), id1, approvalTestOrg, "00000000-0000-4000-a000-000000000010"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if err := svc.Approve(context.Background(), id1, approvalTestOrg, "00000000-0000-4000-a000-000000000010"); err == nil {
		t.Fatal("second approve on approved should fail")
	}
	if err := svc.Reject(context.Background(), id1, approvalTestOrg, "00000000-0000-4000-a000-000000000010", "late"); err == nil {
		t.Fatal("reject on approved should fail")
	}
	row, err := repo.Get(context.Background(), pool, id1, approvalTestOrg)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != "approved" {
		t.Fatalf("status = %s, want approved", row.Status)
	}

	// 场景 2: pending → rejected; 再 approve 应拒绝
	id2 := seedApproval(t, pool, "maintenance", "smoke-asset-reject-"+randomSuffix())
	if err := svc.Reject(context.Background(), id2, approvalTestOrg, "00000000-0000-4000-a000-000000000010", "nope"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if err := svc.Approve(context.Background(), id2, approvalTestOrg, "00000000-0000-4000-a000-000000000010"); err == nil {
		t.Fatal("approve on rejected should fail")
	}
	row2, err := repo.Get(context.Background(), pool, id2, approvalTestOrg)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row2.Status != "rejected" {
		t.Fatalf("status = %s, want rejected", row2.Status)
	}
}

// ============================================================
// TestApprovalStateMachine_ConcurrentApprove — 两 goroutine 同时 approve → 单成功, executor 单次
// ============================================================
func TestApprovalStateMachine_ConcurrentApprove(t *testing.T) {
	pool := testPool(t)
	repo := repository.NewApprovalRepo()
	svc := NewApprovalService(repo, pool)
	exec := &countingExecutor{}
	svc.RegisterExecutor(ApprovalMaintenance, exec)

	id := seedApproval(t, pool, "maintenance", "smoke-asset-concurrent-"+randomSuffix())

	var (
		wg         sync.WaitGroup
		successCnt int32
		errCnt     int32
	)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.Approve(context.Background(), id, approvalTestOrg, "00000000-0000-4000-a000-000000000010")
			if err == nil {
				atomic.AddInt32(&successCnt, 1)
			} else {
				atomic.AddInt32(&errCnt, 1)
			}
		}()
	}
	wg.Wait()

	if successCnt != 1 {
		t.Fatalf("expected exactly 1 successful approve, got %d (err=%d)", successCnt, errCnt)
	}
	if errCnt != 1 {
		t.Fatalf("expected exactly 1 failed approve, got %d", errCnt)
	}
	if exec.calls != 1 {
		t.Fatalf("executor should run exactly once, got %d", exec.calls)
	}

	// 终态校验
	row, err := repo.Get(context.Background(), pool, id, approvalTestOrg)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != "approved" {
		t.Fatalf("status = %s, want approved", row.Status)
	}
}

// ============================================================
// TestApprovalExecutionFailure — executor 失败 → 审批 approved + 审计 exec_error, 业务不发生
// ============================================================
func TestApprovalExecutionFailure(t *testing.T) {
	pool := testPool(t)
	repo := repository.NewApprovalRepo()
	svc := NewApprovalService(repo, pool)
	exec := &countingExecutor{err: errors.New("asset already retired")}
	svc.RegisterExecutor(ApprovalMaintenance, exec)

	id := seedApproval(t, pool, "maintenance", "smoke-asset-execfail-"+randomSuffix())

	err := svc.Approve(context.Background(), id, approvalTestOrg, "00000000-0000-4000-a000-000000000010")
	if err == nil {
		t.Fatal("approve with failing executor should return error")
	}

	// 审批状态机已转为 approved (状态先于执行更新)
	row, err := repo.Get(context.Background(), pool, id, approvalTestOrg)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if row.Status != "approved" {
		t.Fatalf("status = %s, want approved (state machine advances before exec)", row.Status)
	}

	// executor 被调用一次 (业务尝试执行)
	if exec.calls != 1 {
		t.Fatalf("executor should be called once, got %d", exec.calls)
	}

	// 审计摘要包含 exec_error
	summary := auditSummaryForApproval(t, pool, id, "approval_approved")
	if _, ok := summary["exec_error"]; !ok {
		t.Fatalf("audit summary should contain exec_error, got %v", summary)
	}
	if ee, _ := summary["exec_error"].(string); ee == "" {
		t.Fatalf("exec_error should be non-empty, got %v", summary)
	}
}

// randomSuffix 生成短随机后缀避免 resource_id 冲突。
func randomSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
