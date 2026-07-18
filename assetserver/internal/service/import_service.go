// Package service — CSV 导入服务
// Phase H Step 4: CSV 导入 (模板/预览/执行)
package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ImportService 导入服务
type ImportService struct {
	settingsRepo *repository.SettingsRepo
	assetRepo    *repository.AssetRepo
}

func NewImportService(settingsRepo *repository.SettingsRepo, assetRepo *repository.AssetRepo) *ImportService {
	return &ImportService{settingsRepo: settingsRepo, assetRepo: assetRepo}
}

// importColumns 导入模板列名
var importColumns = []string{
	"name", "asset_tag", "type_name", "serial_number", "manufacturer",
	"model", "location_name", "purchase_price", "purchase_date",
	"supplier", "useful_life_months", "depreciation_method",
}

// GetImportTemplate 返回 UTF-8 BOM CSV 模板
func (s *ImportService) GetImportTemplate() []byte {
	var buf strings.Builder
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM
	w := csv.NewWriter(&buf)
	_ = w.Write(importColumns)
	w.Flush()
	return []byte(buf.String())
}

// ImportRowError 单行校验错误
type ImportRowError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ImportPreview 导入预览结果
type ImportPreview struct {
	Total  int              `json:"total"`
	Valid  int              `json:"valid"`
	Errors []ImportRowError `json:"errors"`
}

// ImportResult 执行导入结果
type ImportResult struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Errors  int `json:"errors"`
}

// PreviewImport 预览 CSV 导入
func (s *ImportService) PreviewImport(ctx context.Context, q repository.DBTX, orgID string, reader io.Reader) (*ImportPreview, error) {
	r := csv.NewReader(reader)
	r.TrimLeadingSpace = true
	r.LazyQuotes = true

	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}
	colIndex := buildColumnIndex(headers)
	if colIndex["name"] < 0 {
		return nil, fmt.Errorf("missing required column: name")
	}

	result := &ImportPreview{Errors: make([]ImportRowError, 0)}
	rowNum := 1

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, ImportRowError{
				Row: rowNum, Field: "", Message: fmt.Sprintf("parse error: %v", err),
			})
			rowNum++
			result.Total++
			continue
		}

		rowNum++
		result.Total++

		// 名称必填
		name := getCol(record, colIndex, "name")
		if name == "" {
			result.Errors = append(result.Errors, ImportRowError{
				Row: rowNum, Field: "name", Message: "名称不能为空",
			})
			continue
		}

		// 验证资产类型名
		typeName := getCol(record, colIndex, "type_name")
		if typeName == "" {
			result.Errors = append(result.Errors, ImportRowError{
				Row: rowNum, Field: "type_name", Message: "资产类型不能为空",
			})
			continue
		}
		typeID, err := s.resolveAssetType(ctx, q, typeName)
		if err != nil || typeID == "" {
			result.Errors = append(result.Errors, ImportRowError{
				Row: rowNum, Field: "type_name", Message: fmt.Sprintf("资产类型不存在: %s", typeName),
			})
			continue
		}

		// asset_tag 如果填写，检查重复
		assetTag := getCol(record, colIndex, "asset_tag")
		if assetTag != "" {
			exists, err := s.tagExists(ctx, q, assetTag)
			if err != nil {
				result.Errors = append(result.Errors, ImportRowError{
					Row: rowNum, Field: "asset_tag", Message: fmt.Sprintf("查询编号失败: %v", err),
				})
				continue
			}
			if exists {
				result.Errors = append(result.Errors, ImportRowError{
					Row: rowNum, Field: "asset_tag", Message: fmt.Sprintf("资产编号已存在: %s", assetTag),
				})
				continue
			}
		}

		// 位置名 -> UUID
		locName := getCol(record, colIndex, "location_name")
		if locName != "" {
			_, err := s.resolveLocation(ctx, q, orgID, locName)
			if err != nil {
				result.Errors = append(result.Errors, ImportRowError{
					Row: rowNum, Field: "location_name", Message: fmt.Sprintf("位置不存在: %s", locName),
				})
				continue
			}
		}

		// purchase_price 格式
		ppStr := getCol(record, colIndex, "purchase_price")
		if ppStr != "" {
			if _, err := strconv.ParseFloat(ppStr, 64); err != nil {
				result.Errors = append(result.Errors, ImportRowError{
					Row: rowNum, Field: "purchase_price", Message: fmt.Sprintf("采购价格格式错误: %s", ppStr),
				})
				continue
			}
		}

		// purchase_date 格式
		pdStr := getCol(record, colIndex, "purchase_date")
		if pdStr != "" {
			if _, err := time.Parse("2006-01-02", pdStr); err != nil {
				result.Errors = append(result.Errors, ImportRowError{
					Row: rowNum, Field: "purchase_date", Message: fmt.Sprintf("日期格式错误(需YYYY-MM-DD): %s", pdStr),
				})
				continue
			}
		}

		// useful_life_months 整数
		ulStr := getCol(record, colIndex, "useful_life_months")
		if ulStr != "" {
			if v, err := strconv.Atoi(ulStr); err != nil || v <= 0 {
				result.Errors = append(result.Errors, ImportRowError{
					Row: rowNum, Field: "useful_life_months", Message: fmt.Sprintf("可用月数必须为正整数: %s", ulStr),
				})
				continue
			}
		}

		// depreciation_method 校验
		dm := getCol(record, colIndex, "depreciation_method")
		if dm != "" && dm != "straight_line" && dm != "none" {
			result.Errors = append(result.Errors, ImportRowError{
				Row: rowNum, Field: "depreciation_method", Message: fmt.Sprintf("折旧方法无效(应为 straight_line 或 none): %s", dm),
			})
			continue
		}

		result.Valid++
	}

	return result, nil
}

