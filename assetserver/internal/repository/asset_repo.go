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
	// Phase E: 采购/折旧/报废字段
	PurchasePrice      *float64
	PurchaseDate       *time.Time
	Supplier           *string
	WarrantyUntil      *time.Time
	DepreciationMethod string
	UsefulLifeMonths   *int
	SalvageValue       float64
	ManagedBy          *string
	RetiredAt          *time.Time
	RetireReason       *string
	// Wave 2 G8: 外设挂载 — 父资产 ID (NULL 表示无父资产)
	ParentAssetID *string
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
	// Wave 2 G9: 行级数据权限范围。
	// nil 或零值 (Mode=ScopeOrg) 时回退到 OrgID + "org_id = $N" (历史行为)。
	Scope OrgScope
}

// List 游标分页查询 (支持全文搜索 + 多条件过滤)
func (r *AssetRepo) List(ctx context.Context, q DBTX, f AssetFilter) ([]AssetRow, string, bool, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}

	// G9: 行级数据权限 — 优先使用 Scope (部门级可见范围), 否则回退到 OrgID (组织级, 历史行为)
	scope := f.Scope
	if scope.OrgID == "" {
		scope.OrgID = f.OrgID
	}
	orgClause, orgArgs := scope.Clause(1)

	query := `SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		lifecycle_state, status, properties, version, deleted_at, created_at, updated_at,
		purchase_price, purchase_date, supplier, warranty_until, depreciation_method,
		useful_life_months, salvage_value, managed_by, retired_at, retire_reason, parent_asset_id
		FROM assets.assets WHERE deleted_at IS NULL AND ` + orgClause
	args := orgArgs
	argIdx := 1 + len(orgArgs)

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
			&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt,
			&a.PurchasePrice, &a.PurchaseDate, &a.Supplier, &a.WarrantyUntil,
			&a.DepreciationMethod, &a.UsefulLifeMonths, &a.SalvageValue,
			&a.ManagedBy, &a.RetiredAt, &a.RetireReason, &a.ParentAssetID); err != nil {
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
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at,
		 purchase_price, purchase_date, supplier, warranty_until, depreciation_method,
		 useful_life_months, salvage_value, managed_by, retired_at, retire_reason, parent_asset_id
		 FROM assets.assets WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, id, orgID,
	).Scan(&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
		&a.SerialNumber, &a.Manufacturer, &a.Model,
		&a.LifecycleState, &a.Status, &a.Properties,
		&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt,
		&a.PurchasePrice, &a.PurchaseDate, &a.Supplier, &a.WarrantyUntil,
		&a.DepreciationMethod, &a.UsefulLifeMonths, &a.SalvageValue,
		&a.ManagedBy, &a.RetiredAt, &a.RetireReason, &a.ParentAssetID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("asset not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get asset: %w", err)
	}
	return &a, nil
}

// GetByIDScoped 按 OrgScope 过滤获取单个资产 (G9 部门级可见范围)。
// scope.Mode=ScopeOrg 时等价于 GetByID(ctx, q, id, scope.OrgID)。
func (r *AssetRepo) GetByIDScoped(ctx context.Context, q DBTX, id string, scope OrgScope) (*AssetRow, error) {
	orgClause, orgArgs := scope.Clause(2)
	query := `SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at,
		 purchase_price, purchase_date, supplier, warranty_until, depreciation_method,
		 useful_life_months, salvage_value, managed_by, retired_at, retire_reason, parent_asset_id
		 FROM assets.assets WHERE id = $1 AND ` + orgClause + ` AND deleted_at IS NULL`

	var a AssetRow
	args := append([]interface{}{id}, orgArgs...)
	err := q.QueryRow(ctx, query, args...).Scan(
		&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
		&a.SerialNumber, &a.Manufacturer, &a.Model,
		&a.LifecycleState, &a.Status, &a.Properties,
		&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt,
		&a.PurchasePrice, &a.PurchaseDate, &a.Supplier, &a.WarrantyUntil,
		&a.DepreciationMethod, &a.UsefulLifeMonths, &a.SalvageValue,
		&a.ManagedBy, &a.RetiredAt, &a.RetireReason, &a.ParentAssetID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("asset not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get asset scoped: %w", err)
	}
	return &a, nil
}

