// Package repository — 通知规则与投递记录持久化 (Wave 2 G6)
package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// NotifyRuleRow 通知规则行
type NotifyRuleRow struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id,omitempty"`
	EventType string `json:"event_type"`
	Channel   string `json:"channel"`
	Target    string `json:"target,omitempty"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// NotifyDeliveryRow 通知投递记录行
type NotifyDeliveryRow struct {
	ID        string  `json:"id"`
	RuleID    string  `json:"rule_id,omitempty"`
	OrgID     string  `json:"org_id,omitempty"`
	EventType string  `json:"event_type"`
	Channel   string  `json:"channel"`
	Target    string  `json:"target,omitempty"`
	Status    string  `json:"status"`
	Attempts  int     `json:"attempts"`
	LastError *string `json:"last_error,omitempty"`
	CreatedAt string  `json:"created_at"`
}

// NotifyRepo 通知规则/投递记录仓库 (无状态)
type NotifyRepo struct{}

func NewNotifyRepo() *NotifyRepo { return &NotifyRepo{} }

// ListActiveRules 返回匹配指定事件类型的活跃规则 (含 org_id IS NULL 的全局规则)
func (r *NotifyRepo) ListActiveRules(ctx context.Context, q DBTX, orgID, eventType string) ([]NotifyRuleRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, COALESCE(org_id::text,''), event_type, channel, COALESCE(target,''), active,
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM assets.notify_rules
		 WHERE active=true
		   AND (org_id IS NULL OR org_id::text = $1)
		   AND (event_type = $2 OR event_type = '*')
		 ORDER BY created_at ASC`, orgID, eventType)
	if err != nil {
		return nil, fmt.Errorf("list notify rules: %w", err)
	}
	defer rows.Close()

	var results []NotifyRuleRow
	for rows.Next() {
		var row NotifyRuleRow
		if err := rows.Scan(&row.ID, &row.OrgID, &row.EventType, &row.Channel,
			&row.Target, &row.Active, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notify rule: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// CreateRule 插入一条通知规则
func (r *NotifyRepo) CreateRule(ctx context.Context, q DBTX, row *NotifyRuleRow) (string, error) {
	id := uuid.New().String()
	var orgArg interface{}
	if row.OrgID != "" {
		orgArg = row.OrgID
	}
	_, err := q.Exec(ctx,
		`INSERT INTO assets.notify_rules (id, org_id, event_type, channel, target, active)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		id, orgArg, row.EventType, row.Channel, row.Target, row.Active)
	if err != nil {
		return "", fmt.Errorf("create notify rule: %w", err)
	}
	return id, nil
}

// ListRules 列出所有规则 (admin 配置页用)
func (r *NotifyRepo) ListRules(ctx context.Context, q DBTX, orgID string) ([]NotifyRuleRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, COALESCE(org_id::text,''), event_type, channel, COALESCE(target,''), active,
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM assets.notify_rules
		 WHERE org_id IS NULL OR org_id::text = $1
		 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list notify rules: %w", err)
	}
	defer rows.Close()
	var results []NotifyRuleRow
	for rows.Next() {
		var row NotifyRuleRow
		if err := rows.Scan(&row.ID, &row.OrgID, &row.EventType, &row.Channel,
			&row.Target, &row.Active, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notify rule: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// GetRule 按 id 获取规则 (仅返回调用方有权访问的: 本组织规则或全局规则)
// 用于删除前判断规则是否为全局规则 (org_id IS NULL)。
func (r *NotifyRepo) GetRule(ctx context.Context, q DBTX, id, orgID string) (*NotifyRuleRow, error) {
	row := &NotifyRuleRow{}
	err := q.QueryRow(ctx,
		`SELECT id, COALESCE(org_id::text,''), event_type, channel, COALESCE(target,''), active,
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM assets.notify_rules
		 WHERE id=$1 AND (org_id IS NULL OR org_id::text = $2)`, id, orgID,
	).Scan(&row.ID, &row.OrgID, &row.EventType, &row.Channel,
		&row.Target, &row.Active, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get notify rule: %w", err)
	}
	return row, nil
}

// DeleteRule 删除一条规则 (org_id 校验: 仅可删本组织规则或全局规则;
// 全局规则 (org_id IS NULL) 的 super_admin 鉴权由调用方在 handler/service 层判断)
func (r *NotifyRepo) DeleteRule(ctx context.Context, q DBTX, id, orgID string) error {
	tag, err := q.Exec(ctx,
		`DELETE FROM assets.notify_rules
		 WHERE id=$1 AND (org_id::text = $2 OR org_id IS NULL)`, id, orgID)
	if err != nil {
		return fmt.Errorf("delete notify rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("notify rule not found")
	}
	return nil
}

// RecordDelivery 插入一条投递记录 (org_id 从 rule.OrgID 反查写入, 便于跨租户过滤)
func (r *NotifyRepo) RecordDelivery(ctx context.Context, q DBTX, ruleID, orgID, eventType, channel, target, status string, attempts int, lastError *string) (string, error) {
	id := uuid.New().String()
	var ruleArg interface{}
	if ruleID != "" {
		ruleArg = ruleID
	}
	var orgArg interface{}
	if orgID != "" {
		orgArg = orgID
	}
	_, err := q.Exec(ctx,
		`INSERT INTO assets.notify_deliveries (id, rule_id, org_id, event_type, channel, target, status, attempts, last_error)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, ruleArg, orgArg, eventType, channel, target, status, attempts, lastError)
	if err != nil {
		return "", fmt.Errorf("record notify delivery: %w", err)
	}
	return id, nil
}

// ListDeliveries 列出投递记录 (排查用)。
// 非 super_admin 仅看本组织 (org_id 匹配); super_admin (isSuperAdmin=true) 可看全部。
func (r *NotifyRepo) ListDeliveries(ctx context.Context, q DBTX, orgID string, isSuperAdmin bool, limit int) ([]NotifyDeliveryRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var (
		rows pgx.Rows
		err  error
	)
	if isSuperAdmin {
		rows, err = q.Query(ctx,
			`SELECT id, COALESCE(rule_id::text,''), COALESCE(org_id::text,''), event_type, channel, COALESCE(target,''),
			        status, attempts, last_error,
			        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			 FROM assets.notify_deliveries
			 ORDER BY created_at DESC LIMIT $1`, limit)
	} else {
		rows, err = q.Query(ctx,
			`SELECT id, COALESCE(rule_id::text,''), COALESCE(org_id::text,''), event_type, channel, COALESCE(target,''),
			        status, attempts, last_error,
			        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			 FROM assets.notify_deliveries
			 WHERE org_id::text = $1
			 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("list notify deliveries: %w", err)
	}
	defer rows.Close()
	var results []NotifyDeliveryRow
	for rows.Next() {
		var row NotifyDeliveryRow
		if err := rows.Scan(&row.ID, &row.RuleID, &row.OrgID, &row.EventType, &row.Channel,
			&row.Target, &row.Status, &row.Attempts, &row.LastError, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan notify delivery: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}
