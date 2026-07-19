// Package service — 维修/保养工单服务层 (事务边界) + 报废
// Phase F: 维修/保养工单+报废
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

// Sentinel errors for maintenance
var (
	ErrAssetRetired           = fmt.Errorf("asset is already retired")
	ErrActiveOrderExists      = fmt.Errorf("asset already has an active maintenance order")
	ErrOrderNotFound          = fmt.Errorf("maintenance order not found")
	ErrActiveAssignmentExists = fmt.Errorf("asset has active assignment, cannot retire")
	ErrActiveOrderForRetire   = fmt.Errorf("asset has active maintenance order, cannot retire")
)

// MaintenanceService 维修/保养工单服务 (事务边界)
type MaintenanceService struct {
	maintenanceRepo *repository.MaintenanceRepo
	assetRepo       *repository.AssetRepo
	assignmentRepo  *repository.AssignmentRepo
	settingsRepo    *repository.SettingsRepo
}

func NewMaintenanceService(
	maintenanceRepo *repository.MaintenanceRepo,
	assetRepo *repository.AssetRepo,
	assignmentRepo *repository.AssignmentRepo,
	settingsRepo *repository.SettingsRepo,
) *MaintenanceService {
	return &MaintenanceService{
		maintenanceRepo: maintenanceRepo,
		assetRepo:       assetRepo,
		assignmentRepo:  assignmentRepo,
		settingsRepo:    settingsRepo,
	}
}

// CreateOrderInput 创建工单输入
type CreateOrderInput struct {
	AssetID     string
	Category    string // repair / upkeep
	Title       string
	Description *string
	ReportedBy  string
	Assignee    *string
	Vendor      *string
}

// createOrderOutput 创建工单返回
type createOrderOutput struct {
	order *repository.MaintenanceOrder
}

// CreateOrder 创建维修/保养工单 (事务包裹+审计)
func (s *MaintenanceService) CreateOrder(ctx context.Context, pool *pgxpool.Pool, orgID string, input CreateOrderInput) (*repository.MaintenanceOrder, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. 校验 asset 存在 + 非 retired
	asset, err := s.assetRepo.GetByID(ctx, tx, input.AssetID, orgID)
	if err != nil {
		return nil, fmt.Errorf("asset not found: %w", err)
	}
	if asset.Status == "retired" {
		return nil, ErrAssetRetired
	}

	// 2. 校验此 asset 无活跃工单
	hasActive, err := s.maintenanceRepo.HasActiveOrder(ctx, tx, input.AssetID)
	if err != nil {
		return nil, fmt.Errorf("check active orders: %w", err)
	}
	if hasActive {
		return nil, ErrActiveOrderExists
	}

	// 3. 记录当前 asset status 为 prev_status
	prevStatus := asset.Status

	// 4. UPDATE assets SET status='maintenance'
	now := time.Now()
	tag, err := tx.Exec(ctx,
		`UPDATE assets.assets SET status='maintenance', version=version+1, updated_at=$1
		 WHERE id=$2 AND org_id=$3 AND deleted_at IS NULL`,
		now, input.AssetID, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("update asset status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrAssetNotFound
	}

	// 5. 取号 (scope='maintenance')
	orderNo, err := s.settingsRepo.NextMaintenanceOrderNo(ctx, tx, orgID)
	if err != nil {
		return nil, fmt.Errorf("generate order no: %w", err)
	}

	// 6. INSERT maintenance_orders
	mo := &repository.MaintenanceOrder{
		ID:          uuid.New().String(),
		OrderNo:     orderNo,
		AssetID:     input.AssetID,
		OrgID:       orgID,
		Category:    input.Category,
		Status:      "open",
		Title:       input.Title,
		Description: input.Description,
		ReportedBy:  input.ReportedBy,
		Assignee:    input.Assignee,
		Vendor:      input.Vendor,
		PrevStatus:  prevStatus,
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}

	if err := s.maintenanceRepo.CreateMaintenanceOrder(ctx, tx, mo); err != nil {
		return nil, fmt.Errorf("create maintenance order: %w", err)
	}

	// 7. audit + event
	detail, _ := json.Marshal(map[string]interface{}{
		"order_id":    mo.ID,
		"order_no":    mo.OrderNo,
		"asset_id":    mo.AssetID,
		"category":    mo.Category,
		"prev_status": mo.PrevStatus,
	})
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "maintenance_orders",
		RecordID:  mo.ID,
		Action:    "maintenance_created",
		OrgID:     orgID,
		ActorID:   input.ReportedBy,
		NewValues: detail,
	}); err != nil {
		return nil, fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    "maintenance.created",
		AssetID: mo.AssetID,
		OrgID:   orgID,
		UserID:  input.ReportedBy,
	})

	return mo, nil
}

