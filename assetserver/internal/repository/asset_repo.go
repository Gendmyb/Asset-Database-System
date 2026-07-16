// Package repository — 资产数据访问层
// 对应架构文档 §8 并发控制 (乐观锁/悲观锁/Advisory锁)
package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

// AssetRepo 资产仓库
type AssetRepo struct {
	pool *pgxpool.Pool
}

func NewAssetRepo(pool *pgxpool.Pool) *AssetRepo {
	return &AssetRepo{pool: pool}
}

// List 游标分页查询
func (r *AssetRepo) List(ctx context.Context, orgID, search, typeID, status, cursor string, limit int) ([]AssetRow, string, bool, error) {
	query := `SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		lifecycle_state, status, properties, version, deleted_at, created_at, updated_at
		FROM assets.assets WHERE deleted_at IS NULL AND org_id = $1`
	args := []interface{}{orgID}
	argIdx := 2

	if search != "" {
		query += fmt.Sprintf(" AND (name ILIKE $%d OR asset_tag ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}
	if typeID != "" {
		query += fmt.Sprintf(" AND type_id = $%d", argIdx)
		args = append(args, typeID)
		argIdx++
	}
	if status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, status)
		argIdx++
	}

	// 游标解码
	if cursor != "" {
		decoded, err := decodeCursor(cursor)
		if err == nil {
			query += fmt.Sprintf(" AND (updated_at, id) < ($%d, $%d)", argIdx, argIdx+1)
			args = append(args, decoded.UpdatedAt, decoded.ID)
			argIdx += 2
		}
	}

	query += fmt.Sprintf(" ORDER BY updated_at DESC, id DESC LIMIT $%d", argIdx)
	args = append(args, limit+1)

	rows, err := r.pool.Query(ctx, query, args...)
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

	hasMore := len(assets) > limit
	if hasMore {
		assets = assets[:limit]
	}

	var nextCursor string
	if hasMore && len(assets) > 0 {
		last := assets[len(assets)-1]
		nextCursor = encodeCursor(last.UpdatedAt, last.ID)
	}

	return assets, nextCursor, hasMore, nil
}

// GetByID 获取单个资产
func (r *AssetRepo) GetByID(ctx context.Context, id string) (*AssetRow, error) {
	var a AssetRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at
		 FROM assets.assets WHERE id = $1 AND deleted_at IS NULL`, id,
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
func (r *AssetRepo) Create(ctx context.Context, a *AssetRow) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO assets.assets (id, asset_tag, name, type_id, org_id, serial_number,
		 manufacturer, model, lifecycle_state, status, properties, version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		a.ID, a.AssetTag, a.Name, a.TypeID, a.OrgID,
		a.SerialNumber, a.Manufacturer, a.Model,
		a.LifecycleState, a.Status, a.Properties, a.Version, a.CreatedAt, a.UpdatedAt,
	)
	return err
}

// UpdateWithRetry 乐观锁更新 (最多 3 次重试)
// 对应架构文档 §8.2 乐观锁
func (r *AssetRepo) UpdateWithRetry(ctx context.Context, id string, updates map[string]interface{}, expectedVersion int) (*AssetRow, error) {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		current, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if current.Version != expectedVersion {
			return nil, fmt.Errorf("version conflict: expected %d, got %d", expectedVersion, current.Version)
		}

		now := time.Now()
		tag, err := r.pool.Exec(ctx,
			`UPDATE assets.assets SET name=COALESCE($2,name), serial_number=COALESCE($3,serial_number),
			 manufacturer=COALESCE($4,manufacturer), model=COALESCE($5,model),
			 lifecycle_state=COALESCE($6,lifecycle_state), status=COALESCE($7,status),
			 properties=COALESCE($8,properties), version=version+1, updated_at=$9
			 WHERE id=$1 AND version=$10 AND deleted_at IS NULL`,
			id,
			updates["name"], updates["serial_number"], updates["manufacturer"],
			updates["model"], updates["lifecycle_state"], updates["status"],
			updates["properties"], now, expectedVersion,
		)
		if err != nil {
			return nil, fmt.Errorf("update asset: %w", err)
		}

		if tag.RowsAffected() == 0 {
			if attempt >= maxRetries {
				return nil, fmt.Errorf("max retries exceeded: version conflict")
			}
			// 重新读取当前版本并重试
			expectedVersion = current.Version
			continue
		}
		return r.GetByID(ctx, id)
	}
	return nil, fmt.Errorf("max retries exceeded")
}

// SoftDelete 软删除
func (r *AssetRepo) SoftDelete(ctx context.Context, id string) error {
	now := time.Now()
	tag, err := r.pool.Exec(ctx,
		`UPDATE assets.assets SET deleted_at=$1, updated_at=$1 WHERE id=$2 AND deleted_at IS NULL`,
		now, id,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("asset not found")
	}
	return nil
}

// LockForUpdate 悲观锁 — SELECT ... FOR UPDATE (5s 超时)
// 对应架构文档 §8.3 悲观锁
func (r *AssetRepo) LockForUpdate(ctx context.Context, id string) (*AssetRow, error) {
	// 设置锁超时
	if _, err := r.pool.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, fmt.Errorf("set lock_timeout: %w", err)
	}

	var a AssetRow
	err := r.pool.QueryRow(ctx,
		`SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at
		 FROM assets.assets WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id,
	).Scan(&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
		&a.SerialNumber, &a.Manufacturer, &a.Model,
		&a.LifecycleState, &a.Status, &a.Properties,
		&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("lock asset: %w", err)
	}
	return &a, nil
}

// LockAssetsSorted 按 UUID 字典序锁定多个资产 (死锁预防)
// 对应架构文档 §8.3 全局锁排序规范
func (r *AssetRepo) LockAssetsSorted(ctx context.Context, ids []string) ([]*AssetRow, error) {
	// 按 UUID 字典序排序
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	// 简化: 按字符串排序 (UUID 字典序)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if _, err := r.pool.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return nil, err
	}

	var assets []*AssetRow
	for _, id := range sorted {
		a, err := r.LockForUpdate(ctx, id)
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
