// Package audit — 审计日志记录器 (不可变, 链式哈希)
// 对应 Phase B §6 审计 Recorder
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Entry 审计条目
type Entry struct {
	TableName string          `json:"table"`
	RecordID  string          `json:"record_id"`
	Action    string          `json:"action"` // "created"|"updated"|"deleted"|"assigned"|"released"|"transferred"|...
	OrgID     string          `json:"org_id"`
	ActorID   string          `json:"actor_id"`
	OldValues json.RawMessage `json:"old_values,omitempty"`
	NewValues json.RawMessage `json:"new_values,omitempty"`
}

// Record 在事务内写入审计日志 (含链式哈希)
// 001_init.sql 仅定义了 AFTER INSERT 不可变触发器, 未提供 BEFORE INSERT 哈希触发器,
// 因此由本函数在应用层计算链式哈希后直接插入
func Record(ctx context.Context, tx pgx.Tx, e Entry) error {
	// 1. 获取上一条审计记录的 hash 作为 prev_hash
	var prevHash string
	err := tx.QueryRow(ctx,
		`SELECT hash FROM assets.audit_log
		 WHERE asset_id = $1
		 ORDER BY created_at DESC, id DESC LIMIT 1`,
		e.RecordID,
	).Scan(&prevHash)
	if err != nil {
		prevHash = "" // 首条记录, 无前驱哈希
	}

	// 2. 计算新哈希: SHA256(prev_hash || entry_json)
	entryJSON, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal audit entry: %w", err)
	}

	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write(entryJSON)
	newHash := hex.EncodeToString(h.Sum(nil))

	// 3. INSERT INTO audit_log
	// asset_id / user_id 均为可空 UUID 列; 空串需转为 NULL, 否则 "invalid input syntax for type uuid"
	recordIDArg := interface{}(e.RecordID)
	if e.RecordID == "" {
		recordIDArg = nil
	}
	actorIDArg := interface{}(e.ActorID)
	if e.ActorID == "" {
		actorIDArg = nil
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO assets.audit_log
		 (asset_id, org_id, user_id, action, metadata, prev_hash, hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		recordIDArg, e.OrgID, actorIDArg, e.Action,
		entryJSON, prevHash, newHash,
	)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

// Action constants
const (
	ActionCreated     = "created"
	ActionUpdated     = "updated"
	ActionDeleted     = "deleted"
	ActionAssigned    = "assigned"
	ActionReleased    = "released"
	ActionTransferred = "transferred"
	ActionBorrowed    = "borrowed"
	ActionTransition  = "lifecycle_changed"
)

// AuditLogRow represents a single audit log entry
type AuditLogRow struct {
	ID        string          `json:"id"`
	AssetID   string          `json:"asset_id"`
	OrgID     string          `json:"org_id"`
	UserID    string          `json:"user_id"`
	Action    string          `json:"action"`
	Metadata  json.RawMessage `json:"metadata"`
	PrevHash  string          `json:"prev_hash"`
	Hash      string          `json:"hash"`
	CreatedAt string          `json:"created_at"`
}

// Querier minimal interface for querying audit logs (satisfied by pgxpool.Pool and pgx.Tx)
type Querier interface {
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

// QueryHistory queries audit_log for a given asset (org-scoped, ordered by time DESC)
func QueryHistory(ctx context.Context, q Querier, assetID, orgID string, limit int) ([]AuditLogRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := q.Query(ctx,
		`SELECT id, asset_id, org_id, user_id, action, metadata, prev_hash, hash,
		        to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM assets.audit_log
		 WHERE asset_id=$1 AND org_id=$2
		 ORDER BY created_at DESC, id DESC
		 LIMIT $3`, assetID, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("query audit log: %w", err)
	}
	defer rows.Close()

	var results []AuditLogRow
	for rows.Next() {
		var r AuditLogRow
		if err := rows.Scan(&r.ID, &r.AssetID, &r.OrgID, &r.UserID, &r.Action,
			&r.Metadata, &r.PrevHash, &r.Hash, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
