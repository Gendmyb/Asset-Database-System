// Package repository — 用户数据访问层 (PG)
package repository

import (
	"context"
	"time"
)

// UserRow 用户
type UserRow struct {
	ID             string     `json:"id"`
	Username       string     `json:"username"`
	Role           string     `json:"role"`
	Email          string     `json:"email,omitempty"`
	OrgID          string     `json:"org_id"`
	Status         string     `json:"status"`
	LastLoginAt    *time.Time `json:"last_login_at,omitempty"`
	MustChangePwd  bool       `json:"must_change_password"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// UserRepo 用户仓库 (无状态 — DBTX 由调用方传入)
type UserRepo struct{}

func NewUserRepo() *UserRepo {
	return &UserRepo{}
}

// ListByOrg 获取组织内所有活跃用户
func (r *UserRepo) ListByOrg(ctx context.Context, q DBTX, orgID string) ([]UserRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, username, role, COALESCE(email,''), org_id::text, status,
		        COALESCE(last_login_at, '0001-01-01'::timestamptz),
		        must_change_password, created_at, updated_at
		 FROM assets.users WHERE org_id = $1 AND status = 'active'
		 ORDER BY username`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserRow
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Email, &u.OrgID, &u.Status,
			&u.LastLoginAt, &u.MustChangePwd, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// GetUsername 获取用户名
func (r *UserRepo) GetUsername(ctx context.Context, q DBTX, userID string) (string, error) {
	var username string
	err := q.QueryRow(ctx,
		`SELECT username FROM assets.users WHERE id = $1`, userID,
	).Scan(&username)
	if err != nil {
		return "未知用户", nil
	}
	return username, nil
}

// EnsureSeedUsers 确保种子用户存在 (幂等)
func (r *UserRepo) EnsureSeedUsers(ctx context.Context, q DBTX) error {
	// 确保至少有 admin + 几个演示用户
	users := []struct {
		id, orgID, username, role, email string
	}{
		{"00000000-0000-4000-a000-000000000010", "00000000-0000-4000-a000-000000000001", "admin", "super_admin", "admin@demo.local"},
		{"00000000-0000-4000-a000-000000000011", "00000000-0000-4000-a000-000000000001", "张三", "manager", "zhangsan@demo.local"},
		{"00000000-0000-4000-a000-000000000012", "00000000-0000-4000-a000-000000000001", "李四", "manager", "lisi@demo.local"},
		{"00000000-0000-4000-a000-000000000013", "00000000-0000-4000-a000-000000000001", "王五", "viewer", "wangwu@demo.local"},
	}
	for _, u := range users {
		_, err := q.Exec(ctx,
			`INSERT INTO assets.users (id, org_id, username, password_hash, role, email, status, must_change_password, created_at, updated_at)
			 VALUES ($1,$2,$3,'$2a$10$placeholder',$4,$5,'active',false,$6,$6)
			 ON CONFLICT (id) DO NOTHING`,
			u.id, u.orgID, u.username, u.role, u.email, time.Now())
		if err != nil {
			return err
		}
	}
	return nil
}

// ListAll 获取所有用户 (不限制 org)
func (r *UserRepo) ListAll(ctx context.Context, q DBTX) ([]UserRow, error) {
	rows, err := q.Query(ctx,
		`SELECT id, username, role, COALESCE(email,''), org_id::text, status,
		        COALESCE(last_login_at, '0001-01-01'::timestamptz),
		        must_change_password, created_at, updated_at
		 FROM assets.users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserRow
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.Email, &u.OrgID, &u.Status,
			&u.LastLoginAt, &u.MustChangePwd, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

// CreateUser 创建新用户
func (r *UserRepo) CreateUser(ctx context.Context, q DBTX, username, passwordHash, role, email, orgID string) (string, error) {
	var id string
	err := q.QueryRow(ctx,
		`INSERT INTO assets.users (org_id, username, password_hash, role, email, status, must_change_password, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, 'active', true, now(), now()) RETURNING id::text`,
		orgID, username, passwordHash, role, email,
	).Scan(&id)
	return id, err
}

// UpdateUser 更新用户 role/status/email
func (r *UserRepo) UpdateUser(ctx context.Context, q DBTX, userID string, updates map[string]interface{}) error {
	_, err := q.Exec(ctx,
		`UPDATE assets.users SET
			role = COALESCE($2::varchar, role),
			status = COALESCE($3::varchar, status),
			email = COALESCE($4::varchar, email),
			updated_at = now()
		 WHERE id = $1`,
		userID, updates["role"], updates["status"], updates["email"],
	)
	return err
}

// SetPasswordHash 设置密码 hash
func (r *UserRepo) SetPasswordHash(ctx context.Context, q DBTX, userID, hash string) error {
	_, err := q.Exec(ctx,
		`UPDATE assets.users SET password_hash = $1, must_change_password = true, updated_at = now()
		 WHERE id = $2`, hash, userID)
	return err
}
