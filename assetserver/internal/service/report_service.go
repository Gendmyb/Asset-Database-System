// Package service — 报表服务
// Phase H Step 2: 汇总报表/维保成本/到期查询
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
)

// SummaryReport 汇总报表
type SummaryReport struct {
	TotalAssets        int64            `json:"total_assets"`
	TotalPurchasePrice float64          `json:"total_purchase_price"`
	TotalNetBookValue  float64          `json:"total_net_book_value"`
	ByStatus           []GroupCount     `json:"by_status"`
	ByAssetType        []AssetTypeGroup `json:"by_asset_type"`
	ByLocation         []GroupCount     `json:"by_location"`
}

// GroupCount 通用分组统计
type GroupCount struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

// AssetTypeGroup 资产类型分组 (含类型名)
type AssetTypeGroup struct {
	TypeName string `json:"type_name"`
	Count    int64  `json:"count"`
}

// CostRow 维保成本行
type CostRow struct {
	Vendor    string  `json:"vendor"`
	Count     int64   `json:"count"`
	TotalCost float64 `json:"total_cost"`
}

// DueRow 到期/逾期领用行
type DueRow struct {
	AssignmentID string     `json:"assignment_id"`
	AssetID      string     `json:"asset_id"`
	AssetName    string     `json:"asset_name"`
	AssignedTo   string     `json:"assigned_to"`
	DueDate      *time.Time `json:"due_date,omitempty"`
	Overdue      bool       `json:"overdue"`
}

// ReportService 报表服务
type ReportService struct {
	depSvc *DepreciationService
}

func NewReportService() *ReportService {
	return &ReportService{depSvc: NewDepreciationService()}
}

// netBookValueExpr 内联计算净值 SQL 表达式
const netBookValueExpr = `
	CASE
		WHEN a.depreciation_method = 'straight_line'
		     AND a.purchase_date IS NOT NULL
		     AND a.useful_life_months IS NOT NULL
		     AND a.useful_life_months > 0
		THEN GREATEST(
			COALESCE(a.purchase_price, 0) -
			((COALESCE(a.purchase_price, 0) - COALESCE(a.salvage_value, 0)) / a.useful_life_months) *
			LEAST(
				(EXTRACT(YEAR FROM age($2::date, a.purchase_date::date)) * 12 +
				 EXTRACT(MONTH FROM age($2::date, a.purchase_date::date)))::int,
				a.useful_life_months
			),
			COALESCE(a.salvage_value, 0)
		)
		ELSE COALESCE(a.purchase_price, 0)
	END`

// GetSummary 生成汇总报表
func (s *ReportService) GetSummary(ctx context.Context, q repository.DBTX, orgID string, asOfDate string) (*SummaryReport, error) {
	asOf := time.Now()
	if asOfDate != "" {
		parsed, err := time.Parse("2006-01-02", asOfDate)
		if err == nil {
			asOf = parsed
		}
	}
	dateStr := asOf.Format("2006-01-02")

	report := &SummaryReport{
		ByStatus:    make([]GroupCount, 0),
		ByAssetType: make([]AssetTypeGroup, 0),
		ByLocation:  make([]GroupCount, 0),
	}

	// 1. 总数 + 采购总额 + 净值总额
	totalQuery := fmt.Sprintf(`
		SELECT
			COUNT(*)::int8,
			COALESCE(SUM(COALESCE(a.purchase_price, 0)), 0)::float8,
			COALESCE(SUM(%s), 0)::float8
		FROM assets.assets a
		WHERE a.org_id = $1 AND a.deleted_at IS NULL`, netBookValueExpr)

	if err := q.QueryRow(ctx, totalQuery, orgID, dateStr).Scan(
		&report.TotalAssets, &report.TotalPurchasePrice, &report.TotalNetBookValue,
	); err != nil {
		return nil, fmt.Errorf("query totals: %w", err)
	}

	// 2. 按状态分组
	statusQuery := fmt.Sprintf(`
		SELECT COALESCE(a.status, 'unknown'), COUNT(*)::int8
		FROM assets.assets a
		WHERE a.org_id = $1 AND a.deleted_at IS NULL
		GROUP BY a.status ORDER BY COUNT(*) DESC`)
	statusRows, err := q.Query(ctx, statusQuery, orgID)
	if err == nil {
		defer statusRows.Close()
		for statusRows.Next() {
			var gc GroupCount
			if err := statusRows.Scan(&gc.Key, &gc.Count); err == nil {
				report.ByStatus = append(report.ByStatus, gc)
			}
		}
	}

	// 3. 按资产类型分组
	typeQuery := `
		SELECT at.name, COUNT(*)::int8
		FROM assets.assets a
		JOIN assets.asset_types at ON a.type_id = at.id
		WHERE a.org_id = $1 AND a.deleted_at IS NULL
		GROUP BY at.name ORDER BY COUNT(*) DESC`
	typeRows, err := q.Query(ctx, typeQuery, orgID)
	if err == nil {
		defer typeRows.Close()
		for typeRows.Next() {
			var ag AssetTypeGroup
			if err := typeRows.Scan(&ag.TypeName, &ag.Count); err == nil {
				report.ByAssetType = append(report.ByAssetType, ag)
			}
		}
	}

	// 4. 按位置分组
	locQuery := `
		SELECT COALESCE(l.name, '未指定'), COUNT(*)::int8
		FROM assets.assets a
		LEFT JOIN assets.locations l ON a.location_id = l.id
		WHERE a.org_id = $1 AND a.deleted_at IS NULL
		GROUP BY l.name ORDER BY COUNT(*) DESC`
	locRows, err := q.Query(ctx, locQuery, orgID)
	if err == nil {
		defer locRows.Close()
		for locRows.Next() {
			var gc GroupCount
			if err := locRows.Scan(&gc.Key, &gc.Count); err == nil {
				report.ByLocation = append(report.ByLocation, gc)
			}
		}
	}

	return report, nil
}

