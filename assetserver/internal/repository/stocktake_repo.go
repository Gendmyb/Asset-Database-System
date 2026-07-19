// Package repository — 盘点数据访问层 (PG)
// Phase G: 盘点管理
package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// StocktakeRepo 盘点仓库 (无状态 — DBTX 由调用方传入)
type StocktakeRepo struct{}

func NewStocktakeRepo() *StocktakeRepo {
	return &StocktakeRepo{}
}

// StocktakePlan 盘点计划领域模型
type StocktakePlan struct {
	ID              string     `json:"id"`
	PlanNo          string     `json:"plan_no"`
	OrgID           string     `json:"org_id"`
	Name            string     `json:"name"`
	ScopeLocationID *string    `json:"scope_location_id,omitempty"`
	ScopeTypeID     *string    `json:"scope_type_id,omitempty"`
	Status          string     `json:"status"`
	CreatedBy       string     `json:"created_by"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// StocktakeItem 盘点明细领域模型
type StocktakeItem struct {
	ID                 string     `json:"id"`
	PlanID             string     `json:"plan_id"`
	AssetID            *string    `json:"asset_id,omitempty"`
	ExpectedLocationID *string    `json:"expected_location_id,omitempty"`
	ExpectedStatus     *string    `json:"expected_status,omitempty"`
	Result             string     `json:"result"`
	ActualLocationID   *string    `json:"actual_location_id,omitempty"`
	SurplusNote        *string    `json:"surplus_note,omitempty"`
	CheckedBy          *string    `json:"checked_by,omitempty"`
	CheckedAt          *time.Time `json:"checked_at,omitempty"`
	Notes              *string    `json:"notes,omitempty"`
}

// StocktakeFilter 盘点计划查询过滤条件
type StocktakeFilter struct {
	OrgID  string
	Status string
	Cursor string
	Limit  int
	// Wave 2 G9: 行级数据权限范围 (nil/零值 → 回退到 OrgID, 历史行为)
	Scope OrgScope
}

// StocktakeItemFilter 盘点明细查询过滤条件
type StocktakeItemFilter struct {
	PlanID string
	Result string
	Search string
}

// stocktakeCursorData 游标编码数据
type stocktakeCursorData struct {
	CreatedAt time.Time `json:"a"`
	ID        string    `json:"i"`
}

func encodeStocktakeCursor(createdAt time.Time, id string) string {
	data, _ := json.Marshal(stocktakeCursorData{CreatedAt: createdAt, ID: id})
	return base64.URLEncoding.EncodeToString(data)
}

func decodeStocktakeCursor(c string) (*stocktakeCursorData, error) {
	decoded, err := base64.URLEncoding.DecodeString(c)
	if err != nil {
		return nil, err
	}
	var d stocktakeCursorData
	if err := json.Unmarshal(decoded, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// CreatePlan 创建盘点计划
func (r *StocktakeRepo) CreatePlan(ctx context.Context, q DBTX, p *StocktakePlan) error {
	_, err := q.Exec(ctx,
		`INSERT INTO assets.stocktake_plans
		 (id, plan_no, org_id, name, scope_location_id, scope_type_id, status,
		  created_by, started_at, finished_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		p.ID, p.PlanNo, p.OrgID, p.Name, p.ScopeLocationID, p.ScopeTypeID,
		p.Status, p.CreatedBy, p.StartedAt, p.FinishedAt, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create stocktake plan: %w", err)
	}
	return nil
}

// GetPlan 获取单个盘点计划 (带 org_id 过滤)
func (r *StocktakeRepo) GetPlan(ctx context.Context, q DBTX, id string, orgID string) (*StocktakePlan, error) {
	var p StocktakePlan
	err := q.QueryRow(ctx,
		`SELECT id, plan_no, org_id, name, scope_location_id, scope_type_id,
		 status, created_by, started_at, finished_at, created_at, updated_at
		 FROM assets.stocktake_plans
		 WHERE id = $1 AND org_id = $2`, id, orgID,
	).Scan(&p.ID, &p.PlanNo, &p.OrgID, &p.Name,
		&p.ScopeLocationID, &p.ScopeTypeID, &p.Status, &p.CreatedBy,
		&p.StartedAt, &p.FinishedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("stocktake plan not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get stocktake plan: %w", err)
	}
	return &p, nil
}

