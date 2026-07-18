// Package service — 资产服务层 (事务边界)
// 对应 Phase B §4 Service 层 + §6 审计 Recorder
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/domain"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors
var (
	ErrAssetNotFound      = fmt.Errorf("asset not found")
	ErrVersionConflict    = fmt.Errorf("version conflict")
	ErrInvalidTransition  = fmt.Errorf("invalid lifecycle transition")
	ErrAssetNotAvailable  = fmt.Errorf("asset not available for assignment")
)

// AssetService 资产服务 (事务边界)
type AssetService struct {
	assetRepo    *repository.AssetRepo
	settingsRepo *repository.SettingsRepo
}

func NewAssetService(assetRepo *repository.AssetRepo, settingsRepo *repository.SettingsRepo) *AssetService {
	return &AssetService{assetRepo: assetRepo, settingsRepo: settingsRepo}
}

// CreateAssetInput 创建资产输入
type CreateAssetInput struct {
	Name           string
	TypeID         string
	OrgID          string
	SerialNumber   *string
	Manufacturer   *string
	Model          *string
	LifecycleState string
	Properties     []byte
	ActorID        string // 操作人
}

// CreateAsset 创建资产 (事务: 生成编号 + INSERT + audit_log)
func (s *AssetService) CreateAsset(ctx context.Context, pool *pgxpool.Pool, input CreateAssetInput) (*repository.AssetRow, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 自动生成编号
	tag, err := s.settingsRepo.NextAssetTag(ctx, tx, input.OrgID)
	if err != nil {
		return nil, fmt.Errorf("generate tag: %w", err)
	}

	now := time.Now()
	if input.LifecycleState == "" {
		input.LifecycleState = "procurement"
	}

	row := &repository.AssetRow{
		ID:             uuid.New().String(),
		AssetTag:       tag,
		Name:           input.Name,
		TypeID:         input.TypeID,
		OrgID:          input.OrgID,
		SerialNumber:   input.SerialNumber,
		Manufacturer:   input.Manufacturer,
		Model:          input.Model,
		LifecycleState: input.LifecycleState,
		Status:         "available",
		Properties:     input.Properties,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.assetRepo.Create(ctx, tx, row); err != nil {
		return nil, fmt.Errorf("create asset: %w", err)
	}

	// 审计日志
	newVals, _ := json.Marshal(row)
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assets",
		RecordID:  row.ID,
		Action:    audit.ActionCreated,
		OrgID:     input.OrgID,
		ActorID:   input.ActorID,
		NewValues: newVals,
	}); err != nil {
		return nil, fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// 事件发布
	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    event.EventAssetCreated,
		AssetID: row.ID,
		OrgID:   input.OrgID,
		UserID:  input.ActorID,
	})

	return row, nil
}

// UpdateAsset 更新资产 (事务: UPDATE + audit_log)
func (s *AssetService) UpdateAsset(ctx context.Context, pool *pgxpool.Pool, id string, orgID string, actorID string, updates map[string]interface{}, expectedVersion int) (*repository.AssetRow, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 生命周期状态转换校验 (如果 update 中包含)
	if newState, ok := updates["lifecycle_state"].(string); ok && newState != "" {
		current, err := s.assetRepo.GetByID(ctx, tx, id, orgID)
		if err != nil {
			return nil, ErrAssetNotFound
		}
		if err := domain.ValidateTransition(
			domain.LifecycleState(current.LifecycleState),
			domain.LifecycleState(newState),
		); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidTransition, err)
		}
	}

	row, err := s.assetRepo.UpdateWithRetry(ctx, tx, id, orgID, updates, expectedVersion)
	if err != nil {
		if err.Error() == "version conflict" || contains(err.Error(), "version conflict") {
			return nil, ErrVersionConflict
		}
		return nil, fmt.Errorf("update asset: %w", err)
	}

	// 审计日志
	newVals, _ := json.Marshal(row)
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assets",
		RecordID:  id,
		Action:    audit.ActionUpdated,
		OrgID:     orgID,
		ActorID:   actorID,
		NewValues: newVals,
	}); err != nil {
		return nil, fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    event.EventAssetUpdated,
		AssetID: id,
		OrgID:   orgID,
		UserID:  actorID,
	})

	return row, nil
}

// DeleteAsset 软删除资产 (事务: soft delete + audit_log)
func (s *AssetService) DeleteAsset(ctx context.Context, pool *pgxpool.Pool, id string, orgID string, actorID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.assetRepo.SoftDelete(ctx, tx, id, orgID); err != nil {
		return ErrAssetNotFound
	}

	// 审计日志
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assets",
		RecordID:  id,
		Action:    audit.ActionDeleted,
		OrgID:     orgID,
		ActorID:   actorID,
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    event.EventAssetDeleted,
		AssetID: id,
		OrgID:   orgID,
		UserID:  actorID,
	})

	return nil
}

// TransitionAsset 生命周期状态转换 (事务: SELECT FOR UPDATE + 校验 + UPDATE + audit_log)
func (s *AssetService) TransitionAsset(ctx context.Context, pool *pgxpool.Pool, id string, orgID string, actorID string, to string) (*repository.AssetRow, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 悲观锁: SELECT ... FOR UPDATE (SET LOCAL lock_timeout 已在 LockForUpdate 内)
	row, err := s.assetRepo.LockForUpdate(ctx, tx, id, orgID)
	if err != nil {
		return nil, ErrAssetNotFound
	}

	// 状态机校验
	if err := domain.ValidateTransition(
		domain.LifecycleState(row.LifecycleState),
		domain.LifecycleState(to),
	); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTransition, err)
	}

	updated, err := s.assetRepo.UpdateWithRetry(ctx, tx, id, orgID,
		map[string]interface{}{"lifecycle_state": to}, row.Version)
	if err != nil {
		return nil, fmt.Errorf("update lifecycle: %w", err)
	}

	// 审计日志
	newVals, _ := json.Marshal(updated)
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assets",
		RecordID:  id,
		Action:    audit.ActionTransition,
		OrgID:     orgID,
		ActorID:   actorID,
		NewValues: newVals,
	}); err != nil {
		return nil, fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    event.EventLifecycleChanged,
		AssetID: id,
		OrgID:   orgID,
		UserID:  actorID,
	})

	return updated, nil
}

// GetAsset 获取单个资产 (非事务读)
func (s *AssetService) GetAsset(ctx context.Context, pool *pgxpool.Pool, id string, orgID string) (*repository.AssetRow, error) {
	row, err := s.assetRepo.GetByID(ctx, pool, id, orgID)
	if err != nil {
		return nil, ErrAssetNotFound
	}
	return row, nil
}

// ListAssets 列表查询 (非事务读)
func (s *AssetService) ListAssets(ctx context.Context, pool *pgxpool.Pool, f repository.AssetFilter) ([]repository.AssetRow, string, bool, error) {
	return s.assetRepo.List(ctx, pool, f)
}

// contains 简易字符串包含检查
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// 确保 json 被使用
var _ = json.Marshal
