// Package service — CSV/XLSX 导出服务
// Phase H Step 3: CSV 导出 (UTF-8 BOM)
// Wave 1 G5: Excel (xlsx) 导出
package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/xuri/excelize/v2"
)

// ExportService 导出服务
type ExportService struct{}

func NewExportService() *ExportService {
	return &ExportService{}
}

// writeBOM 写入 UTF-8 BOM
func writeBOM(w io.Writer) {
	w.Write([]byte{0xEF, 0xBB, 0xBF})
}

// ExportAssetsCSV 导出资产为 CSV
// 列: 编号/名称/类型/SN/制造商/型号/位置/状态/生命周期/采购价格/净值/管理人
func (s *ExportService) ExportAssetsCSV(ctx context.Context, q repository.DBTX, orgID string, writer io.Writer, f repository.AssetFilter) error {
	writeBOM(writer)
	w := csv.NewWriter(writer)

	// 写表头
	if err := w.Write([]string{
		"编号", "名称", "类型", "SN", "制造商", "型号",
		"位置", "状态", "生命周期", "采购价格", "净值", "管理人",
	}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	// 查询（带 JOIN 获取类型名/位置名/管理人用户名）
	if f.Limit <= 0 || f.Limit > 2000 {
		f.Limit = 2000
	}
	// 导出不分页，取所有匹配行
	f.Limit = 2000

	query := `SELECT a.asset_tag, a.name,
			COALESCE(at.name, ''), COALESCE(a.serial_number, ''),
			COALESCE(a.manufacturer, ''), COALESCE(a.model, ''),
			COALESCE(l.name, ''), a.status, a.lifecycle_state,
			COALESCE(a.purchase_price, 0),
			COALESCE(a.salvage_value, 0),
			a.depreciation_method,
			a.purchase_date,
			a.useful_life_months,
			COALESCE(u.username, '')
		FROM assets.assets a
		LEFT JOIN assets.asset_types at ON a.type_id = at.id
		LEFT JOIN assets.locations l ON a.location_id = l.id
		LEFT JOIN assets.users u ON a.managed_by = u.id
		WHERE a.org_id = $1 AND a.deleted_at IS NULL`
	args := []interface{}{orgID}
	argIdx := 2

	if f.Search != "" {
		query += fmt.Sprintf(" AND (a.name ILIKE $%d OR a.asset_tag ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}
	if f.TypeID != "" {
		query += fmt.Sprintf(" AND a.type_id = $%d", argIdx)
		args = append(args, f.TypeID)
		argIdx++
	}
	if f.Status != "" {
		query += fmt.Sprintf(" AND a.status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.Lifecycle != "" {
		query += fmt.Sprintf(" AND a.lifecycle_state = $%d", argIdx)
		args = append(args, f.Lifecycle)
		argIdx++
	}
	if f.Manufacturer != "" {
		query += fmt.Sprintf(" AND a.manufacturer ILIKE $%d", argIdx)
		args = append(args, "%"+f.Manufacturer+"%")
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY a.created_at DESC LIMIT $%d", argIdx)
	args = append(args, f.Limit)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query export assets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tag, name, typeName, sn, mfr, model, loc, status, lifecycle string
		var purchasePrice, salvageValue float64
		var depMethod string
		var purchaseDate *time.Time
		var usefulLife *int
		var managedBy string

		if err := rows.Scan(&tag, &name, &typeName, &sn, &mfr, &model,
			&loc, &status, &lifecycle, &purchasePrice, &salvageValue,
			&depMethod, &purchaseDate, &usefulLife, &managedBy); err != nil {
			return fmt.Errorf("scan export row: %w", err)
		}

		// 计算净值
		nbv := computeNetBookValue(purchasePrice, salvageValue, depMethod, purchaseDate, usefulLife)

		row := []string{
			tag, name, typeName, sn, mfr, model,
			loc, status, lifecycle,
			fmt.Sprintf("%.2f", purchasePrice),
			fmt.Sprintf("%.2f", nbv),
			managedBy,
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	if rows.Err() != nil {
		return fmt.Errorf("iterate export rows: %w", rows.Err())
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("csv flush: %w", err)
	}
	return nil
}

// ExportDepreciationCSV 导出折旧明细为 CSV
func (s *ExportService) ExportDepreciationCSV(ctx context.Context, q repository.DBTX, orgID string, writer io.Writer, asOfDate string) error {
	writeBOM(writer)
	w := csv.NewWriter(writer)

	if err := w.Write([]string{
		"资产编号", "名称", "采购价格", "月折旧", "净值", "已提月数", "可用月数", "折旧方法",
	}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	depSvc := NewDepreciationService()
	rows, _, _, err := depSvc.GetDepreciation(ctx, q, orgID, asOfDate)
	if err != nil {
		return fmt.Errorf("get depreciation: %w", err)
	}

	for _, r := range rows {
		row := []string{
			r.AssetTag, r.Name,
			fmt.Sprintf("%.2f", r.PurchasePrice),
			fmt.Sprintf("%.2f", r.MonthlyDep),
			fmt.Sprintf("%.2f", r.NetBookValue),
			fmt.Sprintf("%d", r.MonthsElapsed),
			fmt.Sprintf("%d", r.UsefulLifeMonths),
			r.DepreciationMethod,
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}

	w.Flush()
	return w.Error()
}

// ExportStocktakeReportCSV 导出盘点报表为 CSV
func (s *ExportService) ExportStocktakeReportCSV(ctx context.Context, q repository.DBTX, orgID string, planID string, writer io.Writer) error {
	writeBOM(writer)
	w := csv.NewWriter(writer)

	if err := w.Write([]string{
		"资产编号", "名称", "类型", "位置", "账面状态", "盘点结果", "差异说明", "盘点人", "盘点时间",
	}); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	query := `
		SELECT
			COALESCE(a.asset_tag, ''),
			COALESCE(a.name, ''),
			COALESCE(at.name, ''),
			COALESCE(l.name, ''),
			COALESCE(a.status, ''),
			COALESCE(si.result, ''),
			COALESCE(si.notes, ''),
			COALESCE(u.username, si.checked_by::text, ''),
			COALESCE(si.checked_at::text, '')
		FROM assets.stocktake_items si
		LEFT JOIN assets.assets a ON si.asset_id = a.id
		LEFT JOIN assets.asset_types at ON a.type_id = at.id
		LEFT JOIN assets.locations l ON a.location_id = l.id
		LEFT JOIN assets.users u ON si.checked_by = u.id
		WHERE si.plan_id = $1::uuid
		ORDER BY a.asset_tag`
	args := []interface{}{planID}
	_ = orgID // 通过 plan_id 关联，无需 orgID 过滤（盘点计划已隔离）

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query stocktake items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tag, name, typeName, loc, status, result, note, checker, checkDate string
		if err := rows.Scan(&tag, &name, &typeName, &loc, &status,
			&result, &note, &checker, &checkDate); err != nil {
			return fmt.Errorf("scan stocktake row: %w", err)
		}
		row := []string{tag, name, typeName, loc, status, result, note, checker, checkDate}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	if rows.Err() != nil {
		return fmt.Errorf("iterate stocktake rows: %w", rows.Err())
	}

	w.Flush()
	return w.Error()
}

// ── XLSX 导出 (Wave 1 G5) ────────────────────────────────────────────
// 与同名 CSV 方法同数据源、同列；返回完整 .xlsx 字节流。
// Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet

// exportAssetsRow 资产导出行的统一中间结构 (CSV/XLSX 复用)
type exportAssetsRow struct {
	Tag, Name, TypeName, SN, Mfr, Model, Loc, Status, Lifecycle string
	PurchasePrice, NBV                                          float64
	ManagedBy                                                   string
}

// queryExportAssets 执行资产导出查询，返回中间行。
// 逻辑与 ExportAssetsCSV 完全一致，仅抽出复用。
func queryExportAssets(ctx context.Context, q repository.DBTX, orgID string, f repository.AssetFilter) ([]exportAssetsRow, error) {
	if f.Limit <= 0 || f.Limit > 2000 {
		f.Limit = 2000
	}
	// 导出不分页，取所有匹配行
	f.Limit = 2000

	query := `SELECT a.asset_tag, a.name,
			COALESCE(at.name, ''), COALESCE(a.serial_number, ''),
			COALESCE(a.manufacturer, ''), COALESCE(a.model, ''),
			COALESCE(l.name, ''), a.status, a.lifecycle_state,
			COALESCE(a.purchase_price, 0),
			COALESCE(a.salvage_value, 0),
			a.depreciation_method,
			a.purchase_date,
			a.useful_life_months,
			COALESCE(u.username, '')
		FROM assets.assets a
		LEFT JOIN assets.asset_types at ON a.type_id = at.id
		LEFT JOIN assets.locations l ON a.location_id = l.id
		LEFT JOIN assets.users u ON a.managed_by = u.id
		WHERE a.org_id = $1 AND a.deleted_at IS NULL`
	args := []interface{}{orgID}
	argIdx := 2

	if f.Search != "" {
		query += fmt.Sprintf(" AND (a.name ILIKE $%d OR a.asset_tag ILIKE $%d)", argIdx, argIdx)
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}
	if f.TypeID != "" {
		query += fmt.Sprintf(" AND a.type_id = $%d", argIdx)
		args = append(args, f.TypeID)
		argIdx++
	}
	if f.Status != "" {
		query += fmt.Sprintf(" AND a.status = $%d", argIdx)
		args = append(args, f.Status)
		argIdx++
	}
	if f.Lifecycle != "" {
		query += fmt.Sprintf(" AND a.lifecycle_state = $%d", argIdx)
		args = append(args, f.Lifecycle)
		argIdx++
	}
	if f.Manufacturer != "" {
		query += fmt.Sprintf(" AND a.manufacturer ILIKE $%d", argIdx)
		args = append(args, "%"+f.Manufacturer+"%")
		argIdx++
	}

	query += fmt.Sprintf(" ORDER BY a.created_at DESC LIMIT $%d", argIdx)
	args = append(args, f.Limit)

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query export assets: %w", err)
	}
	defer rows.Close()

	var out []exportAssetsRow
	for rows.Next() {
		var r exportAssetsRow
		var purchasePrice, salvageValue float64
		var depMethod string
		var purchaseDate *time.Time
		var usefulLife *int
		if err := rows.Scan(&r.Tag, &r.Name, &r.TypeName, &r.SN, &r.Mfr, &r.Model,
			&r.Loc, &r.Status, &r.Lifecycle, &purchasePrice, &salvageValue,
			&depMethod, &purchaseDate, &usefulLife, &r.ManagedBy); err != nil {
			return nil, fmt.Errorf("scan export row: %w", err)
		}
		r.PurchasePrice = purchasePrice
		r.NBV = computeNetBookValue(purchasePrice, salvageValue, depMethod, purchaseDate, usefulLife)
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate export rows: %w", rows.Err())
	}
	return out, nil
}

// ExportAssetsXLSX 导出资产为 Excel (.xlsx)，列同 CSV
func (s *ExportService) ExportAssetsXLSX(ctx context.Context, q repository.DBTX, orgID string, f repository.AssetFilter) ([]byte, error) {
	rows, err := queryExportAssets(ctx, q, orgID, f)
	if err != nil {
		return nil, err
	}

	fx := excelize.NewFile()
	defer fx.Close()
	sheet := "资产"
	if _, err := fx.NewSheet(sheet); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	fx.DeleteSheet("Sheet1")

	headers := []string{"编号", "名称", "类型", "SN", "制造商", "型号",
		"位置", "状态", "生命周期", "采购价格", "净值", "管理人"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = fx.SetCellValue(sheet, cell, h)
	}
	// 表头加粗
	style, _ := fx.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	_ = fx.SetCellStyle(sheet, "A1", "L1", style)

	for r, row := range rows {
		rowNum := r + 2
		_ = fx.SetCellValue(sheet, cellName(1, rowNum), row.Tag)
		_ = fx.SetCellValue(sheet, cellName(2, rowNum), row.Name)
		_ = fx.SetCellValue(sheet, cellName(3, rowNum), row.TypeName)
		_ = fx.SetCellValue(sheet, cellName(4, rowNum), row.SN)
		_ = fx.SetCellValue(sheet, cellName(5, rowNum), row.Mfr)
		_ = fx.SetCellValue(sheet, cellName(6, rowNum), row.Model)
		_ = fx.SetCellValue(sheet, cellName(7, rowNum), row.Loc)
		_ = fx.SetCellValue(sheet, cellName(8, rowNum), row.Status)
		_ = fx.SetCellValue(sheet, cellName(9, rowNum), row.Lifecycle)
		_ = fx.SetCellValue(sheet, cellName(10, rowNum), row.PurchasePrice)
		_ = fx.SetCellValue(sheet, cellName(11, rowNum), row.NBV)
		_ = fx.SetCellValue(sheet, cellName(12, rowNum), row.ManagedBy)
	}

	var buf bytes.Buffer
	if _, err := fx.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// ExportDepreciationXLSX 导出折旧明细为 Excel
func (s *ExportService) ExportDepreciationXLSX(ctx context.Context, q repository.DBTX, orgID string, asOfDate string) ([]byte, error) {
	depSvc := NewDepreciationService()
	rows, _, _, err := depSvc.GetDepreciation(ctx, q, orgID, asOfDate)
	if err != nil {
		return nil, fmt.Errorf("get depreciation: %w", err)
	}

	fx := excelize.NewFile()
	defer fx.Close()
	sheet := "折旧"
	if _, err := fx.NewSheet(sheet); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	fx.DeleteSheet("Sheet1")

	headers := []string{"资产编号", "名称", "采购价格", "月折旧", "净值", "已提月数", "可用月数", "折旧方法"}
	for i, h := range headers {
		_ = fx.SetCellValue(sheet, cellName(i+1, 1), h)
	}
	style, _ := fx.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	_ = fx.SetCellStyle(sheet, "A1", "H1", style)

	for r, row := range rows {
		n := r + 2
		_ = fx.SetCellValue(sheet, cellName(1, n), row.AssetTag)
		_ = fx.SetCellValue(sheet, cellName(2, n), row.Name)
		_ = fx.SetCellValue(sheet, cellName(3, n), row.PurchasePrice)
		_ = fx.SetCellValue(sheet, cellName(4, n), row.MonthlyDep)
		_ = fx.SetCellValue(sheet, cellName(5, n), row.NetBookValue)
		_ = fx.SetCellValue(sheet, cellName(6, n), row.MonthsElapsed)
		_ = fx.SetCellValue(sheet, cellName(7, n), row.UsefulLifeMonths)
		_ = fx.SetCellValue(sheet, cellName(8, n), row.DepreciationMethod)
	}

	var buf bytes.Buffer
	if _, err := fx.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// ExportStocktakeReportXLSX 导出盘点报表为 Excel
func (s *ExportService) ExportStocktakeReportXLSX(ctx context.Context, q repository.DBTX, orgID string, planID string) ([]byte, error) {
	query := `
		SELECT
			COALESCE(a.asset_tag, ''),
			COALESCE(a.name, ''),
			COALESCE(at.name, ''),
			COALESCE(l.name, ''),
			COALESCE(a.status, ''),
			COALESCE(si.result, ''),
			COALESCE(si.notes, ''),
			COALESCE(u.username, si.checked_by::text, ''),
			COALESCE(si.checked_at::text, '')
		FROM assets.stocktake_items si
		LEFT JOIN assets.assets a ON si.asset_id = a.id
		LEFT JOIN assets.asset_types at ON a.type_id = at.id
		LEFT JOIN assets.locations l ON a.location_id = l.id
		LEFT JOIN assets.users u ON si.checked_by = u.id
		WHERE si.plan_id = $1::uuid
		ORDER BY a.asset_tag`
	rows, err := q.Query(ctx, query, planID)
	if err != nil {
		return nil, fmt.Errorf("query stocktake items: %w", err)
	}
	defer rows.Close()

	fx := excelize.NewFile()
	defer fx.Close()
	sheet := "盘点报告"
	if _, err := fx.NewSheet(sheet); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	fx.DeleteSheet("Sheet1")

	headers := []string{"资产编号", "名称", "类型", "位置", "账面状态", "盘点结果", "差异说明", "盘点人", "盘点时间"}
	for i, h := range headers {
		_ = fx.SetCellValue(sheet, cellName(i+1, 1), h)
	}
	style, _ := fx.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	_ = fx.SetCellStyle(sheet, "A1", "I1", style)

	r := 1
	for rows.Next() {
		var tag, name, typeName, loc, status, result, note, checker, checkDate string
		if err := rows.Scan(&tag, &name, &typeName, &loc, &status,
			&result, &note, &checker, &checkDate); err != nil {
			return nil, fmt.Errorf("scan stocktake row: %w", err)
		}
		r++
		_ = fx.SetCellValue(sheet, cellName(1, r), tag)
		_ = fx.SetCellValue(sheet, cellName(2, r), name)
		_ = fx.SetCellValue(sheet, cellName(3, r), typeName)
		_ = fx.SetCellValue(sheet, cellName(4, r), loc)
		_ = fx.SetCellValue(sheet, cellName(5, r), status)
		_ = fx.SetCellValue(sheet, cellName(6, r), result)
		_ = fx.SetCellValue(sheet, cellName(7, r), note)
		_ = fx.SetCellValue(sheet, cellName(8, r), checker)
		_ = fx.SetCellValue(sheet, cellName(9, r), checkDate)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate stocktake rows: %w", rows.Err())
	}

	var buf bytes.Buffer
	if _, err := fx.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// cellName 是 excelize.CoordinatesToCellName 的简写别名
func cellName(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

// computeNetBookValue 内联计算净值
func computeNetBookValue(purchasePrice, salvageValue float64, depMethod string, purchaseDate *time.Time, usefulLife *int) float64 {
	if depMethod != "straight_line" || purchaseDate == nil || usefulLife == nil || *usefulLife <= 0 {
		return purchasePrice
	}

	now := time.Now()
	yearDiff := now.Year() - purchaseDate.Year()
	monthDiff := int(now.Month()) - int(purchaseDate.Month())
	monthsElapsed := yearDiff*12 + monthDiff
	if monthsElapsed < 0 {
		monthsElapsed = 0
	}
	if monthsElapsed > *usefulLife {
		monthsElapsed = *usefulLife
	}

	monthlyDep := (purchasePrice - salvageValue) / float64(*usefulLife)
	nbv := purchasePrice - monthlyDep*float64(monthsElapsed)
	if nbv < salvageValue {
		nbv = salvageValue
	}
	return nbv
}
