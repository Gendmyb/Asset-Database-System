// Package service — 多级审批流 (Wave 2 G7)
//
// 职责:
//   - 创建审批请求 (pending), 存储原始请求 payload 供通过后回放
//   - 审批通过/拒绝状态机 (pending → approved/rejected)
//   - 通过后回调执行原业务操作 (领用生效 / 报废执行 / 维修工单创建)
//     —— 通过 Executor 回调解耦, 避免硬编码业务逻辑
//   - 审计: 创建/决策写审计摘要 (独立事务, 参照 Wave1 ldap writeAuditSeparately 模式)
//
// 向后兼容: 审批门由系统设置开关控制 (approval.{resource}.enabled),
// 关闭时调用方走原直操作路径, 行为与 v0.2.0 一致。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ApprovalResourceType 审批资源类型
type ApprovalResourceType string

const (
	ApprovalAssignment  ApprovalResourceType = "assignment"
	ApprovalRetirement  ApprovalResourceType = "retirement"
	ApprovalMaintenance ApprovalResourceType = "maintenance"
)

// ApprovalExecutor 审批通过后的业务执行回调 (由各业务 service 实现)
// 返回的 result 会被记录到审计摘要中 (不含凭据)。
type ApprovalExecutor interface {
	// Execute 在审批通过后执行原业务操作; actorID 为发起人 (requester)
	Execute(ctx context.Context, pool *pgxpool.Pool, req *repository.ApprovalRequestRow) (result interface{}, err error)
}

// ApprovalService 审批服务
type ApprovalService struct {
	repo      *repository.ApprovalRepo
	pool      *pgxpool.Pool
	executors map[ApprovalResourceType]ApprovalExecutor
}

func NewApprovalService(repo *repository.ApprovalRepo, pool *pgxpool.Pool) *ApprovalService {
	return &ApprovalService{
		repo:      repo,
		pool:      pool,
		executors: make(map[ApprovalResourceType]ApprovalExecutor),
	}
}

// RegisterExecutor 注册某资源类型的业务执行回调
func (s *ApprovalService) RegisterExecutor(rt ApprovalResourceType, exec ApprovalExecutor) {
	s.executors[rt] = exec
}

// CreateInput 创建审批请求入参
type CreateInput struct {
	ResourceType ApprovalResourceType
	ResourceID   string
	RequesterID  string
	OrgID        string
	Payload      interface{} // 原始请求载荷, JSON 序列化后存储
}

// Create 创建一条 pending 审批请求 (同资源已有 pending 时拒绝, 避免重复)
func (s *ApprovalService) Create(ctx context.Context, in CreateInput) (string, error) {
	exists, err := s.repo.HasPending(ctx, s.pool, string(in.ResourceType), in.ResourceID, in.OrgID)
	if err != nil {
		return "", err
	}
	if exists {
		return "", fmt.Errorf("该资源已有待审批请求")
	}

	payload, _ := json.Marshal(in.Payload)
	id, err := s.repo.Create(ctx, s.pool, &repository.ApprovalRequestRow{
		ResourceType: string(in.ResourceType),
		ResourceID:   in.ResourceID,
		RequesterID:  in.RequesterID,
		OrgID:        in.OrgID,
		Status:       "pending",
		CurrentStep:  1,
		Payload:      payload,
	})
	if err != nil {
		return "", err
	}

	// 审计: 创建审批 (独立事务)
	s.writeAudit(ctx, audit.Entry{
		TableName: "approval_requests",
		RecordID:  id,
		Action:    "approval_created",
		OrgID:     in.OrgID,
		ActorID:   in.RequesterID,
		NewValues: mustJSON(map[string]interface{}{
			"resource_type": in.ResourceType,
			"resource_id":   in.ResourceID,
		}),
	})
	return id, nil
}

