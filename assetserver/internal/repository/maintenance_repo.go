// Package repository — 维修/保养工单数据访问层 (PG)
// Phase F: 维修/保养工单+报废
package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// MaintenanceRepo 维修/保养工单仓库 (无状态 — DBTX 由调用方传入)
type MaintenanceRepo struct {
	assetRepo *AssetRepo
}

func NewMaintenanceRepo() *MaintenanceRepo {
	return &MaintenanceRepo{
		assetRepo: NewAssetRepo(),
	}
}

// MaintenanceOrder 工单领域模型
type MaintenanceOrder struct {
	ID          string     `json:"id"`
	OrderNo     string     `json:"order_no"`
	AssetID     string     `json:"asset_id"`
	OrgID       string     `json:"org_id"`
	Category    string     `json:"category"`
	Status      string     `json:"status"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	ReportedBy  string     `json:"reported_by"`
	Assignee    *string    `json:"assignee,omitempty"`
	Vendor      *string    `json:"vendor,omitempty"`
	Cost        *float64   `json:"cost,omitempty"`
	Resolution  *string    `json:"resolution,omitempty"`
	PrevStatus  string     `json:"prev_status"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Version     int        `json:"version"`
}

// MaintenanceFilter 工单查询过滤条件
type MaintenanceFilter struct {
	OrgID    string
	Status   string // open, in_progress, completed, canceled
	Category string // repair, upkeep
	AssetID  string
	Cursor   string
	Limit    int
}

// maintenanceCursorData 游标数据
type maintenanceCursorData struct {
	CreatedAt time.Time `json:"a"`
	ID        string    `json:"i"`
}

func encodeMaintenanceCursor(createdAt time.Time, id string) string {
	data, _ := json.Marshal(maintenanceCursorData{CreatedAt: createdAt, ID: id})
	return base64.URLEncoding.EncodeToString(data)
}

