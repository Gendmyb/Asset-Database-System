// Package repository — AD 安全组映射数据访问层 (Wave 3 T2)
package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// GroupMapping AD 安全组 → 角色映射行
type GroupMapping struct {
	ID          string    `json:"id"`
	GroupDN     string    `json:"group_dn"`
	GroupName   string    `json:"group_name,omitempty"`
	Role        string    `json:"role"`
	DataScope   string    `json:"data_scope"`
	SyncEnabled bool      `json:"sync_enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ADGroupRepo 安全组映射仓库
type ADGroupRepo struct{}

// NewADGroupRepo 构造安全组映射仓库
func NewADGroupRepo() *ADGroupRepo {
	return &ADGroupRepo{}
}

// ListEnabled 列出所有启用 (sync_enabled=true) 的映射, 按 group_dn 排序。
// 供同步引擎读取: 只关心 group_dn + role + data_scope。
func (r *ADGroupRepo) ListEnabled(ctx context.Context, q DBTX) ([]GroupMapping, error) {
	rows, err := q.Query(ctx,
		`SELECT id, group_dn, COALESCE(group_name,''), role, data_scope, sync_enabled, created_at, updated_at
		 FROM assets.ad_group_mappings
		 WHERE sync_enabled = true
		 ORDER BY group_dn`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMappings(rows)
}

// ListAll 列出所有映射 (含禁用的), 供管理 API 使用。
func (r *ADGroupRepo) ListAll(ctx context.Context, q DBTX) ([]GroupMapping, error) {
	rows, err := q.Query(ctx,
		`SELECT id, group_dn, COALESCE(group_name,''), role, data_scope, sync_enabled, created_at, updated_at
		 FROM assets.ad_group_mappings
		 ORDER BY group_dn`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMappings(rows)
}

// GetByID 按 ID 查询单条映射
func (r *ADGroupRepo) GetByID(ctx context.Context, q DBTX, id string) (*GroupMapping, error) {
	var m GroupMapping
	err := q.QueryRow(ctx,
		`SELECT id, group_dn, COALESCE(group_name,''), role, data_scope, sync_enabled, created_at, updated_at
		 FROM assets.ad_group_mappings WHERE id = $1`, id,
	).Scan(&m.ID, &m.GroupDN, &m.GroupName, &m.Role, &m.DataScope, &m.SyncEnabled, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Create 新增映射 (group_dn 必须唯一)
func (r *ADGroupRepo) Create(ctx context.Context, q DBTX, groupDN, groupName, role, dataScope string) (*GroupMapping, error) {
	var m GroupMapping
	err := q.QueryRow(ctx,
		`INSERT INTO assets.ad_group_mappings (group_dn, group_name, role, data_scope)
		 VALUES ($1, NULLIF($2,''), $3, $4)
		 RETURNING id, group_dn, COALESCE(group_name,''), role, data_scope, sync_enabled, created_at, updated_at`,
		groupDN, groupName, role, dataScope,
	).Scan(&m.ID, &m.GroupDN, &m.GroupName, &m.Role, &m.DataScope, &m.SyncEnabled, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Update 更新映射字段 (role/data_scope/sync_enabled/group_name)
func (r *ADGroupRepo) Update(ctx context.Context, q DBTX, id string, role, dataScope *string, syncEnabled *bool, groupName *string) (*GroupMapping, error) {
	var m GroupMapping
	err := q.QueryRow(ctx,
		`UPDATE assets.ad_group_mappings SET
		   role = COALESCE($2, role),
		   data_scope = COALESCE($3, data_scope),
		   sync_enabled = COALESCE($4, sync_enabled),
		   group_name = COALESCE($5, group_name),
		   updated_at = now()
		 WHERE id = $1
		 RETURNING id, group_dn, COALESCE(group_name,''), role, data_scope, sync_enabled, created_at, updated_at`,
		id, role, dataScope, syncEnabled, groupName,
	).Scan(&m.ID, &m.GroupDN, &m.GroupName, &m.Role, &m.DataScope, &m.SyncEnabled, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// Delete 删除映射
func (r *ADGroupRepo) Delete(ctx context.Context, q DBTX, id string) error {
	_, err := q.Exec(ctx, `DELETE FROM assets.ad_group_mappings WHERE id = $1`, id)
	return err
}

// GetDefaultRole 返回系统默认角色与 data_scope。
// 当用户不属于任何映射组时使用: viewer + inherit (行为与历史一致)。
func (r *ADGroupRepo) GetDefaultRole() (role, dataScope string) {
	return "viewer", "inherit"
}

func scanMappings(rows pgx.Rows) ([]GroupMapping, error) {
	var out []GroupMapping
	for rows.Next() {
		var m GroupMapping
		if err := rows.Scan(&m.ID, &m.GroupDN, &m.GroupName, &m.Role, &m.DataScope, &m.SyncEnabled, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