// UpdatePlan 更新盘点计划 (动态 SET, 带 org_id)
func (r *StocktakeRepo) UpdatePlan(ctx context.Context, q DBTX, id string, orgID string, updates map[string]interface{}) error {
	setClause := ""
	args := []interface{}{}
	argIdx := 1

	fields := []struct {
		name string
		val  interface{}
	}{
		{"status", updates["status"]},
		{"started_at", updates["started_at"]},
		{"finished_at", updates["finished_at"]},
	}

	for _, f := range fields {
		if f.val != nil {
			if setClause != "" {
				setClause += ", "
			}
			setClause += fmt.Sprintf("%s = $%d", f.name, argIdx)
			args = append(args, f.val)
			argIdx++
		}
	}

	if setClause == "" {
		return fmt.Errorf("no fields to update")
	}

	setClause += fmt.Sprintf(", updated_at = $%d", argIdx)
	args = append(args, time.Now())
	argIdx++

	args = append(args, id, orgID)

	query := fmt.Sprintf(
		`UPDATE assets.stocktake_plans SET %s WHERE id = $%d AND org_id = $%d`,
		setClause, argIdx, argIdx+1,
	)

	tag, err := q.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update stocktake plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("stocktake plan not found")
	}
	return nil
}

// ListPlans 游标分页查询盘点计划
func (r *StocktakeRepo) ListPlans(ctx context.Context, q DBTX, f StocktakeFilter) ([]StocktakePlan, string, bool, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	// G9: 行级数据权限 — 优先 Scope, 否则回退 OrgID
	scope := f.Scope
	if scope.OrgID == "" {
		scope.OrgID = f.OrgID
	}
	orgClause, orgArgs := scope.Clause(1)

	query := `SELECT id, plan_no, org_id, name, scope_location_id, scope_type_id,
		status, created_by, started_at, finished_at, created_at, updated_at
		FROM assets.stocktake_plans WHERE ` + orgClause
	args := orgArgs
	argIdx := 1 + len(orgArgs)

	if f.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}

	if f.Cursor != "" {
		decoded, err := decodeStocktakeCursor(f.Cursor)
		if err == nil {
			query += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argIdx, argIdx+1)
			args = append(args, decoded.CreatedAt, decoded.ID)
			argIdx += 2
		}
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", argIdx)
	args = append(args, f.Limit+1)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("list stocktake plans: %w", err)
	}
	defer rows.Close()

	var plans []StocktakePlan
	for rows.Next() {
		var p StocktakePlan
		if err := rows.Scan(&p.ID, &p.PlanNo, &p.OrgID, &p.Name,
			&p.ScopeLocationID, &p.ScopeTypeID, &p.Status, &p.CreatedBy,
			&p.StartedAt, &p.FinishedAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, "", false, fmt.Errorf("scan stocktake plan: %w", err)
		}
		plans = append(plans, p)
	}

	hasMore := len(plans) > f.Limit
	if hasMore {
		plans = plans[:f.Limit]
	}

	var nextCursor string
	if hasMore && len(plans) > 0 {
		last := plans[len(plans)-1]
		nextCursor = encodeStocktakeCursor(last.CreatedAt, last.ID)
	}

	return plans, nextCursor, hasMore, nil
}

// CreateItems 批量插入盘点明细
func (r *StocktakeRepo) CreateItems(ctx context.Context, q DBTX, items []StocktakeItem) error {
	if len(items) == 0 {
		return nil
	}

	// Build batch INSERT
	query := `INSERT INTO assets.stocktake_items
		(id, plan_id, asset_id, expected_location_id, expected_status, result,
		 actual_location_id, surplus_note, checked_by, checked_at, notes)
		VALUES `
	args := []interface{}{}
	argIdx := 1

	valueRows := make([]string, len(items))
	for i, item := range items {
		valueRows[i] = fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			argIdx, argIdx+1, argIdx+2, argIdx+3, argIdx+4,
			argIdx+5, argIdx+6, argIdx+7, argIdx+8, argIdx+9, argIdx+10,
		)
		args = append(args, item.ID, item.PlanID, item.AssetID,
			item.ExpectedLocationID, item.ExpectedStatus, item.Result,
			item.ActualLocationID, item.SurplusNote, item.CheckedBy,
			item.CheckedAt, item.Notes)
		argIdx += 11
	}

	query += joinStrings(valueRows, ", ")

	_, err := q.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("batch insert stocktake items: %w", err)
	}
	return nil
}

