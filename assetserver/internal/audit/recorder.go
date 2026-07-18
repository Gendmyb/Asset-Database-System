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
	_, err = tx.Exec(ctx,
		`INSERT INTO assets.audit_log
		 (asset_id, org_id, user_id, action, metadata, prev_hash, hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.RecordID, e.OrgID, e.ActorID, e.Action,
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
	ActionTransition  = "lifecycle_changed"
)