// ExecuteImport 执行 CSV 导入 (事务内逐行创建 + 取号 + 审计)
func (s *ImportService) ExecuteImport(ctx context.Context, pool *pgxpool.Pool, orgID string, actorID string, reader io.Reader) (*ImportResult, error) {
	r := csv.NewReader(reader)
	r.TrimLeadingSpace = true
	r.LazyQuotes = true

	headers, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read csv header: %w", err)
	}
	colIndex := buildColumnIndex(headers)
	if colIndex["name"] < 0 {
		return nil, fmt.Errorf("missing required column: name")
	}

	// 预读所有记录
	type rawRecord struct {
		rowNum int
		fields map[string]string
	}
	var records []rawRecord
	rowNum := 1
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv row %d: %w", rowNum+1, err)
		}
		rowNum++
		m := make(map[string]string)
		for k := range colIndex {
			m[k] = getCol(record, colIndex, k)
		}
		records = append(records, rawRecord{rowNum: rowNum, fields: m})
	}

	result := &ImportResult{Total: len(records)}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, rec := range records {
		name := rec.fields["name"]
		typeName := rec.fields["type_name"]
		typeID, err := s.resolveAssetType(ctx, tx, typeName)
		if err != nil || typeID == "" {
			result.Errors++
			continue
		}

		assetTag := rec.fields["asset_tag"]
		if assetTag == "" {
			tag, err := s.settingsRepo.NextAssetTag(ctx, tx, orgID)
			if err != nil {
				result.Errors++
				continue
			}
			assetTag = tag
		}

		// 解析 purchase_price
		var purchasePrice *float64
		if ppStr := rec.fields["purchase_price"]; ppStr != "" {
			if v, err := strconv.ParseFloat(ppStr, 64); err == nil {
				purchasePrice = &v
			}
		}

		// 解析 purchase_date
		var purchaseDate *time.Time
		if pdStr := rec.fields["purchase_date"]; pdStr != "" {
			if t, err := time.Parse("2006-01-02", pdStr); err == nil {
				purchaseDate = &t
			}
		}

		// 解析 supplier
		var supplier *string
		if s := rec.fields["supplier"]; s != "" {
			supplier = &s
		}

		// 解析 useful_life_months
		var usefulLife *int
		if ulStr := rec.fields["useful_life_months"]; ulStr != "" {
			if v, err := strconv.Atoi(ulStr); err == nil {
				usefulLife = &v
			}
		}

		depMethod := rec.fields["depreciation_method"]
		if depMethod == "" {
			depMethod = "none"
		}

		// 解析 serial_number / manufacturer / model
		var serialNumber *string
		if s := rec.fields["serial_number"]; s != "" {
			serialNumber = &s
		}
		var manufacturer *string
		if s := rec.fields["manufacturer"]; s != "" {
			manufacturer = &s
		}
		var model *string
		if s := rec.fields["model"]; s != "" {
			model = &s
		}

		now := time.Now()
		row := &repository.AssetRow{
			ID:                 uuid.New().String(),
			AssetTag:           assetTag,
			Name:               name,
			TypeID:             typeID,
			OrgID:              orgID,
			SerialNumber:       serialNumber,
			Manufacturer:       manufacturer,
			Model:              model,
			LifecycleState:     "procurement",
			Status:             "available",
			Properties:         json.RawMessage("{}"),
			Version:            1,
			CreatedAt:          now,
			UpdatedAt:          now,
			PurchasePrice:      purchasePrice,
			PurchaseDate:       purchaseDate,
			Supplier:           supplier,
			DepreciationMethod: depMethod,
			UsefulLifeMonths:   usefulLife,
			SalvageValue:       0,
		}

		if err := s.assetRepo.Create(ctx, tx, row); err != nil {
			result.Errors++
			continue
		}

		// 审计日志
		newVals, _ := json.Marshal(row)
		if err := audit.Record(ctx, tx, audit.Entry{
			TableName: "assets",
			RecordID:  row.ID,
			Action:    audit.ActionCreated,
			OrgID:     orgID,
			ActorID:   actorID,
			NewValues: newVals,
		}); err != nil {
			result.Errors++
			continue
		}

		result.Created++
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit import: %w", err)
	}

	return result, nil
}

// resolveAssetType 通过名称查找资产类型 UUID
func (s *ImportService) resolveAssetType(ctx context.Context, q repository.DBTX, typeName string) (string, error) {
	if typeName == "" {
		return "", nil
	}
	var id string
	err := q.QueryRow(ctx,
		`SELECT id FROM assets.asset_types WHERE name = $1 LIMIT 1`, typeName,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("asset type not found: %s", typeName)
	}
	return id, nil
}

// resolveLocation 通过名称查找位置 UUID (限定 org)
func (s *ImportService) resolveLocation(ctx context.Context, q repository.DBTX, orgID string, locName string) (string, error) {
	var id string
	err := q.QueryRow(ctx,
		`SELECT id FROM assets.locations WHERE org_id = $1 AND name = $2 LIMIT 1`,
		orgID, locName,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("location not found: %s", locName)
	}
	return id, nil
}

// tagExists 检查资产编号是否已存在
func (s *ImportService) tagExists(ctx context.Context, q repository.DBTX, tag string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM assets.assets WHERE asset_tag = $1 AND deleted_at IS NULL)`,
		tag,
	).Scan(&exists)
	return exists, err
}

// buildColumnIndex 构建列名→索引映射
func buildColumnIndex(headers []string) map[string]int {
	m := make(map[string]int)
	for i, h := range headers {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

// getCol 按列名取值，列不存在返回空串
func getCol(record []string, colIndex map[string]int, col string) string {
	idx, ok := colIndex[col]
	if !ok || idx < 0 || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}