// GetChildren 获取某资产的直接子资产 (外设列表), 按 OrgScope 过滤防 IDOR。
func (r *AssetRepo) GetChildren(ctx context.Context, q DBTX, parentID string, scope OrgScope) ([]AssetRow, error) {
	orgClause, orgArgs := scope.Clause(2)
	query := `SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at,
		 purchase_price, purchase_date, supplier, warranty_until, depreciation_method,
		 useful_life_months, salvage_value, managed_by, retired_at, retire_reason, parent_asset_id
		 FROM assets.assets WHERE parent_asset_id = $1 AND ` + orgClause + ` AND deleted_at IS NULL
		 ORDER BY created_at ASC`
	args := append([]interface{}{parentID}, orgArgs...)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get children: %w", err)
	}
	defer rows.Close()

	var out []AssetRow
	for rows.Next() {
		var a AssetRow
		if err := rows.Scan(&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
			&a.SerialNumber, &a.Manufacturer, &a.Model,
			&a.LifecycleState, &a.Status, &a.Properties,
			&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt,
			&a.PurchasePrice, &a.PurchaseDate, &a.Supplier, &a.WarrantyUntil,
			&a.DepreciationMethod, &a.UsefulLifeMonths, &a.SalvageValue,
			&a.ManagedBy, &a.RetiredAt, &a.RetireReason, &a.ParentAssetID); err != nil {
			return nil, fmt.Errorf("scan child: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// IsDescendant 检查 candidateID 是否是 ancestorID 的后代 (含自身), 用于 G8 防循环引用。
// 沿 parent_asset_id 链向上遍历, 最多 64 层 (防恶意深链)。
// 返回 true 表示 candidate 是 ancestor 的后代 (含自身), 此时禁止把 ancestor 挂到 candidate 下。
func (r *AssetRepo) IsDescendant(ctx context.Context, q DBTX, ancestorID, candidateID, orgID string) (bool, error) {
	if ancestorID == candidateID {
		return true, nil
	}
	const maxDepth = 64
	cur := candidateID
	for i := 0; i < maxDepth; i++ {
		if cur == "" {
			return false, nil
		}
		var parent *string
		err := q.QueryRow(ctx,
			`SELECT parent_asset_id FROM assets.assets
			 WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`, cur, orgID,
		).Scan(&parent)
		if err == pgx.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("walk parent chain: %w", err)
		}
		if parent == nil || *parent == "" {
			return false, nil
		}
		if *parent == ancestorID {
			return true, nil
		}
		cur = *parent
	}
	// 超过最大深度, 视为后代 (拒绝, 防御性)
	return true, nil
}

// SetParent 设置/清除资产的父资产 (G8 挂载/卸载), 含 org_id 过滤防 IDOR。
// parentID 为空串表示卸载 (置 NULL)。乐观锁: 需匹配 expectedVersion。
func (r *AssetRepo) SetParent(ctx context.Context, q DBTX, id, orgID, parentID string, expectedVersion int) (*AssetRow, error) {
	var arg interface{}
	if parentID == "" {
		arg = nil
	} else {
		arg = parentID
	}
	now := time.Now()
	tag, err := q.Exec(ctx,
		`UPDATE assets.assets SET parent_asset_id = $2, version = version + 1, updated_at = $3
		 WHERE id = $1 AND org_id = $4 AND version = $5 AND deleted_at IS NULL`,
		id, arg, now, orgID, expectedVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("set parent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("version conflict or asset not found")
	}
	return r.GetByID(ctx, q, id, orgID)
}
func (r *AssetRepo) GetByTag(ctx context.Context, q DBTX, tag string, orgID string) (*AssetRow, error) {
	var a AssetRow
	err := q.QueryRow(ctx,
		`SELECT id, asset_tag, name, type_id, org_id, serial_number, manufacturer, model,
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at,
		 purchase_price, purchase_date, supplier, warranty_until, depreciation_method,
		 useful_life_months, salvage_value, managed_by, retired_at, retire_reason, parent_asset_id
		 FROM assets.assets WHERE asset_tag = $1 AND org_id = $2 AND deleted_at IS NULL`, tag, orgID,
	).Scan(&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
		&a.SerialNumber, &a.Manufacturer, &a.Model,
		&a.LifecycleState, &a.Status, &a.Properties,
		&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt,
		&a.PurchasePrice, &a.PurchaseDate, &a.Supplier, &a.WarrantyUntil,
		&a.DepreciationMethod, &a.UsefulLifeMonths, &a.SalvageValue,
		&a.ManagedBy, &a.RetiredAt, &a.RetireReason, &a.ParentAssetID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("asset not found")
	}
	if err != nil {
		return nil, fmt.Errorf("get asset by tag: %w", err)
	}
	return &a, nil
}

// Create 创建资产
func (r *AssetRepo) Create(ctx context.Context, q DBTX, a *AssetRow) error {
	_, err := q.Exec(ctx,
		`INSERT INTO assets.assets (id, asset_tag, name, type_id, org_id, serial_number,
		 manufacturer, model, lifecycle_state, status, properties, version, created_at, updated_at,
		 purchase_price, purchase_date, supplier, warranty_until, depreciation_method,
		 useful_life_months, salvage_value, managed_by, parent_asset_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
		a.ID, a.AssetTag, a.Name, a.TypeID, a.OrgID,
		a.SerialNumber, a.Manufacturer, a.Model,
		a.LifecycleState, a.Status, a.Properties, a.Version, a.CreatedAt, a.UpdatedAt,
		a.PurchasePrice, a.PurchaseDate, a.Supplier, a.WarrantyUntil,
		a.DepreciationMethod, a.UsefulLifeMonths, a.SalvageValue, a.ManagedBy,
		a.ParentAssetID,
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
		 lifecycle_state, status, properties, version, deleted_at, created_at, updated_at,
		 purchase_price, purchase_date, supplier, warranty_until, depreciation_method,
		 useful_life_months, salvage_value, managed_by, retired_at, retire_reason, parent_asset_id
		 FROM assets.assets WHERE id=$1 AND org_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, orgID,
	).Scan(&a.ID, &a.AssetTag, &a.Name, &a.TypeID, &a.OrgID,
		&a.SerialNumber, &a.Manufacturer, &a.Model,
		&a.LifecycleState, &a.Status, &a.Properties,
		&a.Version, &a.DeletedAt, &a.CreatedAt, &a.UpdatedAt,
		&a.PurchasePrice, &a.PurchaseDate, &a.Supplier, &a.WarrantyUntil,
		&a.DepreciationMethod, &a.UsefulLifeMonths, &a.SalvageValue,
		&a.ManagedBy, &a.RetiredAt, &a.RetireReason, &a.ParentAssetID)
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

// WarrantyExpiringRow 质保到期扫描结果 (scheduler 用)
type WarrantyExpiringRow struct {
	AssetID       string    `json:"asset_id"`
	AssetTag      string    `json:"asset_tag"`
	Name          string    `json:"name"`
	OrgID         string    `json:"org_id"`
	WarrantyUntil time.Time `json:"warranty_until"`
	Expired       bool      `json:"expired"`
}

// ScanWarrantyExpiring 扫描质保即将到期 (within days 天) 与已过期的资产。
// 跨所有 org 扫描 (scheduler 系统级任务); 每行包含 org_id 供事件发布。
// 只查未软删除且质保日期非空的资产。
func (r *AssetRepo) ScanWarrantyExpiring(ctx context.Context, q DBTX, days int) ([]WarrantyExpiringRow, error) {
	if days <= 0 {
		days = 30
	}
	query := `
		SELECT id, asset_tag, name, org_id, warranty_until,
		       (warranty_until < CURRENT_DATE) AS expired
		FROM assets.assets
		WHERE deleted_at IS NULL
		  AND warranty_until IS NOT NULL
		  AND warranty_until < CURRENT_DATE + $1::int
		ORDER BY warranty_until ASC
		LIMIT 1000`
	rows, err := q.Query(ctx, query, days)
	if err != nil {
		return nil, fmt.Errorf("scan warranty expiring: %w", err)
	}
	defer rows.Close()

	var out []WarrantyExpiringRow
	for rows.Next() {
		var r WarrantyExpiringRow
		if err := rows.Scan(&r.AssetID, &r.AssetTag, &r.Name, &r.OrgID,
			&r.WarrantyUntil, &r.Expired); err != nil {
			return nil, fmt.Errorf("scan warranty row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
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
