// Package ldap — 同步 (AD -> 本系统) Wave 3 T3 重写
//
// 语义:
//   - 按启用的安全组 (ad_group_mappings.sync_enabled=true) 圈人: 不按组过滤则不拉用户
//   - 用户角色来自 resolveRoleForGroups, 不再硬编码 viewer
//   - manual_override=true 时仅刷新 profile 字段, 不覆盖 role/status/scope
//   - AD 禁用 (userAccountControl bit 1) → 本地禁用
//   - 移出所有启用组的用户 → 禁用 (不软删, 保留审计)
//   - 组织: 按 department 一级拍平 (与旧版一致)
//   - 审计摘要含增/改/禁/跳过计数
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
	"strings"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/audit"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SyncService LDAP 同步服务
type SyncService struct {
	client       DirectoryClient
	pool         *pgxpool.Pool
	groupRepo    *repository.ADGroupRepo
}

// NewSyncService 构造同步服务 (Wave 3 T3: 新增 groupRepo 用于组映射)
func NewSyncService(client DirectoryClient, pool *pgxpool.Pool) *SyncService {
	return &SyncService{client: client, pool: pool, groupRepo: repository.NewADGroupRepo()}
}

// SyncResult 同步结果 (可入审计/返回 API) — Wave 3 增强字段
type SyncResult struct {
	Fetched        int `json:"fetched"`
	UsersCreated   int `json:"users_created"`
	UsersUpdated   int `json:"users_updated"`
	UsersDisabled  int `json:"users_disabled"`
	UsersSkipped   int `json:"users_skipped"`  // manual_override 跳过
	OrgsCreated    int `json:"orgs_created"`
	Errors         int `json:"errors"`
	ADUsersDisabled int `json:"ad_users_disabled"` // AD 中已禁用的用户数
}