// GetMaintenanceCost 维保成本统计
func (s *ReportService) GetMaintenanceCost(ctx context.Context, q repository.DBTX, orgID string, from, to string) ([]CostRow, error) {
	query := `
		SELECT COALESCE(vendor, '未指定') AS vendor,
		       COUNT(*)::int8,
		       COALESCE(SUM(cost), 0)::float8
		FROM assets.maintenance_orders
		WHERE org_id = $1
		  AND status = 'completed'
		  AND finished_at IS NOT NULL`
	args := []interface{}{orgID}
	argIdx := 2

	if from != "" {
		query += fmt.Sprintf(" AND finished_at >= $%d", argIdx)
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		query += fmt.Sprintf(" AND finished_at < ($%d::timestamptz + interval '1 day')", argIdx)
		args = append(args, to)
		argIdx++
	}

	query += ` GROUP BY vendor ORDER BY COUNT(*) DESC`

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query maintenance cost: %w", err)
	}
	defer rows.Close()

	var results []CostRow
	for rows.Next() {
		var cr CostRow
		if err := rows.Scan(&cr.Vendor, &cr.Count, &cr.TotalCost); err != nil {
			return nil, fmt.Errorf("scan cost row: %w", err)
		}
		results = append(results, cr)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate cost rows: %w", rows.Err())
	}
	return results, nil
}

// GetAssignmentsDue 查询即将到期/已逾期领用
func (s *ReportService) GetAssignmentsDue(ctx context.Context, q repository.DBTX, orgID string, days int) ([]DueRow, error) {
	if days <= 0 {
		days = 30
	}

	query := `
		SELECT
			asgn.id,
			asgn.asset_id,
			a.name AS asset_name,
			COALESCE(u.username, asgn.assigned_to::text) AS assigned_to,
			asgn.due_date,
			CASE WHEN asgn.due_date < CURRENT_DATE THEN true ELSE false END AS overdue
		FROM assets.assignments asgn
		JOIN assets.assets a ON asgn.asset_id = a.id
		LEFT JOIN assets.users u ON asgn.assigned_to = u.id
		WHERE asgn.org_id = $1
		  AND asgn.status = 'active'
		  AND asgn.assignment_type = 'temporary'
		  AND asgn.due_date < CURRENT_DATE + $2::int
		ORDER BY asgn.due_date ASC`

	rows, err := q.Query(ctx, query, orgID, days)
	if err != nil {
		return nil, fmt.Errorf("query assignments due: %w", err)
	}
	defer rows.Close()

	var results []DueRow
	for rows.Next() {
		var dr DueRow
		if err := rows.Scan(&dr.AssignmentID, &dr.AssetID, &dr.AssetName,
			&dr.AssignedTo, &dr.DueDate, &dr.Overdue); err != nil {
			return nil, fmt.Errorf("scan due row: %w", err)
		}
		results = append(results, dr)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate due rows: %w", rows.Err())
	}
	return results, nil
}
