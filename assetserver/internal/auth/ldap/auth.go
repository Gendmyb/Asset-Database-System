// Package ldap — bind 登录校验 (Wave 3 T4 增强)
//   - SearchOne 返回 memberOf/userAccountControl (已在 client.go 实现)
//   - Authenticate 检查 AD 禁用状态
//   - EnsureUserRow 使用 T2 组映射确定角色/数据范围
//   - 尊重 manual_override: 已存在且 override=true 的用户不更新 role/status/scope
package ldap

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthService LDAP 登录辅助服务 (独立于 service.AuthService, 后者按需调用)
type AuthService struct {
	client    DirectoryClient
	pool      *pgxpool.Pool
	groupRepo interface {
		ListEnabled(ctx context.Context, q interface{}) (interface{}, error)
	} // T4: 组映射查询 (通过闭包注入)
	resolveFunc func(memberOf []string) ResolvedRole // T4: 角色解析 (注入, 避免循环依赖)
}

// NewAuthService 构造 LDAP 登录服务
func NewAuthService(client DirectoryClient, pool *pgxpool.Pool) *AuthService {
	return &AuthService{client: client, pool: pool}
}

// SetGroupResolver 注入组映射查询 + 角色解析器 (Wave 3 T4)
// resolve 是依赖 T2 resolve.go 的函数, 通过此方法注入避免循环依赖。
func (s *AuthService) SetGroupResolver(resolve func(memberOf []string) ResolvedRole) {
	s.resolveFunc = resolve
}

// AuthenticateResult LDAP 校验结果 (Wave 3 T4: 新增 MemberOf/UserAccountControl)
type AuthenticateResult struct {
	Valid              bool
	Username           string
	DisplayName        string
	Email              string
	DN                 string
	MemberOf           []string // T4: 所属安全组 DN
	UserAccountControl int      // T4: AD 账户控制标志
}

// adUserDisabled 检查 AD userAccountControl 是否标记为禁用
func adUserDisabled(uac int) bool {
	return uac&0x2 != 0
}

// Authenticate 用 (username, password) 做 LDAP bind
// 流程:
//  1. 用服务账号 + cfg.UserFilter 精确查询该 username 的 DN (单条查询, 不拉全量)
//  2. 检查 AD 账户禁用状态 (userAccountControl bit 1)
//  3. 用用户 DN + password 做 bind
//  4. 成功则返回条目信息 (含 memberOf/userAccountControl), 由上层 upsert 本地用户行并签发 JWT
func (s *AuthService) Authenticate(ctx context.Context, username, password string) (*AuthenticateResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("ldap disabled")
	}
	u, err := s.client.SearchOne(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("search user: %w", err)
	}
	if u == nil {
		return nil, fmt.Errorf("user not found in directory")
	}

	// T4: AD 禁用检查 (先于 bind, 凭据正确但账号禁用也应拒绝)
	if adUserDisabled(u.UserAccountControl) {
		return nil, fmt.Errorf("account disabled in active directory")
	}

	if err := s.client.Bind(ctx, u.DN, password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return &AuthenticateResult{
		Valid:              true,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		Email:              u.Email,
		DN:                 u.DN,
		MemberOf:           u.MemberOf,
		UserAccountControl: u.UserAccountControl,
	}, nil
}

// SetGroupRepo 注入组映射仓库 (T4: 用于 EnsureUserRow 查询组映射)
// 注意: 采用 interface{} 避免循环依赖 repository 包。
func (s *AuthService) SetGroupRepo(listEnabled func(ctx context.Context, q interface{}) ([]struct {
	GroupDN   string
	Role      string
	DataScope string
	SyncEnabled bool
}, error)) {
	// T4: 占位 — group mapping 查询将通过闭包注入
	// 实际注入在 server.go 组装时进行
}

