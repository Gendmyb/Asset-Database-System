// Package service — XLSX 导出单测 (Wave 1 G5)
package service

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/xuri/excelize/v2"
)

// xlsxRows 模拟 pgx.Rows 用于资产导出查询
type xlsxRows struct {
	rows [][]interface{}
	idx  int
}

func (r *xlsxRows) Next() bool {
	ok := r.idx < len(r.rows)
	if ok {
		r.idx++
	}
	return ok
}
func (r *xlsxRows) Scan(dest ...interface{}) error {
	row := r.rows[r.idx-1]
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			*d = row[i].(string)
		case *float64:
			*d = row[i].(float64)
		case **time.Time:
			if row[i] == nil {
				*d = nil
			} else {
				t := row[i].(time.Time)
				*d = &t
			}
		case **int:
			if row[i] == nil {
				*d = nil
			} else {
				v := row[i].(int)
				*d = &v
			}
		}
	}
	return nil
}
func (r *xlsxRows) Close()                                       {}
func (r *xlsxRows) Err() error                                   { return nil }
func (r *xlsxRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *xlsxRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *xlsxRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *xlsxRows) RawValues() [][]byte                          { return nil }
func (r *xlsxRows) Conn() *pgx.Conn                              { return nil }

// xlsxDBTX 模拟 repository.DBTX (Query 返回预设 Rows)
type xlsxDBTX struct {
	rows pgx.Rows
}

func (f *xlsxDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (f *xlsxDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return f.rows, nil
}
func (f *xlsxDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return nil
}

func TestExportAssetsXLSX_PrefixAndReopen(t *testing.T) {
	// 构造两行资产数据 (与 queryExportAssets SELECT 列顺序对齐)
	now := time.Now()
	rows := &xlsxRows{
		rows: [][]interface{}{
			{"AST-001", "MacBook", "笔记本", "SN001", "Apple", "M4",
				"北京", "available", "utilization", 10000.0, 1000.0,
				"straight_line", now, 36, "admin"},
			{"AST-002", "显示器", "外设", "SN002", "Dell", "U2723",
				"上海", "assigned", "deployment", 3000.0, 0.0,
				"none", nil, nil, ""},
		},
	}
	dbtx := &xlsxDBTX{rows: rows}

	svc := NewExportService()
	data, err := svc.ExportAssetsXLSX(context.Background(), dbtx, "org-1", repository.AssetFilter{OrgID: "org-1"})
	if err != nil {
		t.Fatalf("ExportAssetsXLSX: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty xlsx output")
	}

	// 校验开头 PK zip 签名 (xlsx 是 zip 容器)
	if !bytes.HasPrefix(data, []byte{0x50, 0x4B, 0x03, 0x04}) {
		t.Fatalf("xlsx missing PK zip signature, got prefix % x", data[:4])
	}

	// 校验可被 excelize 重新打开
	fx, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("excelize reopen: %v", err)
	}
	defer fx.Close()

	sheets := fx.GetSheetList()
	if len(sheets) == 0 {
		t.Fatal("no sheets in xlsx")
	}
	// 表头 A1 = "编号"
	a1, err := fx.GetCellValue(sheets[0], "A1")
	if err != nil {
		t.Fatalf("GetCellValue A1: %v", err)
	}
	if a1 != "编号" {
		t.Fatalf("A1 = %q, want 编号", a1)
	}
	// 第一行数据 A2 = AST-001
	a2, err := fx.GetCellValue(sheets[0], "A2")
	if err != nil {
		t.Fatalf("GetCellValue A2: %v", err)
	}
	if a2 != "AST-001" {
		t.Fatalf("A2 = %q, want AST-001", a2)
	}
}
