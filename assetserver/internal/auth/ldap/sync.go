// Package ldap — 同步 (AD -> 本系统)
//
// 语义:
//   - 组织 (organizations 表): upsert, 按 name 唯一; AD 的 department 字符串直接作为一级组织。
//   - 用户 (users 表): upsert by (source='ldap', external_id=username);
//     AD 已不存在 -> 软删除 (复用 user_repo.SoftDelete) 或仅禁用 (SyncDisabledOnly)。
//   - 同步结果摘要写入 audit_log (不含密码)。
//
// 调度: 暴露 RunSyncOnce(ctx) 供 G4 调度器或 /admin/ldap/sync 手动触发;
//
//	本包不自建 ticker, 避免与 G4 的 internal/scheduler/ 抢目录。
package ldap

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncService LDAP 同步服务
type SyncService struct {
	client DirectoryClient
	pool   *pgxpool.Pool
}

// NewSyncService 构造同步服务
func NewSyncService(client DirectoryClient, pool *pgxpool.Pool) *SyncService {
	return &SyncService{client: client, pool: pool}
}

// SyncResult 同步结果 (可入审计/返回 API)
type SyncResult struct {
	Fetched       int `json:"fetched"`
	UsersCreated  int `json:"users_created"`
	UsersUpdated  int `json:"users_updated"`
	UsersDisabled int `json:"users_disabled"`
	OrgsCreated   int `json:"orgs_created"`
	Errors        int `json:"errors"`
}