func decodeMaintenanceCursor(c string) (*maintenanceCursorData, error) {
	decoded, err := base64.URLEncoding.DecodeString(c)
	if err != nil {
		return nil, err
	}
	var d maintenanceCursorData
	if err := json.Unmarshal(decoded, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// CreateMaintenanceOrder 创建工单
func (r *MaintenanceRepo) CreateMaintenanceOrder(ctx context.Context, q DBTX, mo *MaintenanceOrder) error {
	_, err := q.Exec(ctx,
		`INSERT INTO assets.maintenance_orders
		 (id, order_no, asset_id, org_id, category, status, title, description,
		  reported_by, assignee, vendor, cost, resolution, prev_status,
		  started_at, finished_at, created_at, updated_at, version)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`,
		mo.ID, mo.OrderNo, mo.AssetID, mo.OrgID, mo.Category, mo.Status,
		mo.Title, mo.Description, mo.ReportedBy, mo.Assignee, mo.Vendor,
		mo.Cost, mo.Resolution, mo.PrevStatus,
		mo.StartedAt, mo.FinishedAt, mo.CreatedAt, mo.UpdatedAt, mo.Version,
	)
	if err != nil {
		return fmt.Errorf("create maintenance order: %w", err)
	}
	return nil
}

// GetMaintenanceOrder 获取单个工单 (带 org_id 过滤)
func (r *MaintenanceRepo) GetMaintenanceOrder(ctx context.Context, q DBTX, id string, orgID string) (*MaintenanceOrder, error) {
	var mo MaintenanceOrder
	err := q.QueryRow(ctx,
		`SELECT id, order_no, asset_id, org_id, category, status,
		 title, description, reported_by, assignee, vendor, cost, resolution,
		 prev_status, started_at, finished_at, created_at, updated_at, version
		 FROM assets.maintenance_orders
		 WHERE id = $1 AND org_id = $2`, id, orgID,
	).Scan(&mo.ID, &mo.OrderNo, &mo.AssetID, &mo.OrgID, &mo.Category, &mo.Status,
		&mo.Title, &mo.Description, &mo.ReportedBy, &mo.Assignee, &mo.Vendor,
		&mo.Cost, &mo.Resolution, &mo.PrevStatus,
		&mo.StartedAt, &mo.FinishedAt, &mo.CreatedAt, &mo.UpdatedAt, &mo.Version,
	)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("maintenance order not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get maintenance order: %w", err)
	}
	return &mo, nil
}

// HasActiveOrder 检查资产是否有活跃工单
func (r *MaintenanceRepo) HasActiveOrder(ctx context.Context, q DBTX, assetID string) (bool, error) {
	var count int
	err := q.QueryRow(ctx,
		`SELECT COUNT(*) FROM assets.maintenance_orders
		 WHERE asset_id = $1 AND status IN ('open','in_progress')`, assetID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check active order: %w", err)
	}
	return count > 0, nil
}

// UpdateMaintenanceOrder 更新工单 (带 org_id 过滤)
func (r *MaintenanceRepo) UpdateMaintenanceOrder(ctx context.Context, q DBTX, id string, orgID string, updates map[string]interface{}) error {
	// 构建动态 SET 子句
	setClause := ""
	args := []interface{}{}
	argIdx := 1

	// 处理各字段
	fields := []struct {
		name string
		val  interface{}
	}{
		{"status", updates["status"]},
		{"title", updates["title"]},
		{"description", updates["description"]},
		{"assignee", updates["assignee"]},
		{"vendor", updates["vendor"]},
		{"cost", updates["cost"]},
		{"resolution", updates["resolution"]},
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

	// version bump
	setClause += fmt.Sprintf(", version = version + 1, updated_at = $%d", argIdx)
	args = append(args, time.Now())
	argIdx++

	// id and org_id
	args = append(args, id, orgID)

	query := fmt.Sprintf(
		`UPDATE assets.maintenance_orders SET %s WHERE id = $%d AND org_id = $%d`,
		setClause, argIdx, argIdx+1,
	)

	tag, err := q.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update maintenance order: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("maintenance order not found")
	}
	return nil
}

// ListMaintenanceOrders 游标分页查询工单
func (r *MaintenanceRepo) ListMaintenanceOrders(ctx context.Context, q DBTX, f MaintenanceFilter) ([]MaintenanceOrder, string, bool, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	query := `SELECT id, order_no, asset_id, org_id, category, status,
		title, description, reported_by, assignee, vendor, cost, resolution,
		prev_status, started_at, finished_at, created_at, updated_at, version
		FROM assets.maintenance_orders WHERE org_id = $1`
	args := []interface{}{f.OrgID}
	argIdx := 2

	if f.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.Category != "" {
		query += fmt.Sprintf(" AND category = $%d", argIdx)
		args = append(args, f.Category)
		argIdx++
	}
	if f.AssetID != "" {
		query += fmt.Sprintf(" AND asset_id = $%d", argIdx)
		args = append(args, f.AssetID)
		argIdx++
	}

	// 游标分页
	if f.Cursor != "" {
		decoded, err := decodeMaintenanceCursor(f.Cursor)
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
		return nil, "", false, fmt.Errorf("list maintenance orders: %w", err)
	}
	defer rows.Close()

	var orders []MaintenanceOrder
	for rows.Next() {
		var mo MaintenanceOrder
		if err := rows.Scan(&mo.ID, &mo.OrderNo, &mo.AssetID, &mo.OrgID,
			&mo.Category, &mo.Status, &mo.Title, &mo.Description,
			&mo.ReportedBy, &mo.Assignee, &mo.Vendor, &mo.Cost, &mo.Resolution,
			&mo.PrevStatus, &mo.StartedAt, &mo.FinishedAt,
			&mo.CreatedAt, &mo.UpdatedAt, &mo.Version); err != nil {
			return nil, "", false, fmt.Errorf("scan maintenance order: %w", err)
		}
		orders = append(orders, mo)
	}

	hasMore := len(orders) > f.Limit
	if hasMore {
		orders = orders[:f.Limit]
	}

	var nextCursor string
	if hasMore && len(orders) > 0 {
		last := orders[len(orders)-1]
		nextCursor = encodeMaintenanceCursor(last.CreatedAt, last.ID)
	}

	return orders, nextCursor, hasMore, nil
}
