// Package repository — 资产领用数据访问层 (PG)
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/lock"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AssignmentRepo 领用管理仓库
type AssignmentRepo struct {
	pool      *pgxpool.Pool
	assetRepo *AssetRepo
}

func NewAssignmentRepo(pool *pgxpool.Pool) *AssignmentRepo {
	return &AssignmentRepo{
		pool:      pool,
		assetRepo: NewAssetRepo(pool),
	}
}

// Assign 领用资产: 悲观锁 + 写入 assignments 表 + 更新资产状态
func (r *AssignmentRepo) Assign(ctx context.Context, assetID, orgID, assignedTo, assignedBy, notes string) (string, error) {
	asset, err := r.assetRepo.LockForUpdate(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("asset not found: %w", err)
	}
	if asset.Status != "available" {
		return "", fmt.Errorf("asset is %s, cannot assign", asset.Status)
	}

	assignmentID := uuid.New().String()
	now := time.Now()
	_, err = r.pool.Exec(ctx,
		`INSERT INTO assets.assignments (id, asset_id, org_id, assigned_to, assigned_by, status, notes, assigned_at, version)
		 VALUES ($1,$2,$3,$4,$5,'active',$6,$7,1)`,
		assignmentID, assetID, orgID, assignedTo, assignedBy, notes, now)
	if err != nil {
		return "", fmt.Errorf("create assignment: %w", err)
	}

	_, err = r.pool.Exec(ctx,
		`UPDATE assets.assets SET status='assigned', version=version+1, updated_at=$1
		 WHERE id=$2 AND deleted_at IS NULL`, now, assetID)
	if err != nil {
		return "", fmt.Errorf("update asset status: %w", err)
	}

	return assignmentID, nil
}

// Release 归还资产: 关闭 assignment + 恢复资产状态
func (r *AssignmentRepo) Release(ctx context.Context, assetID string) error {
	_, err := r.assetRepo.LockForUpdate(ctx, assetID)
	if err != nil {
		return fmt.Errorf("asset not found: %w", err)
	}

	now := time.Now()

	tag, err := r.pool.Exec(ctx,
		`UPDATE assets.assignments SET status='returned', returned_at=$1
		 WHERE asset_id=$2 AND status='active'`, now, assetID)
	if err != nil {
		return fmt.Errorf("close assignment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("no active assignment found for asset %s", assetID)
	}

	_, err = r.pool.Exec(ctx,
		`UPDATE assets.assets SET status='available', version=version+1, updated_at=$1
		 WHERE id=$2 AND deleted_at IS NULL`, now, assetID)
	if err != nil {
		return fmt.Errorf("update asset status: %w", err)
	}

	return nil
}

// Transfer 转移资产: 字典序锁定防止死锁
func (r *AssignmentRepo) Transfer(ctx context.Context, assetID, toUserID string) error {
	ids := lock.SortedAssetIDs([]string{assetID})
	if err := lock.ValidateSortedOrder(ids); err != nil {
		return err
	}

	_, err := r.assetRepo.LockAssetsSorted(ctx, ids)
	if err != nil {
		return fmt.Errorf("lock asset: %w", err)
	}

	now := time.Now()

	_, err = r.pool.Exec(ctx,
		`UPDATE assets.assignments SET status='transferred', returned_at=$1
		 WHERE asset_id=$2 AND status='active'`, now, assetID)
	if err != nil {
		return fmt.Errorf("close old assignment: %w", err)
	}

	var orgID string
	err = r.pool.QueryRow(ctx,
		`SELECT org_id FROM assets.assets WHERE id=$1 AND deleted_at IS NULL`, assetID).Scan(&orgID)
	if err != nil {
		return fmt.Errorf("get asset org: %w", err)
	}

	_, err = r.pool.Exec(ctx,
		`INSERT INTO assets.assignments (id, asset_id, org_id, assigned_to, assigned_by, status, assigned_at, version)
		 VALUES ($1,$2,$3,$4,$2,'active','transfer',NOW(),1)`,
		uuid.New().String(), assetID, orgID, toUserID)
	if err != nil {
		return fmt.Errorf("create new assignment: %w", err)
	}

	return nil
}

// ActiveAssignment 活跃领用记录
type ActiveAssignment struct {
	ID         string    `json:"id"`
	AssetID    string    `json:"asset_id"`
	AssignedTo string    `json:"assigned_to"`
	AssignedBy string    `json:"assigned_by"`
	Notes      string    `json:"notes"`
	AssignedAt time.Time `json:"assigned_at"`
}

// GetActiveAssignment 获取资产的活跃领用记录
func (r *AssignmentRepo) GetActiveAssignment(ctx context.Context, assetID string) (*ActiveAssignment, error) {
	var a ActiveAssignment
	err := r.pool.QueryRow(ctx,
		`SELECT id, asset_id, assigned_to, assigned_by, COALESCE(notes,''), assigned_at
		 FROM assets.assignments WHERE asset_id = $1 AND status = 'active'
		 ORDER BY assigned_at DESC LIMIT 1`, assetID,
	).Scan(&a.ID, &a.AssetID, &a.AssignedTo, &a.AssignedBy, &a.Notes, &a.AssignedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