// UpdateItem 更新盘点明细 (by plan_id + item_id)
func (r *StocktakeRepo) UpdateItem(ctx context.Context, q DBTX, planID string, itemID string, updates map[string]interface{}) error {
	setClause := ""
	args := []interface{}{}
	argIdx := 1

	fields := []struct {
		name string
		val  interface{}
	}{
		{"result", updates["result"]},
		{"actual_location_id", updates["actual_location_id"]},
		{"notes", updates["notes"]},
		{"checked_by", updates["checked_by"]},
		{"checked_at", updates["checked_at"]},
	}

	for _, f := range fields {
		if f.val != nil {
			if setClause != "" {
				setClause += ", "
			}
			setClause += fmt.Sprintf("%s = $%d", f.name, argIdx)
			args = append(args, f.val)
			argIdx++
		}
	}

	if setClause == "" {
		return fmt.Errorf("no fields to update")
	}

	args = append(args, itemID, planID)

	query := fmt.Sprintf(
		`UPDATE assets.stocktake_items SET %s WHERE id = $%d AND plan_id = $%d`,
		setClause, argIdx, argIdx+1,
	)

	tag, err := q.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update stocktake item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("stocktake item not found")
	}
	return nil
}

// ListItems 查询盘点明细 (by plan, optional filter)
func (r *StocktakeRepo) ListItems(ctx context.Context, q DBTX, f StocktakeItemFilter) ([]StocktakeItem, error) {
	query := `SELECT id, plan_id, asset_id, expected_location_id, expected_status,
		result, actual_location_id, surplus_note, checked_by, checked_at, notes
		FROM assets.stocktake_items WHERE plan_id = $1`
	args := []interface{}{f.PlanID}
	argIdx := 2

	if f.Result != "" {
		query += fmt.Sprintf(" AND result = $%d", argIdx)
		args = append(args, f.Result)
		argIdx++
	}

	if f.Search != "" {
		query += fmt.Sprintf(` AND (COALESCE(surplus_note,'') ILIKE $%d OR COALESCE(notes,'') ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}

	query += " ORDER BY result, id"

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stocktake items: %w", err)
	}
	defer rows.Close()

	var items []StocktakeItem
	for rows.Next() {
		var si StocktakeItem
		if err := rows.Scan(&si.ID, &si.PlanID, &si.AssetID,
			&si.ExpectedLocationID, &si.ExpectedStatus, &si.Result,
			&si.ActualLocationID, &si.SurplusNote,
			&si.CheckedBy, &si.CheckedAt, &si.Notes); err != nil {
			return nil, fmt.Errorf("scan stocktake item: %w", err)
		}
		items = append(items, si)
	}
	return items, nil
}

// GetItem 获取单个盘点明细
func (r *StocktakeRepo) GetItem(ctx context.Context, q DBTX, planID string, itemID string) (*StocktakeItem, error) {
	var si StocktakeItem
	err := q.QueryRow(ctx,
		`SELECT id, plan_id, asset_id, expected_location_id, expected_status,
		 result, actual_location_id, surplus_note, checked_by, checked_at, notes
		 FROM assets.stocktake_items
		 WHERE id = $1 AND plan_id = $2`, itemID, planID,
	).Scan(&si.ID, &si.PlanID, &si.AssetID,
		&si.ExpectedLocationID, &si.ExpectedStatus, &si.Result,
		&si.ActualLocationID, &si.SurplusNote,
		&si.CheckedBy, &si.CheckedAt, &si.Notes,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("stocktake item not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get stocktake item: %w", err)
	}
	return &si, nil
}

// GetPlanProgress 获取盘点进度汇总 (COUNT BY result)
func (r *StocktakeRepo) GetPlanProgress(ctx context.Context, q DBTX, planID string) (map[string]int64, int64, error) {
	rows, err := q.Query(ctx,
		`SELECT result, COUNT(*) FROM assets.stocktake_items
		 WHERE plan_id = $1 GROUP BY result`, planID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("get plan progress: %w", err)
	}
	defer rows.Close()

	progress := map[string]int64{
		"pending": 0,
		"found":   0,
		"missing": 0,
		"moved":   0,
		"surplus": 0,
	}
	var total int64

	for rows.Next() {
		var result string
		var count int64
		if err := rows.Scan(&result, &count); err != nil {
			return nil, 0, fmt.Errorf("scan progress: %w", err)
		}
		progress[result] = count
		total += count
	}

	return progress, total, nil
}

// joinStrings joins string slices with a separator
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
