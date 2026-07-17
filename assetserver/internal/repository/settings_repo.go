// Package repository — 系统设置数据访问层
package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SettingsRepo 系统设置仓库
type SettingsRepo struct {
	pool *pgxpool.Pool
}

func NewSettingsRepo(pool *pgxpool.Pool) *SettingsRepo {
	return &SettingsRepo{pool: pool}
}

// Get 获取设置值
func (r *SettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.pool.QueryRow(ctx,
		`SELECT value FROM assets.system_settings WHERE key = $1`, key,
	).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

// Set 更新设置值
func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO assets.system_settings (key, value, updated_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`,
		key, value, time.Now())
	return err
}

// GetAll 获取所有设置
func (r *SettingsRepo) GetAll(ctx context.Context) (map[string]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, value FROM assets.system_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, nil
}

// NextAssetTag 生成下一个资产编号 (前缀 + 自增序号)
func (r *SettingsRepo) NextAssetTag(ctx context.Context) (string, error) {
	prefix, err := r.Get(ctx, "asset_tag_prefix")
	if err != nil || prefix == "" {
		prefix = "AST-"
	}

	// 统计当前 org 下资产数 +1 作为序号
	var count int64
	err = r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM assets.assets WHERE deleted_at IS NULL`,
	).Scan(&count)
	if err != nil {
		count = 0
	}

	return formatTag(prefix, int(count+1)), nil
}

func formatTag(prefix string, seq int) string {
	return prefix + padInt(seq, 4)
}

func padInt(n, width int) string {
	s := ""
	for i := n; i > 0; i /= 10 {
		s = string(rune('0'+i%10)) + s
	}
	for len(s) < width {
		s = "0" + s
	}
	if s == "" {
		s = "0001"
	}
	return s
}
