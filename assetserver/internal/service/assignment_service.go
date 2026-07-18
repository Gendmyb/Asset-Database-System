// Package service — 领用服务层 (事务边界)
// 对应 Phase B §4 Service 层 + §6 审计 Recorder
package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AssignmentService 领用服务 (事务边界)
type AssignmentService struct {
	assignmentRepo *repository.AssignmentRepo
}

func NewAssignmentService(assignmentRepo *repository.AssignmentRepo) *AssignmentService {
	return &AssignmentService{assignmentRepo: assignmentRepo}
}

// Assign 领用资产 (事务: 悲观锁 + check-then-act + audit_log)
func (s *AssignmentService) Assign(ctx context.Context, pool *pgxpool.Pool, assetID, orgID, assignedTo, assignedBy, notes string) (string, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	assignmentID, err := s.assignmentRepo.Assign(ctx, tx, assetID, orgID, assignedTo, assignedBy, notes)
	if err != nil {
		return "", err
	}

	// 审计日志
	detail, _ := json.Marshal(map[string]string{
		"assignment_id": assignmentID,
		"assigned_to":   assignedTo,
	})
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assignments",
		RecordID:  assetID,
		Action:    audit.ActionAssigned,
		OrgID:     orgID,
		ActorID:   assignedBy,
		NewValues: detail,
	}); err != nil {
		return "", fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    event.EventAssetAssigned,
		AssetID: assetID,
		OrgID:   orgID,
		UserID:  assignedBy,
	})

	return assignmentID, nil
}

// Release 归还资产 (事务: 悲观锁 + 关闭 assignment + 恢复状态 + audit_log)
func (s *AssignmentService) Release(ctx context.Context, pool *pgxpool.Pool, assetID string, orgID string, actorID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.assignmentRepo.Release(ctx, tx, assetID, orgID); err != nil {
		return err
	}

	// 审计日志
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assignments",
		RecordID:  assetID,
		Action:    audit.ActionReleased,
		OrgID:     orgID,
		ActorID:   actorID,
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    event.EventAssetReleased,
		AssetID: assetID,
		OrgID:   orgID,
		UserID:  actorID,
	})

	return nil
}

// Transfer 转移资产 (事务: 字典序锁定 + 关闭旧 assignment + 创建新 assignment + audit_log)
func (s *AssignmentService) Transfer(ctx context.Context, pool *pgxpool.Pool, assetID string, orgID string, toUserID, userID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.assignmentRepo.Transfer(ctx, tx, assetID, orgID, toUserID, userID); err != nil {
		return err
	}

	// 审计日志
	detail, _ := json.Marshal(map[string]string{"to_user_id": toUserID})
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assignments",
		RecordID:  assetID,
		Action:    audit.ActionTransferred,
		OrgID:     orgID,
		ActorID:   userID,
		NewValues: detail,
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    event.EventAssetTransferred,
		AssetID: assetID,
		OrgID:   orgID,
		UserID:  userID,
	})

	return nil
}

// GetActiveAssignment 获取活跃领用记录 (非事务读)
func (s *AssignmentService) GetActiveAssignment(ctx context.Context, pool *pgxpool.Pool, assetID string) (*repository.ActiveAssignment, error) {
	return s.assignmentRepo.GetActiveAssignment(ctx, pool, assetID)
}
