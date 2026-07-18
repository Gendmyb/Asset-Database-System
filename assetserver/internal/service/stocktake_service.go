// Package service — 盘点服务层 (事务边界)
// Phase G: 盘点管理
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors for stocktake
var (
	ErrPlanNotFound       = fmt.Errorf("stocktake plan not found")
	ErrItemNotFound       = fmt.Errorf("stocktake item not found")
	ErrPlanNotInProgress  = fmt.Errorf("stocktake plan is not in progress")
	ErrPlanAlreadyStarted = fmt.Errorf("stocktake plan already started")
	ErrPlanNotStarted     = fmt.Errorf("stocktake plan has not been started")
)

// StocktakeService 盘点服务 (事务边界)
type StocktakeService struct {
	stocktakeRepo *repository.StocktakeRepo
	assetRepo     *repository.AssetRepo
	settingsRepo  *repository.SettingsRepo
}

func NewStocktakeService(
	stocktakeRepo *repository.StocktakeRepo,
	assetRepo *repository.AssetRepo,
	settingsRepo *repository.SettingsRepo,
) *StocktakeService {
	return &StocktakeService{
		stocktakeRepo: stocktakeRepo,
		assetRepo:     assetRepo,
		settingsRepo:  settingsRepo,
	}
}

// CreatePlanInput 创建盘点计划输入
type CreatePlanInput struct {
	Name            string
	ScopeLocationID *string
	ScopeTypeID     *string
	CreatedBy       string
}

// CreatePlan 创建盘点计划 (事务: 取号 + INSERT + audit)
func (s *StocktakeService) CreatePlan(ctx context.Context, pool *pgxpool.Pool, orgID string, input CreatePlanInput) (*repository.StocktakePlan, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 取号 STK-
	planNo, err := s.settingsRepo.NextStocktakePlanNo(ctx, tx, orgID)
	if err != nil {
		return nil, fmt.Errorf("generate plan no: %w", err)
	}

	now := time.Now()
	plan := &repository.StocktakePlan{
		ID:              uuid.New().String(),
		PlanNo:          planNo,
		OrgID:           orgID,
		Name:            input.Name,
		ScopeLocationID: input.ScopeLocationID,
		ScopeTypeID:     input.ScopeTypeID,
		Status:          "draft",
		CreatedBy:       input.CreatedBy,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.stocktakeRepo.CreatePlan(ctx, tx, plan); err != nil {
		return nil, fmt.Errorf("create plan: %w", err)
	}

	// audit
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "stocktake_plans",
		RecordID:  plan.ID,
		Action:    audit.ActionCreated,
		OrgID:     orgID,
		ActorID:   input.CreatedBy,
	}); err != nil {
		return nil, fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:   "stocktake.plan.created",
		OrgID:  orgID,
		UserID: input.CreatedBy,
	})

	return plan, nil
}

// StartPlan 启动盘点 (事务: UPDATE plan → SELECT scope assets → batch INSERT items → audit)
func (s *StocktakeService) StartPlan(ctx context.Context, pool *pgxpool.Pool, orgID string, planID string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. 获取 plan, 校验状态
	plan, err := s.stocktakeRepo.GetPlan(ctx, tx, planID, orgID)
	if err != nil {
		return ErrPlanNotFound
	}
	if plan.Status != "draft" {
		return ErrPlanAlreadyStarted
	}

	// 2. UPDATE plan status='in_progress', started_at
	now := time.Now()
	if err := s.stocktakeRepo.UpdatePlan(ctx, tx, planID, orgID, map[string]interface{}{
		"status":     "in_progress",
		"started_at": now,
	}); err != nil {
		return fmt.Errorf("update plan: %w", err)
	}

	// 3. 按 scope 查询非 retired 资产
	assetQuery := `SELECT id, location_id, status
		FROM assets.assets
		WHERE org_id = $1 AND deleted_at IS NULL AND status != 'retired'`
	args := []interface{}{orgID}
	argIdx := 2

	if plan.ScopeLocationID != nil {
		assetQuery += fmt.Sprintf(" AND location_id = $%d", argIdx)
		args = append(args, *plan.ScopeLocationID)
		argIdx++
	}
	if plan.ScopeTypeID != nil {
		assetQuery += fmt.Sprintf(" AND type_id = $%d", argIdx)
		args = append(args, *plan.ScopeTypeID)
		argIdx++
	}

	type assetResult struct {
		ID         string
		LocationID *string
		Status     string
	}

	rows, err := tx.Query(ctx, assetQuery, args...)
	if err != nil {
		return fmt.Errorf("query assets for stocktake: %w", err)
	}
	defer rows.Close()

	var assets []assetResult
	for rows.Next() {
		var a assetResult
		if err := rows.Scan(&a.ID, &a.LocationID, &a.Status); err != nil {
			return fmt.Errorf("scan asset: %w", err)
		}
		assets = append(assets, a)
	}

	// 4. 批量插入 stocktake_items
	items := make([]repository.StocktakeItem, len(assets))
	for i, a := range assets {
		status := a.Status
		items[i] = repository.StocktakeItem{
			ID:                 uuid.New().String(),
			PlanID:             planID,
			AssetID:            &a.ID,
			ExpectedLocationID: a.LocationID,
			ExpectedStatus:     &status,
			Result:             "pending",
		}
	}

	if err := s.stocktakeRepo.CreateItems(ctx, tx, items); err != nil {
		return fmt.Errorf("batch create items: %w", err)
	}

	// 5. audit
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "stocktake_plans",
		RecordID:  planID,
		Action:    "stocktake_started",
		OrgID:     orgID,
		ActorID:   "",
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:  "stocktake.plan.started",
		OrgID: orgID,
	})

	return nil
}