// adUserAccountDisabled AD userAccountControl 中禁用标志 (bit 1)
const adUserAccountDisabled = 0x2

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

	// 1. 加载启用的组映射
	tx0, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	mappings, err := s.groupRepo.ListEnabled(ctx, tx0)
	tx0.Rollback(ctx) // 读操作不需要保持事务
	if err != nil {
		return nil, fmt.Errorf("load group mappings: %w", err)
	}
	if len(mappings) == 0 {
		slog.Warn("ldap sync: no enabled group mappings, skipping (no groups to sync)")
		return result, nil
	}

	// 2. 收集启用组的 DN
	groupDNs := make([]string, 0, len(mappings))
	for _, m := range mappings {
		groupDNs = append(groupDNs, m.GroupDN)
	}

	// 3. 按组拉取 AD 用户
	users, err := s.client.SearchUsers(ctx, groupDNs)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	result.Fetched = len(users)

	// 4. 收集部门 -> upsert 组织 (一级拍平, 与旧版一致)
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

	// 5. upsert organizations
	deptToOrgID := make(map[string]string)
	for dept := range deptSet {
		var orgIDVal string
		err := tx.QueryRow(ctx,
			`SELECT id::text FROM assets.organizations
			 WHERE name = $1 AND depth = 1 LIMIT 1`, dept,
		).Scan(&orgIDVal)
		if err == nil {
			deptToOrgID[dept] = orgIDVal
			continue
		}
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

	// 6. upsert users (按组成员 → 角色解析)
	const defaultOrgID = "00000000-0000-4000-a000-000000000001"
	seenExternalIDs := make(map[string]struct{}, len(users))
	for _, u := range users {
		seenExternalIDs[u.Username] = struct{}{}

		// 6a. AD 禁用检查
		if u.UserAccountControl&adUserAccountDisabled != 0 {
			result.ADUsersDisabled++
			slog.Info("ldap sync: AD disabled user", "username", u.Username)
			// 仍然更新本地状态为 disabled (不跳过)
		}

		// 6b. 角色解析
		resolved := ResolveRoleForGroups(u.MemberOf, mappings)

		targetOrg := defaultOrgID
		if id, ok := deptToOrgID[u.Department]; ok {
			targetOrg = id
		}

		// 6c. 查找现有 LDAP 用户
		var existingID string
		_ = tx.QueryRow(ctx,
			`SELECT id::text FROM assets.users
			 WHERE source = 'ldap' AND external_id = $1 LIMIT 1`, u.Username,
		).Scan(&existingID)

		if existingID == "" {
			// 6d. 新建 — 检查本地冲突
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
			// 确定新用户的 status: AD disabled → disabled
			status := "active"
			if u.UserAccountControl&adUserAccountDisabled != 0 {
				status = "disabled"
			}
			_, err := tx.Exec(ctx,
				`INSERT INTO assets.users
				   (org_id, username, password_hash, role, email, status, source,
				    external_id, display_name, dn, data_scope, must_change_password, created_at, updated_at)
				 VALUES ($1, $2, '!', $3, $4, $5, 'ldap', $6, $7, $8, $9, false, now(), now())`,
				targetOrg, u.Username, resolved.Role, nullableStr(u.Email), status,
				u.Username, nullableStr(u.DisplayName), nullableStr(u.DN), resolved.DataScope,
			)
			if err != nil {
				slog.Warn("ldap sync: user insert failed", "username", u.Username, "err", err)
				result.Errors++
				continue
			}
			result.UsersCreated++
		} else {
			// 6e. 更新 — 检查 manual_override
			var manualOverride bool
			_ = tx.QueryRow(ctx,
				`SELECT manual_override FROM assets.users WHERE id = $1`, existingID,
			).Scan(&manualOverride)

			if manualOverride {
				// 仅刷新 profile 字段, 不覆盖 role/status/scope
				if _, err := tx.Exec(ctx,
					`UPDATE assets.users SET
					   email = COALESCE(NULLIF($2,''), email),
					   display_name = COALESCE(NULLIF($3,''), display_name),
					   dn = COALESCE(NULLIF($4,''), dn),
					   org_id = $5,
					   deleted_at = NULL,
					   updated_at = now()
					 WHERE id = $1`,
					existingID, nullableStr(u.Email), nullableStr(u.DisplayName), nullableStr(u.DN), targetOrg,
				); err != nil {
					slog.Warn("ldap sync: user update (override) failed", "username", u.Username, "err", err)
					result.Errors++
					continue
				}
				result.UsersSkipped++
			} else {
				status := "active"
				if u.UserAccountControl&adUserAccountDisabled != 0 {
					status = "disabled"
				}
				if _, err := tx.Exec(ctx,
					`UPDATE assets.users SET
					   email = COALESCE(NULLIF($2,''), email),
					   display_name = COALESCE(NULLIF($3,''), display_name),
					   dn = COALESCE(NULLIF($4,''), dn),
					   org_id = $5,
					   role = $6,
					   data_scope = $7,
					   status = $8,
					   deleted_at = NULL,
					   updated_at = now()
					 WHERE id = $1`,
					existingID, nullableStr(u.Email), nullableStr(u.DisplayName), nullableStr(u.DN),
					targetOrg, resolved.Role, resolved.DataScope, status,
				); err != nil {
					slog.Warn("ldap sync: user update failed", "username", u.Username, "err", err)
					result.Errors++
					continue
				}
				result.UsersUpdated++
			}
		}
	}

	// 7. 处理 AD 中已不存在的 LDAP 用户 (不在任何启用组 → 禁用)
	rows, err := tx.Query(ctx,
		`SELECT id::text, external_id FROM assets.users
		 WHERE source = 'ldap' AND deleted_at IS NULL AND status = 'active'`)
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
		// 仅禁用不软删: 保留审计与领用历史
		_, _ = tx.Exec(ctx,
			`UPDATE assets.users SET status = 'disabled', updated_at = now() WHERE id = $1`, d.id)
		result.UsersDisabled++
	}

	// 8. 审计摘要
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
	slog.Info("ldap sync complete",
		"created", result.UsersCreated,
		"updated", result.UsersUpdated,
		"disabled", result.UsersDisabled,
		"skipped", result.UsersSkipped,
		"ad_disabled", result.ADUsersDisabled,
	)
	return result, nil
}

// truncateAuditMetadata 确保 audit metadata::text 不超过 4096 字节 (audit_log CHECK 约束)。
func truncateAuditMetadata(raw []byte) []byte {
	const maxSummaryLen = 3500
	if len(raw) <= maxSummaryLen {
		return raw
	}
	return []byte(fmt.Sprintf(`{"truncated":true,"orig_len":%d}`, len(raw)))
}

// writeAuditSeparately 在独立事务中写入审计条目, 与业务事务解耦。
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

// nullableStr 空串返回 NULL 友好参数
func nullableStr(s string) string { return s }

// ensure strings import is used (for the split in sanitizeLTree)
var _ = strings.TrimSpace
