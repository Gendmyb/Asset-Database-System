// Package ldap — bind 登录校验
package ldap

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AuthService LDAP 登录辅助服务 (独立于 service.AuthService, 后者按需调用)
type AuthService struct {
	client DirectoryClient
	pool   *pgxpool.Pool
}

// NewAuthService 构造 LDAP 登录服务
func NewAuthService(client DirectoryClient, pool *pgxpool.Pool) *AuthService {
	return &AuthService{client: client, pool: pool}
}

// AuthenticateResult LDAP 校验结果
type AuthenticateResult struct {
	Valid       bool
	Username    string
	DisplayName string
	Email       string
	DN          string
}

// Authenticate 用 (username, password) 做 LDAP bind
// 流程:
//  1. 用服务账号 + cfg.UserFilter 精确查询该 username 的 DN (单条查询, 不拉全量)
//  2. 用用户 DN + password 做 bind
//  3. 成功则返回条目信息, 由上层 upsert 本地用户行并签发 JWT
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
	if err := s.client.Bind(ctx, u.DN, password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return &AuthenticateResult{
		Valid:       true,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		DN:          u.DN,
	}, nil
}

// EnsureUserRow 登录成功后确保本系统有对应用户行 (source='ldap')
// 返回 (userID, role, orgID, error)。密码绝不入日志/审计。
// 若 username 与本地用户冲突 (source='local') 则返回错误, 拒绝 LDAP 登录该用户。
func (s *AuthService) EnsureUserRow(ctx context.Context, r *AuthenticateResult, defaultOrgID string) (userID, role, orgID string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", "", "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. 查既有 LDAP 用户
	err = tx.QueryRow(ctx,
		`SELECT id::text, role, org_id::text FROM assets.users
		 WHERE source = 'ldap' AND external_id = $1 AND deleted_at IS NULL`,
		r.Username,
	).Scan(&userID, &role, &orgID)
	if err == nil {
		// 更新 display_name/dn/email (不更新 role/org)
		_, _ = tx.Exec(ctx,
			`UPDATE assets.users SET
			   display_name = COALESCE(NULLIF($2,''), display_name),
			   dn = COALESCE(NULLIF($3,''), dn),
			   email = COALESCE(NULLIF($4,''), email),
			   last_login_at = now(), updated_at = now()
			 WHERE id = $1`,
			userID, r.DisplayName, r.DN, r.Email)
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return "", "", "", commitErr
		}
		return userID, role, orgID, nil
	}

	// 2. 检查本地用户名冲突
	var localConflict bool
	_ = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM assets.users WHERE username = $1 AND source = 'local')`,
		r.Username,
	).Scan(&localConflict)
	if localConflict {
		return "", "", "", fmt.Errorf("username conflicts with local user")
	}

	// 3. 新建 LDAP 用户行 (默认 viewer 角色, 默认组织)
	err = tx.QueryRow(ctx,
		`INSERT INTO assets.users
		   (org_id, username, password_hash, role, email, status, source,
		    external_id, display_name, dn, must_change_password, last_login_at,
		    created_at, updated_at)
		 VALUES ($1, $2, '!', 'viewer', $3, 'active', 'ldap', $4, $5, $6, false, now(), now(), now())
		 RETURNING id::text, role, org_id::text`,
		defaultOrgID, r.Username, nullableStr(r.Email), r.Username,
		nullableStr(r.DisplayName), nullableStr(r.DN),
	).Scan(&userID, &role, &orgID)
	if err != nil {
		return "", "", "", fmt.Errorf("create ldap user row: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", "", err
	}
	return userID, role, orgID, nil
}
