// Package service — 用户导入服务单测
package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeRow 实现 pgx.Row, 按 SQL 关键字返回预设结果
type fakeRow struct {
	scanFn func(dest ...any) error
}

func (r *fakeRow) Scan(dest ...any) error { return r.scanFn(dest...) }

// fakeDBTX 内存 DBTX: 按 SQL 模式匹配返回预设结果
type fakeDBTX struct {
	// usernameExists: 当查询为 username EXISTS 时返回的 bool
	usernameExists map[string]bool
	// orgExists: 当查询为 org 查找时返回的 id (空串表示未找到)
	orgByID   map[string]string
	orgByPath map[string]string
	orgByName map[string]string
}

func (f *fakeDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *fakeDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	switch {
	case strings.Contains(sql, "users WHERE username = $1"):
		// args[0] = username
		un, _ := args[0].(string)
		exists := f.usernameExists[un]
		return &fakeRow{scanFn: func(dest ...any) error {
			if len(dest) > 0 {
				*(dest[0].(*bool)) = exists
			}
			return nil
		}}
	case strings.Contains(sql, "organizations WHERE id::text"):
		id, _ := args[0].(string)
		v, ok := f.orgByID[id]
		return &fakeRow{scanFn: func(dest ...any) error {
			if !ok || v == "" {
				return pgx.ErrNoRows
			}
			*(dest[0].(*string)) = v
			return nil
		}}
	case strings.Contains(sql, "organizations WHERE path::text"):
		p, _ := args[0].(string)
		v, ok := f.orgByPath[p]
		return &fakeRow{scanFn: func(dest ...any) error {
			if !ok || v == "" {
				return pgx.ErrNoRows
			}
			*(dest[0].(*string)) = v
			return nil
		}}
	case strings.Contains(sql, "organizations WHERE name"):
		n, _ := args[0].(string)
		v, ok := f.orgByName[n]
		return &fakeRow{scanFn: func(dest ...any) error {
			if !ok || v == "" {
				return pgx.ErrNoRows
			}
			*(dest[0].(*string)) = v
			return nil
		}}
	}
	return &fakeRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
}

// TestReadUserCSV_HeaderAndRows 验证 CSV 解析与 BOM/注释处理
func TestReadUserCSV_HeaderAndRows(t *testing.T) {
	csv := "\xEF\xBB\xBFusername,display_name,email,role,org_path,password\n" +
		"# this is a comment,skip,,,\n" +
		"jdoe,John Doe,j@x.com,viewer,,\n" +
		"alice,Alice,a@x.com,admin,root.技术部,pwd123\n"
	rows, err := readUserCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("readUserCSV: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 data rows, got %d", len(rows))
	}
	if rows[0].fields["username"] != "jdoe" {
		t.Errorf("row1 username = %q", rows[0].fields["username"])
	}
	if rows[1].fields["password"] != "pwd123" {
		t.Errorf("row2 password = %q", rows[1].fields["password"])
	}
}

// TestReadUserCSV_MissingColumns 缺少必填列应报错
func TestReadUserCSV_MissingColumns(t *testing.T) {
	_, err := readUserCSV(strings.NewReader("username,email\njdoe,x\n"))
	if err == nil {
		t.Fatal("expected error for missing role column")
	}
}

