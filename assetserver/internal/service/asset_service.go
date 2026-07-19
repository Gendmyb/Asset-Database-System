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
	ErrAssetNotFound     = fmt.Errorf("asset not found")
	ErrVersionConflict   = fmt.Errorf("version conflict")
	ErrInvalidTransition = fmt.Errorf("invalid lifecycle transition")
	ErrAssetNotAvailable = fmt.Errorf("asset not available for assignment")
	ErrInvalidParent     = fmt.Errorf("invalid parent asset")        // G8
	ErrCycleDetected     = fmt.Errorf("parent would create a cycle") // G8
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
	AssetTag       string // 留空则自动生成
	SerialNumber   *string
	Manufacturer   *string
	Model          *string
	LifecycleState string
	Properties     []byte
	ActorID        string // 操作人
	// Phase E: 采购/折旧字段
	PurchasePrice      *float64
	PurchaseDate       *time.Time
	Supplier           *string
	WarrantyUntil      *time.Time
	DepreciationMethod string
	UsefulLifeMonths   *int
	SalvageValue       float64
	ManagedBy          *string
	// Wave 2 G8: 父资产 (外设挂载); nil/空 表示无父资产
	ParentAssetID *string
}

// CreateAsset 创建资产 (事务: 生成编号 + INSERT + audit_log)
func (s *AssetService) CreateAsset(ctx context.Context, pool *pgxpool.Pool, input CreateAssetInput) (*repository.AssetRow, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 编号: 用户提供则用用户的, 否则自动生成
	tag := input.AssetTag
	if tag == "" {
		tag, err = s.settingsRepo.NextAssetTag(ctx, tx, input.OrgID)
		if err != nil {
			return nil, fmt.Errorf("generate tag: %w", err)
		}
	}

	now := time.Now()
	if input.LifecycleState == "" {
		input.LifecycleState = "deployment"
	}
	if input.DepreciationMethod == "" {
		input.DepreciationMethod = "none"
	}

	// G8: 校验父资产 (存在性 + 同 org)。新建资产无后代, 无循环风险。
	var parentAssetID *string
	if input.ParentAssetID != nil && *input.ParentAssetID != "" {
		pid := *input.ParentAssetID
		// 父资产必须存在且属于同 org
		if _, err := s.assetRepo.GetByID(ctx, tx, pid, input.OrgID); err != nil {
			return nil, ErrInvalidParent
		}
		parentAssetID = &pid
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
		// Phase E fields
		PurchasePrice:      input.PurchasePrice,
		PurchaseDate:       input.PurchaseDate,
		Supplier:           input.Supplier,
		WarrantyUntil:      input.WarrantyUntil,
		DepreciationMethod: input.DepreciationMethod,
		UsefulLifeMonths:   input.UsefulLifeMonths,
		SalvageValue:       input.SalvageValue,
		ManagedBy:          input.ManagedBy,
		// G8
		ParentAssetID: parentAssetID,
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

// CreateAssetBatch 批量创建资产 (事务: 原子取号 + 逐个 INSERT + audit_log)
// Phase E: 使用 doc_sequences 原子取号防止并发重号
func (s *AssetService) CreateAssetBatch(ctx context.Context, pool *pgxpool.Pool, input CreateAssetInput, count int) ([]*repository.AssetRow, error) {
	if count <= 0 || count > 100 {
		return nil, fmt.Errorf("count must be between 1 and 100, got %d", count)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 原子批量取号
	tags, err := s.settingsRepo.NextBatchTags(ctx, tx, input.OrgID, count)
	if err != nil {
		return nil, fmt.Errorf("batch generate tags: %w", err)
	}

	now := time.Now()
	if input.LifecycleState == "" {
		input.LifecycleState = "deployment"
	}
	if input.DepreciationMethod == "" {
		input.DepreciationMethod = "none"
	}

	assets := make([]*repository.AssetRow, 0, count)
	for i := 0; i < count; i++ {
		row := &repository.AssetRow{
			ID:             uuid.New().String(),
			AssetTag:       tags[i],
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
			// Phase E fields
			PurchasePrice:      input.PurchasePrice,
			PurchaseDate:       input.PurchaseDate,
			Supplier:           input.Supplier,
			WarrantyUntil:      input.WarrantyUntil,
			DepreciationMethod: input.DepreciationMethod,
			UsefulLifeMonths:   input.UsefulLifeMonths,
			SalvageValue:       input.SalvageValue,
			ManagedBy:          input.ManagedBy,
		}

		if err := s.assetRepo.Create(ctx, tx, row); err != nil {
			return nil, fmt.Errorf("create asset %d: %w", i+1, err)
		}

		// 审计日志 (每条资产单独记录)
		newVals, _ := json.Marshal(row)
		if err := audit.Record(ctx, tx, audit.Entry{
			TableName: "assets",
			RecordID:  row.ID,
			Action:    audit.ActionCreated,
			OrgID:     input.OrgID,
			ActorID:   input.ActorID,
			NewValues: newVals,
		}); err != nil {
			return nil, fmt.Errorf("audit record for asset %d: %w", i+1, err)
		}

		assets = append(assets, row)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// 事件发布 (批量)
	for _, row := range assets {
		_ = event.DefaultBus.Publish(ctx, &event.Event{
			Type:    event.EventAssetCreated,
			AssetID: row.ID,
			OrgID:   input.OrgID,
			UserID:  input.ActorID,
		})
	}

	return assets, nil
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

// AssetDetail 资产详情 (含 G8 外设树: parent + children)
type AssetDetail struct {
	Asset    *repository.AssetRow
	Parent   *repository.AssetRow
	Children []repository.AssetRow
}

// GetAssetDetail 获取资产详情 + 外设树 (G8), 按 scope 过滤防 IDOR (G9)。
func (s *AssetService) GetAssetDetail(ctx context.Context, pool *pgxpool.Pool, id string, scope repository.OrgScope) (*AssetDetail, error) {
	asset, err := s.assetRepo.GetByIDScoped(ctx, pool, id, scope)
	if err != nil {
		return nil, ErrAssetNotFound
	}
	detail := &AssetDetail{Asset: asset}

	if asset.ParentAssetID != nil && *asset.ParentAssetID != "" {
		// 父资产用同 scope 查询 (部门级可见时, 跨部门父资产不可见 → 视为无父)
		parent, err := s.assetRepo.GetByIDScoped(ctx, pool, *asset.ParentAssetID, scope)
		if err == nil {
			detail.Parent = parent
		}
	}

	children, err := s.assetRepo.GetChildren(ctx, pool, id, scope)
	if err != nil {
		return nil, fmt.Errorf("get children: %w", err)
	}
	if children == nil {
		children = []repository.AssetRow{}
	}
	detail.Children = children
	return detail, nil
}

// MountAsset 将 asset 挂载到 parentAsset 下 (G8)。事务: 校验 + UPDATE + audit。
// 防循环: parent 不能是 asset 自身或其后代。
func (s *AssetService) MountAsset(ctx context.Context, pool *pgxpool.Pool, assetID, parentAssetID, orgID, actorID string) (*repository.AssetRow, error) {
	if assetID == parentAssetID {
		return nil, ErrCycleDetected
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 悲观锁资产 (防并发挂载)
	asset, err := s.assetRepo.LockForUpdate(ctx, tx, assetID, orgID)
	if err != nil {
		return nil, ErrAssetNotFound
	}

	// 父资产必须存在且同 org
	parent, err := s.assetRepo.GetByID(ctx, tx, parentAssetID, orgID)
	if err != nil {
		return nil, ErrInvalidParent
	}

	// 防循环: parent 不能是 asset 的后代 (否则形成环)
	isDesc, err := s.assetRepo.IsDescendant(ctx, tx, assetID, parent.ID, orgID)
	if err != nil {
		return nil, fmt.Errorf("cycle check: %w", err)
	}
	if isDesc {
		return nil, ErrCycleDetected
	}

	updated, err := s.assetRepo.SetParent(ctx, tx, assetID, orgID, parentAssetID, asset.Version)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVersionConflict, err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assets",
		RecordID:  assetID,
		Action:    audit.ActionUpdated,
		OrgID:     orgID,
		ActorID:   actorID,
		NewValues: []byte(`{"parent_asset_id":"` + parentAssetID + `"}`),
	}); err != nil {
		return nil, fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return updated, nil
}

// UnmountAsset 解除资产的挂载 (G8, parent_asset_id 置 NULL)。
func (s *AssetService) UnmountAsset(ctx context.Context, pool *pgxpool.Pool, assetID, orgID, actorID string) (*repository.AssetRow, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	asset, err := s.assetRepo.LockForUpdate(ctx, tx, assetID, orgID)
	if err != nil {
		return nil, ErrAssetNotFound
	}

	updated, err := s.assetRepo.SetParent(ctx, tx, assetID, orgID, "", asset.Version)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVersionConflict, err)
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assets",
		RecordID:  assetID,
		Action:    audit.ActionUpdated,
		OrgID:     orgID,
		ActorID:   actorID,
		NewValues: []byte(`{"parent_asset_id":null}`),
	}); err != nil {
		return nil, fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return updated, nil
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
