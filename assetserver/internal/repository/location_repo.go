// Package repository — 位置数据访问层 (PG)
// 对应 Phase B §7 location_repo
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// LocationRow 位置数据库行
type LocationRow struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Name      string    `json:"name"`
	Code      *string   `json:"code"`
	ParentID  *string   `json:"parent_id"`
	Notes     *string   `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LocationRepo 位置仓库 (无状态)
type LocationRepo struct{}

func NewLocationRepo() *LocationRepo {
	return &LocationRepo{}
}

// ListByOrg 获取组织内所有位置
func (r *LocationRepo) ListByOrg(ctx context.Context, q DBTX, orgID string) ([]LocationRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, org_id, name, code, parent_id, notes, created_at, updated_at
		 FROM assets.locations WHERE org_id = $1 ORDER BY name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	defer rows.Close()

	var locs []LocationRow
	for rows.Next() {
		var l LocationRow
		if err := rows.Scan(&l.ID, &l.OrgID, &l.Name, &l.Code, &l.ParentID, &l.Notes, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan location: %w", err)
		}
		locs = append(locs, l)
	}
	return locs, nil
}

// GetByID 获取单个位置 (含 org_id 过滤)
func (r *LocationRepo) GetByID(ctx context.Context, q DBTX, id string, orgID string) (*LocationRow, error) {
	var l LocationRow
	err := q.QueryRow(ctx,
		`SELECT id, org_id, name, code, parent_id, notes, created_at, updated_at
		 FROM assets.locations WHERE id = $1 AND org_id = $2`, id, orgID,
	).Scan(&l.ID, &l.OrgID, &l.Name, &l.Code, &l.ParentID, &l.Notes, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("location not found: %w", err)
	}
	return &l, nil
}

// CreateLocationInput 创建位置输入
type CreateLocationInput struct {
	Name     string
	Code     *string
	OrgID    string
	ParentID *string
	Notes    *string
}

// Create 创建位置
func (r *LocationRepo) Create(ctx context.Context, q DBTX, input CreateLocationInput) (*LocationRow, error) {
	id := uuid.New().String()
	now := time.Now()

	_, err := q.Exec(ctx,
		`INSERT INTO assets.locations (id, org_id, name, code, parent_id, notes, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id, input.OrgID, input.Name, input.Code, input.ParentID, input.Notes, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create location: %w", err)
	}

	return &LocationRow{
		ID: id, OrgID: input.OrgID, Name: input.Name,
		Code: input.Code, ParentID: input.ParentID, Notes: input.Notes,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Update 更新位置 (含 org_id 过滤防止 IDOR)
func (r *LocationRepo) Update(ctx context.Context, q DBTX, id string, orgID string, name, code, notes *string, parentID *string) error {
	tag, err := q.Exec(ctx,
		`UPDATE assets.locations
		 SET name = COALESCE($3, name),
		     code = COALESCE($4, code),
		     notes = COALESCE($5, notes),
		     parent_id = COALESCE($6, parent_id),
		     updated_at = now()
		 WHERE id = $1 AND org_id = $2`,
		id, orgID, name, code, notes, parentID,
	)
	if err != nil {
		return fmt.Errorf("update location: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("location not found")
	}
	return nil
}

// Delete 删除位置 (含 org_id 过滤防止 IDOR)
func (r *LocationRepo) Delete(ctx context.Context, q DBTX, id string, orgID string) error {
	tag, err := q.Exec(ctx,
		`DELETE FROM assets.locations WHERE id = $1 AND org_id = $2`, id, orgID,
	)
	if err != nil {
		return fmt.Errorf("delete location: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("location not found")
	}
	return nil
}
