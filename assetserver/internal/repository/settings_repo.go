// Package repository — 系统设置数据访问层
package repository

import (
	"context"
	"fmt"
	"time"
)

// SettingsRepo 系统设置仓库 (无状态 — DBTX 由调用方传入)
type SettingsRepo struct{}

func NewSettingsRepo() *SettingsRepo {
	return &SettingsRepo{}
}

// Get 获取设置值
func (r *SettingsRepo) Get(ctx context.Context, q DBTX, key string) (string, error) {
	var value string
	err := q.QueryRow(ctx,
		`SELECT value FROM assets.system_settings WHERE key = $1`, key,
	).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

// Set 更新设置值
func (r *SettingsRepo) Set(ctx context.Context, q DBTX, key, value string) error {
	_, err := q.Exec(ctx,
		`INSERT INTO assets.system_settings (key, value, updated_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = $3`,
		key, value, time.Now())
	return err
}

// GetAll 获取所有设置
func (r *SettingsRepo) GetAll(ctx context.Context, q DBTX) (map[string]string, error) {
	rows, err := q.Query(ctx, `SELECT key, value FROM assets.system_settings ORDER BY key`)
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

// NextAssetTag 生成下一个资产编号 (原子取号 via doc_sequences)
// 对齐到"已存在最大编号 + 1", 兼容历史 COUNT+1 路径与软删除遗留的编号,
// 避免与已存在(含软删除, 因 asset_tag 为全表 UNIQUE)的编号撞车。
func (r *SettingsRepo) NextAssetTag(ctx context.Context, q DBTX, orgID string) (string, error) {
	prefix, err := r.Get(ctx, q, "asset_tag_prefix")
	if err != nil || prefix == "" {
		prefix = "AST-"
	}

	var endSeq int64
	err = q.QueryRow(ctx, `
		WITH existing AS (
			SELECT COALESCE(MAX(NULLIF(regexp_replace(asset_tag, '\D', '', 'g'), '')::bigint), 0) AS max_seq
			FROM assets.assets WHERE org_id = $1
		)
		INSERT INTO assets.doc_sequences (org_id, scope, next_seq)
		VALUES ($1, 'asset', GREATEST(1, (SELECT max_seq FROM existing) + 1))
		ON CONFLICT (org_id, scope) DO UPDATE
			SET next_seq = GREATEST(assets.doc_sequences.next_seq + 1, (SELECT max_seq FROM existing) + 1)
		RETURNING next_seq`, orgID).Scan(&endSeq)
	if err != nil {
		return "", fmt.Errorf("claim asset sequence: %w", err)
	}

	return formatTag(prefix, int(endSeq)), nil
}

// NextBatchTags 批量生成资产编号 (使用 doc_sequences 原子取号)
// Phase E: 防止批量创建时的并发重号问题
// 同样对齐到已存在最大编号, 兼容历史/软删除遗留。
func (r *SettingsRepo) NextBatchTags(ctx context.Context, q DBTX, orgID string, count int) ([]string, error) {
	prefix, err := r.Get(ctx, q, "asset_tag_prefix")
	if err != nil || prefix == "" {
		prefix = "AST-"
	}

	var endSeq int64
	err = q.QueryRow(ctx, `
		WITH existing AS (
			SELECT COALESCE(MAX(NULLIF(regexp_replace(asset_tag, '\D', '', 'g'), '')::bigint), 0) AS max_seq
			FROM assets.assets WHERE org_id = $1
		)
		INSERT INTO assets.doc_sequences (org_id, scope, next_seq)
		VALUES ($1, 'asset', GREATEST(1, (SELECT max_seq FROM existing) + $2))
		ON CONFLICT (org_id, scope) DO UPDATE
			SET next_seq = GREATEST(assets.doc_sequences.next_seq + $2, (SELECT max_seq FROM existing) + $2)
		RETURNING next_seq`,
		orgID, count,
	).Scan(&endSeq)
	if err != nil {
		return nil, fmt.Errorf("claim sequence: %w", err)
	}

	startSeq := endSeq - int64(count) + 1
	tags := make([]string, count)
	for i := 0; i < count; i++ {
		tags[i] = formatTag(prefix, int(startSeq)+i)
	}
	return tags, nil
}

// NextMaintenanceOrderNo 生成下一个维修工单编号 (scope='maintenance', 前缀 'MNT-')
func (r *SettingsRepo) NextMaintenanceOrderNo(ctx context.Context, q DBTX, orgID string) (string, error) {
	var endSeq int64
	err := q.QueryRow(ctx,
		`INSERT INTO assets.doc_sequences (org_id, scope, next_seq)
		 VALUES ($1, 'maintenance', $2)
		 ON CONFLICT (org_id, scope) DO UPDATE SET next_seq = assets.doc_sequences.next_seq + $2
		 RETURNING next_seq`,
		orgID, 1,
	).Scan(&endSeq)
	if err != nil {
		return "", fmt.Errorf("claim maintenance sequence: %w", err)
	}

	return "MNT-" + padInt(int(endSeq), 4), nil
}

// NextStocktakePlanNo 生成下一个盘点计划编号 (scope='stocktake', 前缀 'STK-')
func (r *SettingsRepo) NextStocktakePlanNo(ctx context.Context, q DBTX, orgID string) (string, error) {
	var endSeq int64
	err := q.QueryRow(ctx,
		`INSERT INTO assets.doc_sequences (org_id, scope, next_seq)
		 VALUES ($1, 'stocktake', $2)
		 ON CONFLICT (org_id, scope) DO UPDATE SET next_seq = assets.doc_sequences.next_seq + $2
		 RETURNING next_seq`,
		orgID, 1,
	).Scan(&endSeq)
	if err != nil {
		return "", fmt.Errorf("claim stocktake sequence: %w", err)
	}

	return "STK-" + padInt(int(endSeq), 4), nil
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