// StartOrder 开始工单 (status→in_progress, started_at=now)
func (s *MaintenanceService) StartOrder(ctx context.Context, pool *pgxpool.Pool, orgID string, id string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	now := time.Now()
	if err := s.maintenanceRepo.UpdateMaintenanceOrder(ctx, tx, id, orgID, map[string]interface{}{
		"status":     "in_progress",
		"started_at": now,
	}); err != nil {
		return err
	}

	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "maintenance_orders",
		RecordID:  id,
		Action:    "maintenance_started",
		OrgID:     orgID,
		ActorID:   "",
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// CompleteOrder 完成工单 (status→completed, 恢复资产原状态)
func (s *MaintenanceService) CompleteOrder(ctx context.Context, pool *pgxpool.Pool, orgID string, id string, resolution string, cost float64) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. SELECT 工单获取 prev_status
	mo, err := s.maintenanceRepo.GetMaintenanceOrder(ctx, tx, id, orgID)
	if err != nil {
		return ErrOrderNotFound
	}

	// 2. UPDATE 工单 SET status='completed'
	now := time.Now()
	if err := s.maintenanceRepo.UpdateMaintenanceOrder(ctx, tx, id, orgID, map[string]interface{}{
		"status":      "completed",
		"resolution":  resolution,
		"cost":        cost,
		"finished_at": now,
	}); err != nil {
		return err
	}

	// 3. UPDATE assets SET status=prev_status
	tag, err := tx.Exec(ctx,
		`UPDATE assets.assets SET status=$1, version=version+1, updated_at=$2
		 WHERE id=$3 AND org_id=$4 AND deleted_at IS NULL`,
		mo.PrevStatus, now, mo.AssetID, orgID,
	)
	if err != nil {
		return fmt.Errorf("restore asset status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAssetNotFound
	}

	// 4. audit + event
	detail, _ := json.Marshal(map[string]interface{}{
		"order_id":    id,
		"resolution":  resolution,
		"cost":        cost,
		"prev_status": mo.PrevStatus,
	})
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "maintenance_orders",
		RecordID:  id,
		Action:    "maintenance_completed",
		OrgID:     orgID,
		ActorID:   "",
		NewValues: detail,
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    "maintenance.completed",
		AssetID: mo.AssetID,
		OrgID:   orgID,
	})

	return nil
}

// CancelOrder 取消工单 (status→canceled, 恢复资产原状态)
func (s *MaintenanceService) CancelOrder(ctx context.Context, pool *pgxpool.Pool, orgID string, id string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. SELECT 获取 prev_status
	mo, err := s.maintenanceRepo.GetMaintenanceOrder(ctx, tx, id, orgID)
	if err != nil {
		return ErrOrderNotFound
	}

	// 2. UPDATE 工单 SET status='canceled'
	now := time.Now()
	if err := s.maintenanceRepo.UpdateMaintenanceOrder(ctx, tx, id, orgID, map[string]interface{}{
		"status":      "canceled",
		"finished_at": now,
	}); err != nil {
		return err
	}

	// 3. UPDATE assets SET status=prev_status
	tag, err := tx.Exec(ctx,
		`UPDATE assets.assets SET status=$1, version=version+1, updated_at=$2
		 WHERE id=$3 AND org_id=$4 AND deleted_at IS NULL`,
		mo.PrevStatus, now, mo.AssetID, orgID,
	)
	if err != nil {
		return fmt.Errorf("restore asset status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAssetNotFound
	}

	// 4. audit
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "maintenance_orders",
		RecordID:  id,
		Action:    "maintenance_canceled",
		OrgID:     orgID,
		ActorID:   "",
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// ListOrders 列表查询工单 (游标分页)
func (s *MaintenanceService) ListOrders(ctx context.Context, pool *pgxpool.Pool, f repository.MaintenanceFilter) ([]repository.MaintenanceOrder, string, bool, error) {
	return s.maintenanceRepo.ListMaintenanceOrders(ctx, pool, f)
}

// GetOrder 获取单个工单
func (s *MaintenanceService) GetOrder(ctx context.Context, pool *pgxpool.Pool, id string, orgID string) (*repository.MaintenanceOrder, error) {
	mo, err := s.maintenanceRepo.GetMaintenanceOrder(ctx, pool, id, orgID)
	if err != nil {
		return nil, ErrOrderNotFound
	}
	return mo, nil
}

// RetireAsset 报废资产 (事务包裹+审计)
func (s *MaintenanceService) RetireAsset(ctx context.Context, pool *pgxpool.Pool, orgID string, assetID string, reason string, actorID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. SELECT asset 校验非 retired (终态不可再退)
	asset, err := s.assetRepo.LockForUpdate(ctx, tx, assetID, orgID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}
	if asset.Status == "retired" {
		return ErrAssetRetired
	}

	// 2. 校验无活跃 assignment
	var activeAssignments int64
	err = tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM assets.assignments
		 WHERE asset_id = $1 AND status = 'active'`, assetID,
	).Scan(&activeAssignments)
	if err != nil {
		return fmt.Errorf("check active assignments: %w", err)
	}
	if activeAssignments > 0 {
		return ErrActiveAssignmentExists
	}

	// 3. 校验无活跃工单
	hasActive, err := s.maintenanceRepo.HasActiveOrder(ctx, tx, assetID)
	if err != nil {
		return fmt.Errorf("check active orders: %w", err)
	}
	if hasActive {
		return ErrActiveOrderForRetire
	}

	// 4. lifecycle.ValidateTransition 状态机校验
	if err := domain.ValidateTransition(
		domain.LifecycleState(asset.LifecycleState),
		domain.StateRetirement,
	); err != nil {
		return fmt.Errorf("lifecycle validation: %w", err)
	}

	// 5. UPDATE asset SET status='retired', lifecycle='retirement', retired_at, retire_reason, version+1
	now := time.Now()
	tag, err := tx.Exec(ctx,
		`UPDATE assets.assets SET status='retired', lifecycle_state='retirement',
		 retired_at=$1, retire_reason=$2, version=version+1, updated_at=$3
		 WHERE id=$4 AND org_id=$5 AND deleted_at IS NULL`,
		now, reason, now, assetID, orgID,
	)
	if err != nil {
		return fmt.Errorf("update asset to retired: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAssetNotFound
	}

	// 6. audit + event
	detail, _ := json.Marshal(map[string]interface{}{
		"asset_id": assetID,
		"reason":   reason,
	})
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "assets",
		RecordID:  assetID,
		Action:    "retired",
		OrgID:     orgID,
		ActorID:   actorID,
		NewValues: detail,
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:    "asset.retired",
		AssetID: assetID,
		OrgID:   orgID,
		UserID:  actorID,
	})

	return nil
}
