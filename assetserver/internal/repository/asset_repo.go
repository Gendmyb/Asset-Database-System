// Package repository — 资产数据访问层
// 对应架构文档 §8 并发控制 (乐观锁/悲观锁/Advisory锁)
package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// AssetRow 数据库行
type AssetRow struct {
	ID             string
	AssetTag       string
	Name           string
	TypeID         string
	OrgID          string
	SerialNumber   *string
	Manufacturer   *string
	Model          *string
	LifecycleState string
	Status         string
	Properties     []byte // JSONB
	Version        int
	DeletedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AssetRepo 资产仓库 (无状态 — DBTX 由调用方传入)
type AssetRepo struct{}

func NewAssetRepo() *AssetRepo {
	return &AssetRepo{}
}

// AssetFilter 查询过滤条件
type AssetFilter struct {
	OrgID        string
	Search       string // 全文搜索 (tsvector) 或 ILIKE fallback
	TypeID       string
	Status       string
	Lifecycle    string
	Manufacturer string
	Cursor       string
	Limit        int
}

// List 游标分页查询 (支持全文搜索 + 多条件过滤)
func (r *AssetRepo) List(ctx context.Context, q DBTX, f AssetFilter) ([]AssetRow, string, bool, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	query := `SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		lifecycle_state, status, properties, version, deleted_at, created_at, updated_at
		FROM assets.assets WHERE deleted_at IS NULL AND org_id = $1`
	args := []interface{}{f.OrgID}
	argIdx := 2

	// 全文搜索优先，回退到 ILIKE
	if f.Search != "" {
		// 检查是否是中文搜索
		if containsCJK(f.Search) {
			query += fmt.Sprintf(" AND (name ILIKE $%d OR asset_tag ILIKE $%d OR COALESCE(manufacturer,'') ILIKE $%d OR COALESCE(model,'') ILIKE $%d OR COALESCE(serial_number,'') ILIKE $%d)", argIdx, argIdx, argIdx, argIdx, argIdx)
			args = append(args, "%"+f.Search+"%")
			argIdx++
		} else {
			// 英文使用 PostgreSQL 全文搜索
			query += fmt.Sprintf(" AND to_tsvector('english', name || ' ' || COALESCE(manufacturer,'') || ' ' || COALESCE(model,'') || ' ' || COALESCE(serial_number,'') || ' ' || asset_tag) @@ plainto_tsquery('english', $%d)", argIdx)
			args = append(args, f.Search)
			argIdx++
		}
	}

	if f.TypeID != "" {
		query += fmt.Sprintf(" AND type_id = $%d", argIdx)
		args = append(args, f.TypeID)
		argIdx++
	}
	if f.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.Lifecycle != "" {
		query += fmt.Sprintf(" AND lifecycle_state = $%d", argIdx)
		args = append(args, f.Lifecycle)
		argIdx++
	}
	if f.Manufacturer != "" {
		query += fmt.Sprintf(" AND manufacturer ILIKE $%d", argIdx)
		args = append(args, "%"+f.Manufacturer+"%")
		argIdx++
	}

	// 游标解码
	if f.Cursor != "" {
		decoded, err := decodeCursor(f.Cursor)
		if err == nil {
			query += fmt.Sprintf(" AND (updated_at, id) < ($%d, $%d)", argIdx, argIdx+1)
			args = append(args, decoded.UpdatedAt, decoded.ID)
			argIdx += 2
		}
	}

	query += fmt.Sprintf(" ORDER BY updated_at DESC, id DESC LIMIT $%d", argIdx)
	args = append(args, f.Limit+1)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("list assets: %w", err)
	}
	defer rows.Close()

	var assets []AssetRow
	for rows.Next() {
		var a AssetRow
		if err := rows.Scan(&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
			&a.SerialNumber, &a.Manufacturer, &a.Model,
			&a.LifecycleState, &a.Status, &a.Properties,
			&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, "", false, fmt.Errorf("scan asset: %w", err)
		}
		assets = append(assets, a)
	}

	hasMore := len(assets) > f.Limit
	if hasMore {
		assets = assets[:f.Limit]
	}

	var nextCursor string
	if hasMore && len(assets) > 0 {
		last := assets[len(assets)-1]
		nextCursor = encodeCursor(last.UpdatedAt, last.ID)
	}

	return assets, nextCursor, hasMore, nil
}

// containsCJK 检测字符串是否包含中日韩字符
func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified
			(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0xAC00 && r <= 0xD7AF) { // Hangul
			return true
		}
	}
	return false
}

