// Package service — 折旧计算服务
// Phase H Step 1: 折旧计算 (SQL 内联计算，不建快照表)
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
)

// DepreciationRow 折旧计算结果
type DepreciationRow struct {
	AssetID            string  `json:"asset_id"`
	AssetTag           string  `json:"asset_tag"`
	Name               string  `json:"name"`
	PurchasePrice      float64 `json:"purchase_price"`
	MonthlyDep         float64 `json:"monthly_depreciation"`
	NetBookValue       float64 `json:"net_book_value"`
	MonthsElapsed      int     `json:"months_elapsed"`
	UsefulLifeMonths   int     `json:"useful_life_months"`
	DepreciationMethod string  `json:"depreciation_method"`
}

// DepreciationService 折旧服务
type DepreciationService struct{}

func NewDepreciationService() *DepreciationService {
	return &DepreciationService{}
}

// GetDepreciation 计算折旧 (游标分页)
// 公式: 月折旧=(purchase_price−salvage_value)/useful_life_months
//
//	已提月数=MIN(整月差, useful_life_months)
//	净值=MAX(purchase_price−月折旧×月数, salvage_value)
func (s *DepreciationService) GetDepreciation(ctx context.Context, q repository.DBTX, orgID string, asOfDate string) ([]DepreciationRow, string, bool, error) {
	// 解析 asOfDate，默认今天
	asOf := time.Now()
	if asOfDate != "" {
		parsed, err := time.Parse("2006-01-02", asOfDate)
		if err == nil {
			asOf = parsed
		}
	}

	query := `
		SELECT
			a.id,
			a.asset_tag,
			a.name,
			COALESCE(a.purchase_price, 0)::float8,
			CASE
				WHEN a.useful_life_months IS NOT NULL AND a.useful_life_months > 0
				THEN (COALESCE(a.purchase_price, 0) - COALESCE(a.salvage_value, 0)) / a.useful_life_months
				ELSE 0
			END::float8,
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
			END::float8,
			LEAST(
				COALESCE((EXTRACT(YEAR FROM age($2::date, a.purchase_date::date)) * 12 +
				          EXTRACT(MONTH FROM age($2::date, a.purchase_date::date)))::int, 0),
				COALESCE(a.useful_life_months, 0)
			)::int,
			COALESCE(a.useful_life_months, 0)::int,
			a.depreciation_method
		FROM assets.assets a
		WHERE a.org_id = $1
		  AND a.deleted_at IS NULL
		  AND a.depreciation_method = 'straight_line'
		  AND a.purchase_date IS NOT NULL
		  AND a.useful_life_months IS NOT NULL
		  AND a.useful_life_months > 0
		ORDER BY a.purchase_date DESC, a.id DESC
	`

	rows, err := q.Query(ctx, query, orgID, asOf.Format("2006-01-02"))
	if err != nil {
		return nil, "", false, fmt.Errorf("query depreciation: %w", err)
	}
	defer rows.Close()

	var results []DepreciationRow
	for rows.Next() {
		var r DepreciationRow
		if err := rows.Scan(&r.AssetID, &r.AssetTag, &r.Name,
			&r.PurchasePrice, &r.MonthlyDep, &r.NetBookValue,
			&r.MonthsElapsed, &r.UsefulLifeMonths, &r.DepreciationMethod); err != nil {
			return nil, "", false, fmt.Errorf("scan depreciation row: %w", err)
		}
		results = append(results, r)
	}

	if rows.Err() != nil {
		return nil, "", false, fmt.Errorf("iterate depreciation rows: %w", rows.Err())
	}

	return results, "", false, nil
}
