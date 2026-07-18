// Package repository — 数据访问层接口
// 对应 Phase B §3 DBTX 接口
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX 抽象数据库连接 (pgxpool.Pool | pgx.Tx)
// 允许 repo 方法在事务内外复用同一套实现
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}
