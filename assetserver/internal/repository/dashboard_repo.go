// Package repository — Dashboard 聚合查询 (PG)
package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DashboardRepo 仪表盘数据访问
type DashboardRepo struct {
	pool *pgxpool.Pool
}

func NewDashboardRepo(pool *pgxpool.Pool) *DashboardRepo {
	return &DashboardRepo{pool: pool}
}

// DashboardStats 仪表盘统计
type DashboardStats struct {
	TotalAssets  int64            `json:"total_assets"`
	ByStatus     map[string]int64 `json:"by_status"`
	ByCategory   map[string]int64 `json:"by_category"`
	ByLifecycle  map[string]int64 `json:"by_lifecycle"`
}

// GetStats 获取仪表盘统计数据
func (r *DashboardRepo) GetStats(ctx context.Context, orgID string) (*DashboardStats, error) {
	stats := &DashboardStats{
		ByStatus:    make(map[string]int64),
		ByCategory:  make(map[string]int64),
		ByLifecycle: make(map[string]int64),
	}

	// 总资产数
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM assets.assets WHERE org_id = $1 AND deleted_at IS NULL`, orgID,
	).Scan(&stats.TotalAssets)
	if err != nil {
		return nil, err
	}

	// 按状态分组
	rows, err := r.pool.Query(ctx,
		`SELECT status, COUNT(*) FROM assets.assets 
		 WHERE org_id = $1 AND deleted_at IS NULL GROUP BY status`, orgID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err == nil {
				stats.ByStatus[status] = count
			}
		}
	}

	// 按类别分组 (JOIN asset_types)
	rows2, err := r.pool.Query(ctx,
		`SELECT at.category, COUNT(*) FROM assets.assets a
		 JOIN assets.asset_types at ON a.type_id = at.id
		 WHERE a.org_id = $1 AND a.deleted_at IS NULL
		 GROUP BY at.category`, orgID)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var category string
			var count int64
			if err := rows2.Scan(&category, &count); err == nil {
				stats.ByCategory[category] = count
			}
		}
	}

	// 按生命周期分组
	rows3, err := r.pool.Query(ctx,
		`SELECT lifecycle_state, COUNT(*) FROM assets.assets
		 WHERE org_id = $1 AND deleted_at IS NULL GROUP BY lifecycle_state`, orgID)
	if err == nil {
		defer rows3.Close()
		for rows3.Next() {
			var state string
			var count int64
			if err := rows3.Scan(&state, &count); err == nil {
				stats.ByLifecycle[state] = count
			}
		}
	}

	return stats, nil
}

// AssetType 资产类型 (简化)
type AssetType struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

// ListAssetTypes 获取所有资产类型
func (r *DashboardRepo) ListAssetTypes(ctx context.Context) ([]AssetType, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, category FROM assets.asset_types ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var types []AssetType
	for rows.Next() {
		var t AssetType
		if err := rows.Scan(&t.ID, &t.Name, &t.Category); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, nil
}
