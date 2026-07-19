// Package repository — 资产领用数据访问层 (PG)
package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/lock"
	"github.com/google/uuid"
)

// AssignmentRepo 领用管理仓库 (无状态 — DBTX 由调用方传入)
type AssignmentRepo struct {
	assetRepo *AssetRepo
}

func NewAssignmentRepo() *AssignmentRepo {
	return &AssignmentRepo{
		assetRepo: NewAssetRepo(),
	}
}

// ActiveAssignment 活跃领用记录
type ActiveAssignment struct {
	ID             string     `json:"id"`
	AssetID        string     `json:"asset_id"`
	AssignedTo     string     `json:"assigned_to"`
	AssignedBy     string     `json:"assigned_by"`
	AssignmentType string     `json:"assignment_type"`
	Notes          string     `json:"notes"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	AssignedAt     time.Time  `json:"assigned_at"`
}

// AssignmentRow 完整领用记录 (列表/查询用)
type AssignmentRow struct {
	ID             string     `json:"id"`
	AssetID        string     `json:"asset_id"`
	OrgID          string     `json:"org_id"`
	AssignedTo     string     `json:"assigned_to"`
	AssignedBy     string     `json:"assigned_by"`
	Status         string     `json:"status"`
	AssignmentType string     `json:"assignment_type"`
	Notes          string     `json:"notes"`
	ReturnNotes    *string    `json:"return_notes,omitempty"`
	DueDate        *time.Time `json:"due_date,omitempty"`
	AssignedAt     time.Time  `json:"assigned_at"`
	ReturnedAt     *time.Time `json:"returned_at,omitempty"`
	Version        int        `json:"version"`
	// JOIN 解析的展示字段 (避免前端显示 UUID)
	AssetName      string `json:"asset_name"`
	AssetTag       string `json:"asset_tag"`
	AssignedToName string `json:"assigned_to_name"`
}

// AssignmentFilter 领用查询过滤条件
type AssignmentFilter struct {
	OrgID      string
	Status     string // active, returned
	Type       string // permanent, temporary
	AssignedTo string // user UUID
	Overdue    bool
	Cursor     string
	Limit      int
}

// Assign 领用资产: 悲观锁 + 写入 assignments 表 + 更新资产状态
func (r *AssignmentRepo) Assign(ctx context.Context, q DBTX, assetID, orgID, assignedTo, assignedBy, notes string) (string, error) {
	asset, err := r.assetRepo.LockForUpdate(ctx, q, assetID, orgID)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}
	if asset.Status != "available" {
		return "", fmt.Errorf("asset is %s, cannot assign", asset.Status)
	}

	assignmentID := uuid.New().String()
	now := time.Now()
	_, err = q.Exec(ctx,
		`INSERT INTO assets.assignments (id, asset_id, org_id, assigned_to, assigned_by, status, assignment_type, notes, assigned_at, version)
		 VALUES ($1,$2,$3,$4,$5,'active','permanent',$6,$7,1)`,
		assignmentID, assetID, orgID, assignedTo, assignedBy, notes, now)
	if err != nil {
		return "", fmt.Errorf("create assignment: %w", err)
	}

	_, err = q.Exec(ctx,
		`UPDATE assets.assets SET status='assigned', version=version+1, updated_at=$1
		 WHERE id=$2 AND deleted_at IS NULL`, now, assetID)
	if err != nil {
		return "", fmt.Errorf("update asset status: %w", err)
	}

	return assignmentID, nil
}

// Borrow 借用资产: 悲观锁 + 写入 assignments(type=temporary) + 更新资产状态为 borrowed
func (r *AssignmentRepo) Borrow(ctx context.Context, q DBTX, assetID, orgID, assignedTo, assignedBy string, dueDate time.Time, notes string) (string, error) {
	asset, err := r.assetRepo.LockForUpdate(ctx, q, assetID, orgID)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}
	if asset.Status != "available" {
		return "", fmt.Errorf("asset is %s, cannot borrow", asset.Status)
	}

	assignmentID := uuid.New().String()
	now := time.Now()
	_, err = q.Exec(ctx,
		`INSERT INTO assets.assignments (id, asset_id, org_id, assigned_to, assigned_by, status, assignment_type, due_date, notes, assigned_at, version)
		 VALUES ($1,$2,$3,$4,$5,'active','temporary',$6,$7,$8,1)`,
		assignmentID, assetID, orgID, assignedTo, assignedBy, dueDate, notes, now)
	if err != nil {
		return "", fmt.Errorf("create borrow assignment: %w", err)
	}

	_, err = q.Exec(ctx,
		`UPDATE assets.assets SET status='borrowed', version=version+1, updated_at=$1
		 WHERE id=$2 AND deleted_at IS NULL`, now, assetID)
	if err != nil {
		return "", fmt.Errorf("update asset status: %w", err)
	}

	return assignmentID, nil
}

// Release 归还资产: 关闭 assignment + 恢复资产状态 (含归还备注)
func (r *AssignmentRepo) Release(ctx context.Context, q DBTX, assetID string, orgID string, returnNotes string) error {
	_, err := r.assetRepo.LockForUpdate(ctx, q, assetID, orgID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}

	now := time.Now()

	tag, err := q.Exec(ctx,
		`UPDATE assets.assignments SET status='returned', return_notes=$2, returned_at=$3
		 WHERE asset_id=$1 AND status='active'`, assetID, returnNotes, now)
	if err != nil {
		return fmt.Errorf("close assignment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no active assignment found for asset %s", assetID)
	}

	_, err = q.Exec(ctx,
		`UPDATE assets.assets SET status='available', version=version+1, updated_at=$1
		 WHERE id=$2 AND deleted_at IS NULL`, now, assetID)
	if err != nil {
		return fmt.Errorf("update asset status: %w", err)
	}

	return nil
}

// Transfer 转移资产: 字典序锁定防止死锁 (含 org_id 过滤防止 IDOR)
// Phase E: 检查借用资产不可转移 + 检查 RowsAffected
func (r *AssignmentRepo) Transfer(ctx context.Context, q DBTX, assetID, orgID, toUserID, userID string) error {
	ids := lock.SortedAssetIDs([]string{assetID})
	if err := lock.ValidateSortedOrder(ids); err != nil {
		return err
	}

	_, err := r.assetRepo.LockAssetsSorted(ctx, q, ids, orgID)
	if err != nil {
		return fmt.Errorf("lock asset: %w", err)
	}

	// Phase E: 检查当前活跃领用是否为临时借用
	var assignmentType string
	err = q.QueryRow(ctx,
		`SELECT assignment_type FROM assets.assignments
		 WHERE asset_id=$1 AND status='active' LIMIT 1`, assetID,
	).Scan(&assignmentType)
	if err != nil {
		return fmt.Errorf("no active assignment found for asset %s", assetID)
	}
	if assignmentType == "temporary" {
		return fmt.Errorf("借用中的资产不可转移，请先归还")
	}

	now := time.Now()

	tag, err := q.Exec(ctx,
		`UPDATE assets.assignments SET status='transferred', returned_at=$1
		 WHERE asset_id=$2 AND status='active'`, now, assetID)
	if err != nil {
		return fmt.Errorf("close old assignment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no active assignment found for asset %s", assetID)
	}

	_, err = q.Exec(ctx,
		`INSERT INTO assets.assignments (id, asset_id, org_id, assigned_to, assigned_by, status, assignment_type, assigned_at, version)
		 VALUES ($1,$2,$3,$4,$5,'active','permanent',NOW(),1)`,
		uuid.New().String(), assetID, orgID, toUserID, userID)
	if err != nil {
		return fmt.Errorf("create new assignment: %w", err)
	}

	return nil
}

// GetActiveAssignment 获取资产的活跃领用记录
func (r *AssignmentRepo) GetActiveAssignment(ctx context.Context, q DBTX, assetID string) (*ActiveAssignment, error) {
	var a ActiveAssignment
	err := q.QueryRow(ctx,
		`SELECT id, asset_id, assigned_to, assigned_by, assignment_type, COALESCE(notes,''), due_date, assigned_at
		 FROM assets.assignments WHERE asset_id = $1 AND status = 'active'
		 ORDER BY assigned_at DESC LIMIT 1`, assetID,
	).Scan(&a.ID, &a.AssetID, &a.AssignedTo, &a.AssignedBy, &a.AssignmentType, &a.Notes, &a.DueDate, &a.AssignedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAssignments 列表查询领用记录 (支持分页 + 过滤)
func (r *AssignmentRepo) ListAssignments(ctx context.Context, q DBTX, f AssignmentFilter) ([]AssignmentRow, string, bool, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	query := `SELECT asn.id, asn.asset_id, asn.org_id, asn.assigned_to, asn.assigned_by, asn.status,
		asn.assignment_type, COALESCE(asn.notes,''), asn.return_notes, asn.due_date, asn.assigned_at, asn.returned_at, asn.version,
		COALESCE(a.name,''), COALESCE(a.asset_tag,''), COALESCE(u.username,'')
		FROM assets.assignments asn
		LEFT JOIN assets.assets a ON a.id = asn.asset_id
		LEFT JOIN assets.users u ON u.id = asn.assigned_to
		WHERE asn.org_id = $1`
	args := []interface{}{f.OrgID}
	argIdx := 2

	if f.Status != "" {
		query += fmt.Sprintf(" AND asn.status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.Type != "" {
		query += fmt.Sprintf(" AND asn.assignment_type = $%d", argIdx)
		args = append(args, f.Type)
		argIdx++
	}
	if f.AssignedTo != "" {
		query += fmt.Sprintf(" AND asn.assigned_to = $%d", argIdx)
		args = append(args, f.AssignedTo)
		argIdx++
	}
	if f.Overdue {
		query += " AND asn.status = 'active' AND asn.assignment_type = 'temporary' AND asn.due_date < CURRENT_DATE"
	}

	// 游标分页
	if f.Cursor != "" {
		decoded, err := decodeAssignmentCursor(f.Cursor)
		if err == nil {
			query += fmt.Sprintf(" AND (assigned_at, id) < ($%d, $%d)", argIdx, argIdx+1)
			args = append(args, decoded.AssignedAt, decoded.ID)
			argIdx += 2
		}
	}

	query += fmt.Sprintf(" ORDER BY asn.assigned_at DESC, asn.id DESC LIMIT $%d", argIdx)
	args = append(args, f.Limit+1)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("list assignments: %w", err)
	}
	defer rows.Close()

	var assignments []AssignmentRow
	for rows.Next() {
		var a AssignmentRow
		if err := rows.Scan(&a.ID, &a.AssetID, &a.OrgID, &a.AssignedTo, &a.AssignedBy,
			&a.Status, &a.AssignmentType, &a.Notes, &a.ReturnNotes, &a.DueDate,
			&a.AssignedAt, &a.ReturnedAt, &a.Version,
			&a.AssetName, &a.AssetTag, &a.AssignedToName); err != nil {
			return nil, "", false, fmt.Errorf("scan assignment: %w", err)
		}
		assignments = append(assignments, a)
	}

	hasMore := len(assignments) > f.Limit
	if hasMore {
		assignments = assignments[:f.Limit]
	}

	var nextCursor string
	if hasMore && len(assignments) > 0 {
		last := assignments[len(assignments)-1]
		nextCursor = encodeAssignmentCursor(last.AssignedAt, last.ID)
	}

	return assignments, nextCursor, hasMore, nil
}

// assignmentCursorData 领用游标
type assignmentCursorData struct {
	AssignedAt time.Time `json:"a"`
	ID         string    `json:"i"`
}

func encodeAssignmentCursor(assignedAt time.Time, id string) string {
	data, _ := json.Marshal(assignmentCursorData{AssignedAt: assignedAt, ID: id})
	return base64.URLEncoding.EncodeToString(data)
}

func decodeAssignmentCursor(c string) (*assignmentCursorData, error) {
	decoded, err := base64.URLEncoding.DecodeString(c)
	if err != nil {
		return nil, err
	}
	var d assignmentCursorData
	if err := json.Unmarshal(decoded, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