// RunSyncOnce 执行一次单向 (AD -> DB) 同步
// actorID 触发者 (定时任务为系统用户 UUID; 手动为 admin id), 用于审计归属。
func (s *SyncService) RunSyncOnce(ctx context.Context, actorID, orgID string) (*SyncResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("ldap client not configured")
	}
	if s.pool == nil {
		return nil, fmt.Errorf("db pool not configured")
	}
	result := &SyncResult{}

	users, err := s.client.SearchUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	result.Fetched = len(users)

	// 收集 AD 中存在的部门 -> upsert 组织
	deptSet := make(map[string]struct{})
	for _, u := range users {
		if u.Department != "" {
			deptSet[u.Department] = struct{}{}
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. upsert organizations (按 name 唯一, 一级组织)
	deptToOrgID := make(map[string]string)
	for dept := range deptSet {
		// 先查一级组织 (depth=1) 是否已存在同名; 不存在再插入。
		// 不加全局 UNIQUE 约束: 同名但不同父的子组织允许共存。
		var orgIDVal string
		err := tx.QueryRow(ctx,
			`SELECT id::text FROM assets.organizations
			 WHERE name = $1 AND depth = 1 LIMIT 1`, dept,
		).Scan(&orgIDVal)
		if err == nil {
			deptToOrgID[dept] = orgIDVal
			continue
		}
		// 插入新一级组织
		err = tx.QueryRow(ctx,
			`INSERT INTO assets.organizations (name, path, depth, created_at, updated_at)
			 VALUES ($1, $2::ltree, 1, now(), now())
			 RETURNING id::text`,
			dept, "root."+sanitizeLTree(dept),
		).Scan(&orgIDVal)
		if err != nil {
			slog.Warn("ldap sync: org upsert failed", "dept", dept, "err", err)
			result.Errors++
			continue
		}
		deptToOrgID[dept] = orgIDVal
		result.OrgsCreated++
	}

	// 2. upsert users
	// LDAP 用户登录后无本地密码; password_hash 用占位符 (无法本地校验)。
	// 默认组织兜底 (无 department 的 AD 用户)
	const defaultOrgID = "00000000-0000-4000-a000-000000000001"
	seenExternalIDs := make(map[string]struct{}, len(users))
	for _, u := range users {
		seenExternalIDs[u.Username] = struct{}{}
		targetOrg := defaultOrgID
		if id, ok := deptToOrgID[u.Department]; ok {
			targetOrg = id
		}

		// upsert by (source='ldap', external_id=username)
		// 先查是否已有 LDAP 用户
		var existingID string
		_ = tx.QueryRow(ctx,
			`SELECT id::text FROM assets.users
			 WHERE source = 'ldap' AND external_id = $1 LIMIT 1`, u.Username,
		).Scan(&existingID)

		if existingID == "" {
			// 新增: 但需排除 username 与本地用户冲突
			var localConflict bool
			_ = tx.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM assets.users WHERE username = $1 AND source = 'local')`,
				u.Username,
			).Scan(&localConflict)
			if localConflict {
				slog.Warn("ldap sync: username conflicts with local user, skipped",
					"username", u.Username)
				result.Errors++
				continue
			}
			_, err := tx.Exec(ctx,
				`INSERT INTO assets.users
				   (org_id, username, password_hash, role, email, status, source,
				    external_id, display_name, dn, must_change_password, created_at, updated_at)
				 VALUES ($1, $2, '!', 'viewer', $3, 'active', 'ldap', $4, $5, $6, false, now(), now())`,
				targetOrg, u.Username, nullableStr(u.Email), u.Username,
				nullableStr(u.DisplayName), nullableStr(u.DN),
			)
			if err != nil {
				slog.Warn("ldap sync: user insert failed", "username", u.Username, "err", err)
				result.Errors++
				continue
			}
			result.UsersCreated++
		} else {
			// 更新 (含复活软删除)
			if _, err := tx.Exec(ctx,
				`UPDATE assets.users SET
				   email = COALESCE(NULLIF($2,''), email),
				   display_name = COALESCE(NULLIF($3,''), display_name),
				   dn = COALESCE(NULLIF($4,''), dn),
				   org_id = $5,
				   status = 'active',
				   deleted_at = NULL,
				   updated_at = now()
				 WHERE id = $1`,
				existingID, u.Email, u.DisplayName, u.DN, targetOrg,
			); err != nil {
				slog.Warn("ldap sync: user update failed", "username", u.Username, "err", err)
				result.Errors++
				continue
			}
			result.UsersUpdated++
		}
	}

	// 3. 处理 AD 中已不存在的 LDAP 用户 (软删除 / 禁用)
	// 拉取当前 DB 中所有 LDAP 活跃用户
	rows, err := tx.Query(ctx,
		`SELECT id::text, external_id FROM assets.users
		 WHERE source = 'ldap' AND deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list ldap users: %w", err)
	}
	var toDisable []struct{ id, ext string }
	for rows.Next() {
		var id, ext string
		if err := rows.Scan(&id, &ext); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan ldap user: %w", err)
		}
		if _, ok := seenExternalIDs[ext]; !ok {
			toDisable = append(toDisable, struct{ id, ext string }{id, ext})
		}
	}
	rows.Close()

	for _, d := range toDisable {
		if s.syncDisabledOnly() {
			_, _ = tx.Exec(ctx,
				`UPDATE assets.users SET status = 'disabled', updated_at = now() WHERE id = $1`, d.id)
		} else {
			_, _ = tx.Exec(ctx,
				`UPDATE assets.users SET deleted_at = now(), status = 'disabled', updated_at = now()
				 WHERE id = $1 AND deleted_at IS NULL`, d.id)
		}
		result.UsersDisabled++
	}

	// 4. 审计 (摘要入 audit_log, 不含凭据)
	// 用独立事务写审计, 与业务事务解耦: 审计失败仅记日志, 不影响主事务提交。
	summary, _ := json.Marshal(result)
	summary = truncateAuditMetadata(summary)
	if err := writeAuditSeparately(ctx, s.pool, audit.Entry{
		TableName: "users",
		RecordID:  "",
		Action:    "ldap_sync",
		OrgID:     orgID,
		ActorID:   actorID,
		NewValues: summary,
	}); err != nil {
		slog.Warn("ldap sync: write audit summary failed", "error", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit sync: %w", err)
	}
	slog.Info("ldap sync complete", "result", result)
	return result, nil
}

// truncateAuditMetadata 确保 audit metadata::text 不超过 4096 字节 (audit_log CHECK 约束)。
// metadata 列存的是完整 Entry JSON (含 NewValues), 故对 summary 预留 Entry 包装字段的空间。
// 超长时替换为占位 JSON, 防止 INSERT 失败导致事务中毒。
func truncateAuditMetadata(raw []byte) []byte {
	// 3500 字节留给 summary, 剩余 ~500 字节给 Entry 包装字段 (TableName/Action/OrgID/ActorID)
	const maxSummaryLen = 3500
	if len(raw) <= maxSummaryLen {
		return raw
	}
	return []byte(fmt.Sprintf(`{"truncated":true,"orig_len":%d}`, len(raw)))
}

// writeAuditSeparately 在独立事务中写入审计条目, 与业务事务解耦。
// 失败仅返回 error 由调用方记日志, 不回滚业务事务。
func writeAuditSeparately(ctx context.Context, pool *pgxpool.Pool, e audit.Entry) error {
	if pool == nil {
		return nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin audit tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := audit.Record(ctx, tx, e); err != nil {
		return fmt.Errorf("record audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit: %w", err)
	}
	return nil
}

// syncDisabledOnly 读取运行时配置 (从 client 转型获取)
// 这里通过 client.adClient.cfg 读取; 测试时可注入 mock。
func (s *SyncService) syncDisabledOnly() bool {
	if c, ok := s.client.(*adClient); ok {
		return c.cfg.SyncDisabledOnly
	}
	return false
}

// sanitizeLTree 将部门名转为合法 ltree 标签 (字母/数字/下划线)
func sanitizeLTree(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "dept"
	}
	return string(out)
}

// nullableStr 空串返回 NULL 友好参数: 仍传 string, 由 SQL COALESCE/NULLIF 处理
func nullableStr(s string) string { return s }
