// Package repository — 组织数据访问层 (PG ltree)
// 对应 Phase B §7 org_repo
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// OrgRow 组织数据库行
type OrgRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id"`
	Path      string    `json:"path"`
	Depth     int       `json:"depth"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgRepo 组织仓库 (无状态)
type OrgRepo struct{}

func NewOrgRepo() *OrgRepo {
	return &OrgRepo{}
}

// ListOrgs 获取所有组织 (按路径排序以支持树构建)
func (r *OrgRepo) ListOrgs(ctx context.Context, q DBTX) ([]OrgRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, name, parent_id, path::text, depth, created_at, updated_at
		 FROM assets.organizations ORDER BY path`)
	if err != nil {
		return nil, fmt.Errorf("list orgs: %w", err)
	}
	defer rows.Close()

	var orgs []OrgRow
	for rows.Next() {
		var o OrgRow
		if err := rows.Scan(&o.ID, &o.Name, &o.ParentID, &o.Path, &o.Depth, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan org: %w", err)
		}
		orgs = append(orgs, o)
	}
	return orgs, nil
}

// GetOrg 获取单个组织
func (r *OrgRepo) GetOrg(ctx context.Context, q DBTX, id string) (*OrgRow, error) {
	var o OrgRow
	err := q.QueryRow(ctx,
		`SELECT id, name, parent_id, path::text, depth, created_at, updated_at
		 FROM assets.organizations WHERE id = $1`, id,
	).Scan(&o.ID, &o.Name, &o.ParentID, &o.Path, &o.Depth, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("org not found: %w", err)
	}
	return &o, nil
}

// CreateOrgInput 创建组织输入
type CreateOrgInput struct {
	Name     string
	ParentID *string
}

// CreateOrg 创建组织 (含 ltree 路径维护)
func (r *OrgRepo) CreateOrg(ctx context.Context, q DBTX, input CreateOrgInput) (*OrgRow, error) {
	var parentPath string
	var depth int

	if input.ParentID != nil {
		parent, err := r.GetOrg(ctx, q, *input.ParentID)
		if err != nil {
			return nil, fmt.Errorf("parent org not found: %w", err)
		}
		if parent.Depth >= 20 {
			return nil, fmt.Errorf("max depth (20) exceeded")
		}
		parentPath = parent.Path
		depth = parent.Depth + 1
	} else {
		parentPath = "root"
		depth = 1
	}

	// 构建 ltree 路径: parent_path.sanitized_name
	sanitized := strings.ReplaceAll(input.Name, " ", "_")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	path := parentPath + "." + sanitized

	id := uuid.New().String()
	now := time.Now()

	_, err := q.Exec(ctx,
		`INSERT INTO assets.organizations (id, name, parent_id, path, depth, created_at, updated_at)
		 VALUES ($1, $2, $3, $4::ltree, $5, $6, $7)`,
		id, input.Name, input.ParentID, path, depth, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}

	return &OrgRow{
		ID: id, Name: input.Name, ParentID: input.ParentID,
		Path: path, Depth: depth,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Subtree 查询 path 前缀的子孙组织 (ltree <@ 操作符)
func (r *OrgRepo) Subtree(ctx context.Context, q DBTX, path string) ([]OrgRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, name, parent_id, path::text, depth, created_at, updated_at
		 FROM assets.organizations WHERE path <@ $1::ltree ORDER BY path`, path)
	if err != nil {
		return nil, fmt.Errorf("subtree: %w", err)
	}
	defer rows.Close()

	var orgs []OrgRow
	for rows.Next() {
		var o OrgRow
		if err := rows.Scan(&o.ID, &o.Name, &o.ParentID, &o.Path, &o.Depth, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan org: %w", err)
		}
		orgs = append(orgs, o)
	}
	return orgs, nil
}