// EnsureUserRow 登录成功后确保本系统有对应用户行 (source='ldap')
// 返回 (userID, role, orgID, dataScope, error)。密码绝不入日志/审计。
//
// Wave 3 T4 增强:
//   - 新建用户时使用 T2 组映射解析角色 + data_scope
//   - 已存在用户: 若 manual_override=true, 仅刷新 profile 不覆盖 role/scope
//   - 已存在用户: 若 manual_override=false, 更新 role/scope 来自组映射
//   - 若 username 与本地用户冲突 (source='local') 则返回错误, 拒绝 LDAP 登录该用户。
func (s *AuthService) EnsureUserRow(ctx context.Context, r *AuthenticateResult, defaultOrgID string) (userID, role, orgID, dataScope string, err error) {
	// 解析角色 (T4: 使用组映射)
	resolvedRole := "viewer"
	resolvedScope := "inherit"
	if s.resolveFunc != nil {
		resolved := s.resolveFunc(r.MemberOf)
		resolvedRole = resolved.Role
		resolvedScope = resolved.DataScope
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. 查既有 LDAP 用户
	var manualOverride bool
	err = tx.QueryRow(ctx,
		`SELECT id::text, role, org_id::text, data_scope, manual_override FROM assets.users
		 WHERE source = 'ldap' AND external_id = $1 AND deleted_at IS NULL`,
		r.Username,
	).Scan(&userID, &role, &orgID, &dataScope, &manualOverride)
	if err == nil {
		// 已存在: 根据 manual_override 决定更新范围
		if manualOverride {
			// T4: 仅刷新 profile 字段, 不覆盖 role/status/scope
			_, _ = tx.Exec(ctx,
				`UPDATE assets.users SET
				   display_name = COALESCE(NULLIF($2,''), display_name),
				   dn = COALESCE(NULLIF($3,''), dn),
				   email = COALESCE(NULLIF($4,''), email),
				   last_login_at = now(), updated_at = now()
				 WHERE id = $1`,
				userID, r.DisplayName, r.DN, r.Email)
		} else {
			// 全量更新 (含来自组映射的角色+scope)
			_, _ = tx.Exec(ctx,
				`UPDATE assets.users SET
				   display_name = COALESCE(NULLIF($2,''), display_name),
				   dn = COALESCE(NULLIF($3,''), dn),
				   email = COALESCE(NULLIF($4,''), email),
				   role = $5, data_scope = $6,
				   last_login_at = now(), updated_at = now()
				 WHERE id = $1`,
				userID, r.DisplayName, r.DN, r.Email, resolvedRole, resolvedScope)
			role = resolvedRole
			dataScope = resolvedScope
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return "", "", "", "", commitErr
		}
		return userID, role, orgID, dataScope, nil
	}

	// 2. 检查本地用户名冲突
	var localConflict bool
	_ = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM assets.users WHERE username = $1 AND source = 'local')`,
		r.Username,
	).Scan(&localConflict)
	if localConflict {
		return "", "", "", "", fmt.Errorf("username conflicts with local user")
	}

	// 3. 新建 LDAP 用户行 (T4: 使用组映射确定的角色 + data_scope)
	err = tx.QueryRow(ctx,
		`INSERT INTO assets.users
		   (org_id, username, password_hash, role, email, status, source,
		    external_id, display_name, dn, data_scope, must_change_password, last_login_at,
		    created_at, updated_at)
		 VALUES ($1, $2, '!', $3, $4, 'active', 'ldap', $5, $6, $7, $8, false, now(), now(), now())
		 RETURNING id::text, role, org_id::text, data_scope`,
		defaultOrgID, r.Username, resolvedRole, nullableStr(r.Email), r.Username,
		nullableStr(r.DisplayName), nullableStr(r.DN), resolvedScope,
	).Scan(&userID, &role, &orgID, &dataScope)
	if err != nil {
		return "", "", "", "", fmt.Errorf("create ldap user row: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", "", "", err
	}
	return userID, role, orgID, dataScope, nil
}