// UpdateItemInput 更新盘点明细输入
type UpdateItemInput struct {
	Result           string
	ActualLocationID *string
	Notes            *string
	CheckedBy        string
}

// UpdateItem 更新盘点明细 (仅允许 in_progress 计划)
func (s *StocktakeService) UpdateItem(ctx context.Context, pool *pgxpool.Pool, orgID string, planID string, itemID string, input UpdateItemInput) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. 校验 plan status = in_progress
	plan, err := s.stocktakeRepo.GetPlan(ctx, tx, planID, orgID)
	if err != nil {
		return ErrPlanNotFound
	}
	if plan.Status != "in_progress" {
		return ErrPlanNotInProgress
	}

	// 2. 校验 result 值
	validResults := map[string]bool{"pending": true, "found": true, "missing": true, "moved": true, "surplus": true}
	if !validResults[input.Result] {
		return fmt.Errorf("invalid result: %s", input.Result)
	}

	// 3. UPDATE item
	now := time.Now()
	updates := map[string]interface{}{
		"result":     input.Result,
		"checked_by": input.CheckedBy,
		"checked_at": now,
	}
	if input.ActualLocationID != nil {
		updates["actual_location_id"] = input.ActualLocationID
	}
	if input.Notes != nil {
		updates["notes"] = input.Notes
	}

	if err := s.stocktakeRepo.UpdateItem(ctx, tx, planID, itemID, updates); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// AddSurplusItemInput 盘盈登记输入
type AddSurplusItemInput struct {
	SurplusNote      string
	ActualLocationID *string
	Notes            *string
	CreatedBy        string
}