// Approve 审批通过 → 执行业务回调
func (s *ApprovalService) Approve(ctx context.Context, id, orgID, deciderID string) error {
	req, err := s.repo.Get(ctx, s.pool, id, orgID)
	if err != nil {
		return fmt.Errorf("审批请求不存在: %w", err)
	}
	if req.Status != "pending" {
		return fmt.Errorf("审批请求状态为 %s, 无法审批", req.Status)
	}

	// 1. 状态机: pending → approved
	if err := s.repo.UpdateStatus(ctx, s.pool, id, orgID, "approved", deciderID, ""); err != nil {
		return fmt.Errorf("更新审批状态失败: %w", err)
	}

	// 2. 执行业务回调 (审批通过才真正执行领用/报废/维修)
	exec, ok := s.executors[ApprovalResourceType(req.ResourceType)]
	if !ok {
		slog.Warn("approval: no executor registered, skipping business op",
			"resource_type", req.ResourceType, "id", id)
		return nil
	}
	result, execErr := exec.Execute(ctx, s.pool, req)

	// 3. 审计: 决策摘要 (独立事务; 不含凭据)
	summary := map[string]interface{}{
		"resource_type": req.ResourceType,
		"resource_id":   req.ResourceID,
		"decided_by":    deciderID,
		"status":        "approved",
	}
	if execErr != nil {
		summary["exec_error"] = execErr.Error()
	} else if result != nil {
		summary["result"] = result
	}
	s.writeAudit(ctx, audit.Entry{
		TableName: "approval_requests",
		RecordID:  id,
		Action:    "approval_approved",
		OrgID:     orgID,
		ActorID:   deciderID,
		NewValues: mustJSON(summary),
	})

	if execErr != nil {
		return fmt.Errorf("审批已通过但业务执行失败: %w", execErr)
	}
	return nil
}

// Reject 审批拒绝
func (s *ApprovalService) Reject(ctx context.Context, id, orgID, deciderID, reason string) error {
	// 先取审批请求以获得 resource_type (审计摘要需要, 且校验状态)
	req, err := s.repo.Get(ctx, s.pool, id, orgID)
	if err != nil {
		return fmt.Errorf("审批请求不存在: %w", err)
	}
	if req.Status != "pending" {
		return fmt.Errorf("审批请求状态为 %s, 无法拒绝", req.Status)
	}

	if err := s.repo.UpdateStatus(ctx, s.pool, id, orgID, "rejected", deciderID, reason); err != nil {
		return fmt.Errorf("更新审批状态失败: %w", err)
	}

	s.writeAudit(ctx, audit.Entry{
		TableName: "approval_requests",
		RecordID:  id,
		Action:    "approval_rejected",
		OrgID:     orgID,
		ActorID:   deciderID,
		NewValues: mustJSON(map[string]interface{}{
			"resource_type": req.ResourceType,
			"resource_id":   req.ResourceID,
			"reason":        reason,
			"decided_by":    deciderID,
		}),
	})
	return nil
}

// Get 获取审批请求
func (s *ApprovalService) Get(ctx context.Context, id, orgID string) (*repository.ApprovalRequestRow, error) {
	return s.repo.Get(ctx, s.pool, id, orgID)
}

// List 列出审批请求
func (s *ApprovalService) List(ctx context.Context, orgID, status string, limit int) ([]repository.ApprovalRequestRow, error) {
	return s.repo.List(ctx, s.pool, orgID, status, limit)
}

// writeAudit 在独立事务写审计摘要, 失败仅记日志 (参照 Wave1 ldap writeAuditSeparately)
func (s *ApprovalService) writeAudit(ctx context.Context, e audit.Entry) {
	if s.pool == nil {
		return
	}
	// 截断 ≤3500 字节 (audit_log CHECK 4096, 留余量给 Entry 包装字段)
	if len(e.NewValues) > 3500 {
		e.NewValues = []byte(`{"truncated":true}`)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		slog.Warn("approval: begin audit tx failed", "error", err)
		return
	}
	defer tx.Rollback(ctx)
	if err := audit.Record(ctx, tx, e); err != nil {
		slog.Warn("approval: write audit failed", "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Warn("approval: commit audit failed", "error", err)
	}
}

// mustJSON JSON 编码, 失败返回空对象
func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

// IsApprovalEnabled 读取系统设置 approval.<resource>.enabled (默认 false)
// 设置缺失或解析失败时返回 false (向后兼容: 关闭时直操作)。
func IsApprovalEnabled(ctx context.Context, settingsRepo *repository.SettingsRepo, pool *pgxpool.Pool, resource string) bool {
	if settingsRepo == nil || pool == nil {
		return false
	}
	v, err := settingsRepo.Get(ctx, pool, "approval."+resource+".enabled")
	if err != nil {
		return false
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}