// GetByID 获取单个资产 (含 org_id 过滤防止 IDOR)
func (r *AssetRepo) GetByID(ctx context.Context, q DBTX, id string, orgID string) (*AssetRow, error) {
	var a AssetRow
	err := q.QueryRow(ctx,
		`SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at
		 FROM assets.assets WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID,
	).Scan(&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
		&a.SerialNumber, &a.Manufacturer, &a.Model,
		&a.LifecycleState, &a.Status, &a.Properties,
		&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("asset not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get asset: %w", err)
	}
	return &a, nil
}

// Create 创建资产
func (r *AssetRepo) Create(ctx context.Context, q DBTX, a *AssetRow) error {
	_, err := q.Exec(ctx,
		`INSERT INTO assets.assets (id, asset_tag, name, type_id, org_id, serial_number,
		 manufacturer, model, lifecycle_state, status, properties, version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		a.ID, a.AssetTag, a.Name, a.TypeID, a.OrgID,
		a.SerialNumber, a.Manufacturer, a.Model,
		a.LifecycleState, a.Status, a.Properties, a.Version, a.CreatedAt, a.UpdatedAt,
	)
	return err
}

// UpdateWithRetry 乐观锁更新 (最多 3 次重试, 含 org_id 过滤防止 IDOR)
func (r *AssetRepo) UpdateWithRetry(ctx context.Context, q DBTX, id string, orgID string, updates map[string]interface{}, expectedVersion int) (*AssetRow, error) {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		current, err := r.GetByID(ctx, q, id, orgID)
		if err != nil {
			return nil, err
		}
		if current.Version != expectedVersion {
			return nil, fmt.Errorf("version conflict: expected %d, got %d", expectedVersion, current.Version)
		}

		now := time.Now()
		tag, err := q.Exec(ctx,
			`UPDATE assets.assets SET name=COALESCE($2,name), serial_number=COALESCE($3,serial_number),
			 manufacturer=COALESCE($4,manufacturer), model=COALESCE($5,model),
			 lifecycle_state=COALESCE($6,lifecycle_state), status=COALESCE($7,status),
			 properties=COALESCE($8,properties), version=version+1, updated_at=$9
			 WHERE id=$1 AND org_id=$11 AND version=$10 AND deleted_at IS NULL`,
			id,
			updates["name"], updates["serial_number"], updates["manufacturer"],
			updates["model"], updates["lifecycle_state"], updates["status"],
			updates["properties"], now, expectedVersion, orgID,
		)
		if err != nil {
			return nil, fmt.Errorf("update asset: %w", err)
		}

		if tag.RowsAffected() == 0 {
			if attempt >= maxRetries {
				return nil, fmt.Errorf("max retries exceeded: version conflict")
			}
			expectedVersion = current.Version
			continue
		}
		return r.GetByID(ctx, q, id, orgID)
	}
	return nil, fmt.Errorf("max retries exceeded")
}

// SoftDelete 软删除 (含 org_id 过滤防止 IDOR)
func (r *AssetRepo) SoftDelete(ctx context.Context, q DBTX, id string, orgID string) error {
	now := time.Now()
	tag, err := q.Exec(ctx,
		`UPDATE assets.assets SET deleted_at=$1, updated_at=$1 WHERE id=$2 AND org_id=$3 AND deleted_at IS NULL`,
		now, id, orgID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("asset not found")
	}
	return nil
}

// LockForUpdate 悲观锁 — SELECT ... FOR UPDATE (5s 超时, 含 org_id 过滤防止 IDOR)
func (r *AssetRepo) LockForUpdate(ctx context.Context, q DBTX, id string, orgID string) (*AssetRow, error) {
	if _, err := q.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("set lock_timeout: %w", err)
	}

	var a AssetRow
	err := q.QueryRow(ctx,
		`SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at
		 FROM assets.assets WHERE id=$1 AND org_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, orgID,
	).Scan(&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
		&a.SerialNumber, &a.Manufacturer, &a.Model,
		&a.LifecycleState, &a.Status, &a.Properties,
		&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("lock asset: %w", err)
	}
	return &a, nil
}

// LockAssetsSorted 按 UUID 字典序锁定多个资产 (死锁预防, 含 org_id 过滤)
func (r *AssetRepo) LockAssetsSorted(ctx context.Context, q DBTX, ids []string, orgID string) ([]*AssetRow, error) {
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if _, err := q.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, err
	}

	var assets []*AssetRow
	for _, id := range sorted {
		a, err := r.LockForUpdate(ctx, q, id, orgID)
		if err != nil {
			return nil, err
		}
		assets = append(assets, a)
	}
	return assets, nil
}

// cursor 编解码
type cursorData struct {
	UpdatedAt time.Time `json:"u"`
	ID        string    `json:"i"`
}

func encodeCursor(updatedAt time.Time, id string) string {
	data, _ := json.Marshal(cursorData{UpdatedAt: updatedAt, ID: id})
	return base64.URLEncoding.EncodeToString(data)
}

func decodeCursor(c string) (*cursorData, error) {
	decoded, err := base64.URLEncoding.DecodeString(c)
	if err != nil {
		return nil, err
	}
	var d cursorData
	if err := json.Unmarshal(decoded, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// 确保 strings 被使用 (containsCJK 用到 range)
var _ = strings.Contains