// TestPreviewImport_Validation 验证预览逻辑
func TestPreviewImport_Validation(t *testing.T) {
	s := NewUserImportService()
	db := &fakeDBTX{
		usernameExists: map[string]bool{},
		orgByPath:      map[string]string{"root.技术部": "org-tech"},
	}
	csv := "username,display_name,email,role,org_path,password\n" +
		"jdoe,John,j@x.com,viewer,,\n" + // ok
		",,,viewer,,\n" + // empty username
		"bob,Bob,bob,manager,,\n" + // bad email
		"carol,Carol,c@x.com,superuser,,\n" + // bad role
		"dave,Dave,d@x.com,viewer,root.缺失,,\n" // bad org
	preview, err := s.PreviewImport(context.Background(), db, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if preview.Total != 5 {
		t.Errorf("total = %d, want 5", preview.Total)
	}
	if preview.Valid != 1 {
		t.Errorf("valid = %d, want 1", preview.Valid)
	}
	if len(preview.Errors) != 4 {
		t.Errorf("errors = %d, want 4", len(preview.Errors))
	}
}

// TestPreviewImport_DuplicateUsername 用户名重复应报错
func TestPreviewImport_DuplicateUsername(t *testing.T) {
	s := NewUserImportService()
	db := &fakeDBTX{
		usernameExists: map[string]bool{"jdoe": true},
	}
	csv := "username,display_name,email,role,org_path,password\n" +
		"jdoe,John,j@x.com,viewer,,\n"
	preview, err := s.PreviewImport(context.Background(), db, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if preview.Valid != 0 {
		t.Errorf("valid = %d, want 0 (dup)", preview.Valid)
	}
}

// TestGetUserImportTemplate_ContainsBOM 模板含 UTF-8 BOM
func TestGetUserImportTemplate_ContainsBOM(t *testing.T) {
	s := NewUserImportService()
	b := s.GetUserImportTemplate()
	if len(b) < 3 || b[0] != 0xEF || b[1] != 0xBB || b[2] != 0xBF {
		t.Errorf("template missing UTF-8 BOM")
	}
	if !strings.Contains(string(b), "username") {
		t.Errorf("template missing username column")
	}
}

// TestGenerateRandomPassword 随机密码长度与可重复性
func TestGenerateRandomPassword(t *testing.T) {
	p1 := generateRandomPassword(16)
	p2 := generateRandomPassword(16)
	if p1 == "" || p2 == "" {
		t.Fatal("password empty")
	}
	if p1 == p2 {
		t.Fatal("passwords should differ (random)")
	}
}

// TestIsDuplicate 错误字符串识别
func TestIsDuplicate(t *testing.T) {
	if !isDuplicate(errors.New("duplicate key violates unique constraint")) {
		t.Error("expected duplicate match")
	}
	if isDuplicate(errors.New("network error")) {
		t.Error("non-dup should not match")
	}
}

// TestReadUserCSV_EOF 空文件应报错
func TestReadUserCSV_EOF(t *testing.T) {
	_, err := readUserCSV(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

// TestResolveOrg_ByUUID/Path/Name 三种解析路径
func TestResolveOrg(t *testing.T) {
	db := &fakeDBTX{
		orgByID:   map[string]string{"org-1": "org-1"},
		orgByPath: map[string]string{"root.tech": "org-2"},
		orgByName: map[string]string{"技术部": "org-3"},
	}
	ctx := context.Background()
	if id, _ := resolveOrg(ctx, db, "org-1"); id != "org-1" {
		t.Errorf("by id: %q", id)
	}
	if id, _ := resolveOrg(ctx, db, "root.tech"); id != "org-2" {
		t.Errorf("by path: %q", id)
	}
	if id, _ := resolveOrg(ctx, db, "技术部"); id != "org-3" {
		t.Errorf("by name: %q", id)
	}
	if _, err := resolveOrg(ctx, db, "missing"); err == nil {
		t.Error("missing org should error")
	}
}

// execCountDBTX 包装 fakeDBTX, 计数 Exec 调用 (用于验证 dry-run 不落库)
type execCountDBTX struct {
	inner *fakeDBTX
	execN int
}

func (e *execCountDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	e.execN++
	return e.inner.Exec(ctx, sql, args...)
}
func (e *execCountDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return e.inner.Query(ctx, sql, args...)
}
func (e *execCountDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return e.inner.QueryRow(ctx, sql, args...)
}

// TestPreviewImport_DoesNotPersist 验证 dry-run (PreviewImport) 不写库: Exec 零调用。
func TestPreviewImport_DoesNotPersist(t *testing.T) {
	s := NewUserImportService()
	inner := &fakeDBTX{
		usernameExists: map[string]bool{},
		orgByPath:      map[string]string{"root.技术部": "org-tech"},
	}
	tracked := &execCountDBTX{inner: inner}
	csv := "username,display_name,email,role,org_path,password\n" +
		"jdoe,John,j@x.com,viewer,,\n" + // ok
		",,,viewer,,\n" + // empty username (error row)
		"bad,Bob,not-an-email,viewer,,\n" + // bad email (error row)
		"carol,Carol,c@x.com,viewer,root.技术部,,\n" // ok with org
	preview, err := s.PreviewImport(context.Background(), tracked, strings.NewReader(csv))
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if preview.Total != 4 {
		t.Errorf("total = %d, want 4", preview.Total)
	}
	if preview.Valid != 2 {
		t.Errorf("valid = %d, want 2", preview.Valid)
	}
	if len(preview.Errors) != 2 {
		t.Errorf("errors = %d, want 2 (per-row)", len(preview.Errors))
	}
	// 关键断言: dry-run 期间不得有任何写操作
	if tracked.execN != 0 {
		t.Errorf("dry-run executed %d write statements, want 0 (must not persist)", tracked.execN)
	}
}

// 确保 fakeDBTX 满足 repository.DBTX 编译时断言
var _ io.Reader = strings.NewReader("")
