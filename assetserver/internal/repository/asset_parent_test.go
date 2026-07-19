// Package repository — G8 外设挂载防循环单测
package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeParentChainDBTX 模拟 parent_asset_id 链: parentMap[id] = parentID (nil = 无父)
type fakeParentChainDBTX struct {
	parentMap map[string]*string // id -> parent_asset_id (nil 表示 NULL/无父)
	missing   map[string]bool    // id -> 不存在 (返回 ErrNoRows)
}

func (f *fakeParentChainDBTX) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *fakeParentChainDBTX) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeParentChainDBTX) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	// args[0] = id, args[1] = orgID
	id, _ := args[0].(string)
	if f.missing[id] {
		return &fakeParentRow{err: pgx.ErrNoRows}
	}
	parent, ok := f.parentMap[id]
	if !ok {
		return &fakeParentRow{err: pgx.ErrNoRows}
	}
	return &fakeParentRow{parent: parent}
}

type fakeParentRow struct {
	parent *string
	err    error
}

func (r *fakeParentRow) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	// IsDescendant 调用 Scan(&parent) 其中 parent 是 *string → dest[0] 是 **string
	ptr, ok := dest[0].(**string)
	if !ok {
		return errors.New("dest must be **string")
	}
	*ptr = r.parent // nil 表示 NULL, 否则指向字符串副本
	return nil
}

// strPtr 辅助
func strPtr(s string) *string { return &s }

// TestIsDescendant_Self 自身视为后代 (禁止自引用)
func TestIsDescendant_Self(t *testing.T) {
	repo := &AssetRepo{}
	db := &fakeParentChainDBTX{parentMap: map[string]*string{}}
	got, err := repo.IsDescendant(context.Background(), db, "A", "A", "org-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got {
		t.Fatal("asset should be its own descendant (self-reference guard)")
	}
}

// TestIsDescendant_Chain 链 X → Z → Y: Y 是 X 的后代
func TestIsDescendant_Chain(t *testing.T) {
	repo := &AssetRepo{}
	// Y.parent = Z, Z.parent = X, X.parent = nil
	db := &fakeParentChainDBTX{parentMap: map[string]*string{
		"Y": strPtr("Z"),
		"Z": strPtr("X"),
		"X": nil,
	}}
	got, err := repo.IsDescendant(context.Background(), db, "X", "Y", "org-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !got {
		t.Fatal("Y should be detected as descendant of X (cycle risk)")
	}
}

// TestIsDescendant_NotDescendant 链 X → Z → Y: X 不是 Y 的后代 (X 是根)
func TestIsDescendant_NotDescendant(t *testing.T) {
	repo := &AssetRepo{}
	db := &fakeParentChainDBTX{parentMap: map[string]*string{
		"Y": strPtr("Z"),
		"Z": strPtr("X"),
		"X": nil,
	}}
	// 检查 X 是否是 Y 的后代: 从 X 往上走, X.parent=nil → false
	got, err := repo.IsDescendant(context.Background(), db, "Y", "X", "org-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got {
		t.Fatal("X should NOT be descendant of Y (no cycle)")
	}
}

// TestIsDescendant_SiblingNotDescendant 兄弟节点互不为后代
func TestIsDescendant_SiblingNotDescendant(t *testing.T) {
	repo := &AssetRepo{}
	// A.parent=P, B.parent=P (兄弟)
	db := &fakeParentChainDBTX{parentMap: map[string]*string{
		"A": strPtr("P"),
		"B": strPtr("P"),
		"P": nil,
	}}
	got, err := repo.IsDescendant(context.Background(), db, "A", "B", "org-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got {
		t.Fatal("sibling B should NOT be descendant of A")
	}
}

// TestIsDescendant_MissingAsset 资产不存在 → false (无后代关系)
func TestIsDescendant_MissingAsset(t *testing.T) {
	repo := &AssetRepo{}
	db := &fakeParentChainDBTX{missing: map[string]bool{"ghost": true}}
	got, err := repo.IsDescendant(context.Background(), db, "A", "ghost", "org-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got {
		t.Fatal("missing asset should not be a descendant")
	}
}
