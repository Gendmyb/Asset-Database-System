// Package repository — 审批请求持久化 (Wave 2 G7)
package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ApprovalRequestRow 审批请求行
type ApprovalRequestRow struct {
	ID           string          `json:"id"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	RequesterID  string          `json:"requester_id,omitempty"`
	OrgID        string          `json:"org_id"`
	Status       string          `json:"status"`
	CurrentStep  int             `json:"current_step"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Reason       *string         `json:"reason,omitempty"`
	CreatedAt    string          `json:"created_at"`
	DecidedAt    *string         `json:"decided_at,omitempty"`
	DecidedBy    *string         `json:"decided_by,omitempty"`
}

// ApprovalRepo 审批请求仓库 (无状态)
type ApprovalRepo struct{}

func NewApprovalRepo() *ApprovalRepo { return &ApprovalRepo{} }

// Create 插入一条 pending 审批请求
func (r *ApprovalRepo) Create(ctx context.Context, q DBTX, row *ApprovalRequestRow) (string, error) {
	id := uuid.New().String()
	var requesterArg interface{}
	if row.RequesterID != "" {
		requesterArg = row.RequesterID
	}
	_, err := q.Exec(ctx,
		`INSERT INTO assets.approval_requests
		 (id, resource_type, resource_id, requester_id, org_id, status, current_step, payload)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		id, row.ResourceType, row.ResourceID, requesterArg, row.OrgID,
		"pending", 1, row.Payload)
	if err != nil {
		return "", fmt.Errorf("create approval request: %w", err)
	}
	return id, nil
}

// Get 按 id 获取审批请求 (org_id 校验)
func (r *ApprovalRepo) Get(ctx context.Context, q DBTX, id, orgID string) (*ApprovalRequestRow, error) {
	row := &ApprovalRequestRow{}
	var requesterID, decidedBy, decidedAt *string
	err := q.QueryRow(ctx,
		`SELECT id, resource_type, resource_id, COALESCE(requester_id::text,''),
		        org_id::text, status, current_step, payload, COALESCE(reason,''),
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(decided_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        COALESCE(decided_by::text,'')
		 FROM assets.approval_requests
		 WHERE id=$1 AND org_id=$2`, id, orgID,
	).Scan(&row.ID, &row.ResourceType, &row.ResourceID, &requesterID,
		&row.OrgID, &row.Status, &row.CurrentStep, &row.Payload, &row.Reason,
		&row.CreatedAt, &decidedAt, &decidedBy)
	if err != nil {
		return nil, fmt.Errorf("get approval request: %w", err)
	}
	if requesterID != nil {
		row.RequesterID = *requesterID
	}
	if decidedBy != nil {
		row.DecidedBy = decidedBy
	}
	if decidedAt != nil {
		row.DecidedAt = decidedAt
	}
	return row, nil
}

// List 列出审批请求 (按 status 过滤; status 为空时返回全部)
func (r *ApprovalRepo) List(ctx context.Context, q DBTX, orgID, status string, limit int) ([]ApprovalRequestRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	qry := `SELECT id, resource_type, resource_id, COALESCE(requester_id::text,''),
		        org_id::text, status, current_step, payload, COALESCE(reason,''),
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        to_char(decided_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		        COALESCE(decided_by::text,'')
		 FROM assets.approval_requests
		 WHERE org_id=$1`
	args := []interface{}{orgID}
	if status != "" {
		qry += ` AND status=$2`
		args = append(args, status)
	}
	qry += ` ORDER BY created_at DESC LIMIT ` + fmt.Sprintf("%d", limit)

	rs, err := q.Query(ctx, qry, args...)
	if err != nil {
		return nil, fmt.Errorf("list approval requests: %w", err)
	}
	defer rs.Close()

	var results []ApprovalRequestRow
	for rs.Next() {
		var row ApprovalRequestRow
		var requesterID, decidedBy, decidedAt *string
		if err := rs.Scan(&row.ID, &row.ResourceType, &row.ResourceID, &requesterID,
			&row.OrgID, &row.Status, &row.CurrentStep, &row.Payload, &row.Reason,
			&row.CreatedAt, &decidedAt, &decidedBy); err != nil {
			return nil, fmt.Errorf("scan approval request: %w", err)
		}
		if requesterID != nil {
			row.RequesterID = *requesterID
		}
		if decidedBy != nil {
			row.DecidedBy = decidedBy
		}
		if decidedAt != nil {
			row.DecidedAt = decidedAt
		}
		results = append(results, row)
	}
	return results, rs.Err()
}

// UpdateStatus 更新状态 (approved/rejected) 并记录决策信息
func (r *ApprovalRepo) UpdateStatus(ctx context.Context, q DBTX, id, orgID, status, decidedBy, reason string) error {
	var reasonArg interface{}
	if reason != "" {
		reasonArg = reason
	}
	var decidedByArg interface{}
	if decidedBy != "" {
		decidedByArg = decidedBy
	}
	tag, err := q.Exec(ctx,
		`UPDATE assets.approval_requests
		 SET status=$1, decided_by=$2, decided_at=$3, reason=$4
		 WHERE id=$5 AND org_id=$6 AND status='pending'`,
		status, decidedByArg, time.Now(), reasonArg, id, orgID)
	if err != nil {
		return fmt.Errorf("update approval status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("approval request not found or not pending")
	}
	return nil
}

// HasPending 检查指定资源是否存在 pending 审批 (避免重复发起)
func (r *ApprovalRepo) HasPending(ctx context.Context, q DBTX, resourceType, resourceID, orgID string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM assets.approval_requests
		 WHERE resource_type=$1 AND resource_id=$2 AND org_id=$3 AND status='pending')`,
		resourceType, resourceID, orgID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check pending approval: %w", err)
	}
	return exists, nil
}