// AddSurplusItem 盘盈登记 (asset_id=NULL, result='surplus')
func (s *StocktakeService) AddSurplusItem(ctx context.Context, pool *pgxpool.Pool, orgID string, planID string, input AddSurplusItemInput) (*repository.StocktakeItem, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. 校验 plan status = in_progress
	plan, err := s.stocktakeRepo.GetPlan(ctx, tx, planID, orgID)
	if err != nil {
		return nil, ErrPlanNotFound
	}
	if plan.Status != "in_progress" {
		return nil, ErrPlanNotInProgress
	}

	// 2. INSERT surplus item
	item := repository.StocktakeItem{
		ID:               uuid.New().String(),
		PlanID:           planID,
		AssetID:          nil,
		Result:           "surplus",
		ActualLocationID: input.ActualLocationID,
		SurplusNote:      &input.SurplusNote,
		Notes:            input.Notes,
		CheckedBy:        &input.CreatedBy,
		CheckedAt:        timePtr(time.Now()),
	}

	items := []repository.StocktakeItem{item}
	if err := s.stocktakeRepo.CreateItems(ctx, tx, items); err != nil {
		return nil, fmt.Errorf("create surplus item: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &item, nil
}

// CompletePlan 完成盘点 (事务: UPDATE plan → 可选 apply_moves → audit)
func (s *StocktakeService) CompletePlan(ctx context.Context, pool *pgxpool.Pool, orgID string, planID string, applyMoves bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. 校验 plan status = in_progress
	plan, err := s.stocktakeRepo.GetPlan(ctx, tx, planID, orgID)
	if err != nil {
		return ErrPlanNotFound
	}
	if plan.Status != "in_progress" {
		return ErrPlanNotInProgress
	}

	// 2. UPDATE plan status='completed', finished_at
	now := time.Now()
	if err := s.stocktakeRepo.UpdatePlan(ctx, tx, planID, orgID, map[string]interface{}{
		"status":      "completed",
		"finished_at": now,
	}); err != nil {
		return fmt.Errorf("update plan: %w", err)
	}

	// 3. 若 apply_moves, 将 result='moved' 的 items 更新资产位置
	if applyMoves {
		movedItems, err := s.stocktakeRepo.ListItems(ctx, tx, repository.StocktakeItemFilter{
			PlanID: planID,
			Result: "moved",
		})
		if err != nil {
			return fmt.Errorf("list moved items: %w", err)
		}

		for _, item := range movedItems {
			if item.AssetID != nil && item.ActualLocationID != nil {
				now := time.Now()
				tag, err := tx.Exec(ctx,
					`UPDATE assets.assets SET location_id=$1, updated_at=$2, version=version+1
					 WHERE id=$3 AND org_id=$4 AND deleted_at IS NULL`,
					*item.ActualLocationID, now, *item.AssetID, orgID,
				)
				if err != nil {
					return fmt.Errorf("update asset location for moved item %s: %w", item.ID, err)
				}
				if tag.RowsAffected() == 0 {
					continue // asset not found, skip
				}

				// audit each moved asset
				if err := audit.Record(ctx, tx, audit.Entry{
					TableName: "assets",
					RecordID:  *item.AssetID,
					Action:    "stocktake_moved",
					OrgID:     orgID,
					ActorID:   "",
				}); err != nil {
					return fmt.Errorf("audit record for moved asset %s: %w", *item.AssetID, err)
				}
			}
		}
	}

	// 4. audit plan
	if err := audit.Record(ctx, tx, audit.Entry{
		TableName: "stocktake_plans",
		RecordID:  planID,
		Action:    "stocktake_completed",
		OrgID:     orgID,
		ActorID:   "",
	}); err != nil {
		return fmt.Errorf("audit record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	_ = event.DefaultBus.Publish(ctx, &event.Event{
		Type:  "stocktake.plan.completed",
		OrgID: orgID,
	})

	return nil
}

// PlanReport 盘点报告
type PlanReport struct {
	Plan      *repository.StocktakePlan  `json:"plan"`
	Summary   map[string]int64           `json:"summary"`
	Total     int64                      `json:"total"`
	Moved     []repository.StocktakeItem `json:"moved,omitempty"`
}

// GetPlanReport 获取盘点报告 (汇总 counts by result + moved 明细)
func (s *StocktakeService) GetPlanReport(ctx context.Context, pool *pgxpool.Pool, orgID string, planID string) (*PlanReport, error) {
	// 1. Get plan
	plan, err := s.stocktakeRepo.GetPlan(ctx, pool, planID, orgID)
	if err != nil {
		return nil, ErrPlanNotFound
	}

	// 2. Get progress counts
	progress, total, err := s.stocktakeRepo.GetPlanProgress(ctx, pool, planID)
	if err != nil {
		return nil, fmt.Errorf("get plan progress: %w", err)
	}

	// 3. Get moved details
	movedItems, err := s.stocktakeRepo.ListItems(ctx, pool, repository.StocktakeItemFilter{
		PlanID: planID,
		Result: "moved",
	})
	if err != nil {
		return nil, fmt.Errorf("list moved items: %w", err)
	}

	return &PlanReport{
		Plan:    plan,
		Summary: progress,
		Total:   total,
		Moved:   movedItems,
	}, nil
}

// ListPlans 列表查询盘点计划 (游标分页)
func (s *StocktakeService) ListPlans(ctx context.Context, pool *pgxpool.Pool, f repository.StocktakeFilter) ([]repository.StocktakePlan, string, bool, error) {
	return s.stocktakeRepo.ListPlans(ctx, pool, f)
}

// GetPlan 获取单个盘点计划
func (s *StocktakeService) GetPlan(ctx context.Context, pool *pgxpool.Pool, id string, orgID string) (*repository.StocktakePlan, error) {
	plan, err := s.stocktakeRepo.GetPlan(ctx, pool, id, orgID)
	if err != nil {
		return nil, ErrPlanNotFound
	}
	return plan, nil
}

// GetItems 获取盘点明细列表
func (s *StocktakeService) GetItems(ctx context.Context, pool *pgxpool.Pool, planID string, result string, search string) ([]repository.StocktakeItem, error) {
	return s.stocktakeRepo.ListItems(ctx, pool, repository.StocktakeItemFilter{
		PlanID: planID,
		Result: result,
		Search: search,
	})
}

// timePtr helper
func timePtr(t time.Time) *time.Time {
	return &t
}
