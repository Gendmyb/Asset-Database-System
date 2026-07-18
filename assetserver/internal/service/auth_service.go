// Package service — 认证服务 (登录/刷新/登出)
// Phase C: 真实认证与 RBAC
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// AuthService 认证服务
type AuthService struct {
	pool     *pgxpool.Pool
	km       *crypto.KeyManager
	failures *loginFailures
}

// loginFailures 内存限速 (per-username 失败计数)
type loginFailures struct {
	mu       sync.Mutex
	attempts map[string]*failEntry
}

type failEntry struct {
	count     int
	lockedAt  time.Time
}

// NewAuthService 创建认证服务
func NewAuthService(pool *pgxpool.Pool, km *crypto.KeyManager) *AuthService {
	return &AuthService{
		pool: pool,
		km:   km,
		failures: &loginFailures{
			attempts: make(map[string]*failEntry),
		},
	}
}

// maxFailures 触发锁定的连续失败次数
const maxFailures = 5

// lockDuration 锁定时间
const lockDuration = 15 * time.Minute

// checkRateLimit 检查并更新限速
func (lf *loginFailures) checkRateLimit(username string) error {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	entry, exists := lf.attempts[username]
	if !exists {
		lf.attempts[username] = &failEntry{count: 1}
		return nil
	}

	// 锁定过期则重置
	if !entry.lockedAt.IsZero() && time.Since(entry.lockedAt) > lockDuration {
		entry.count = 1
		entry.lockedAt = time.Time{}
		return nil
	}

	if !entry.lockedAt.IsZero() {
		return fmt.Errorf("帐户已临时锁定，请 %v 后重试",
			lockDuration-time.Since(entry.lockedAt).Round(time.Minute))
	}

	entry.count++
	if entry.count >= maxFailures {
		entry.lockedAt = time.Now()
		return fmt.Errorf("连续 %d 次失败，帐户已锁定 %v",
			maxFailures, lockDuration)
	}
	return nil
}

// resetFailures 登录成功则重置失败计数
func (lf *loginFailures) resetFailures(username string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	delete(lf.attempts, username)
}

// LoginResult 登录结果
type LoginResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         UserInfo `json:"user"`
}

// UserInfo 用户信息
type UserInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	OrgID    string `json:"org_id"`
	Email    string `json:"email,omitempty"`
}

// Login 用户登录
func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResult, error) {
	// 1. 限速检查
	if err := s.failures.checkRateLimit(username); err != nil {
		return nil, err
	}

	// 2. 从 DB 查询用户
	var (
		userID       string
		role         string
		orgID        string
		email        string
		passwordHash string
		status       string
	)
	err := s.pool.QueryRow(ctx,
		`SELECT id, role, org_id, COALESCE(email,''), password_hash, status
		 FROM assets.users WHERE username = $1`, username,
	).Scan(&userID, &role, &orgID, &email, &passwordHash, &status)
	if err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	if status != "active" {
		return nil, fmt.Errorf("帐户已被禁用")
	}

	// 3. bcrypt 比对
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("用户名或密码错误")
	}

	// 成功 → 重置失败计数
	s.failures.resetFailures(username)

	// 4. 签发 access token (15min)
	accessToken, err := s.km.IssueAccessToken(ctx, userID, role, orgID)
	if err != nil {
		return nil, fmt.Errorf("签发 token 失败: %w", err)
	}

	// 5. 生成 refresh token
	refreshToken, _, err := s.storeRefreshToken(ctx, userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("生成 refresh token 失败: %w", err)
	}

	// 6. 更新 last_login_at
	_, _ = s.pool.Exec(ctx,
		`UPDATE assets.users SET last_login_at = now() WHERE id = $1`, userID)

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: UserInfo{
			ID:       userID,
			Username: username,
			Role:     role,
			OrgID:    orgID,
			Email:    email,
		},
	}, nil
}

// RefreshResult refresh 结果
type RefreshResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// Refresh 刷新令牌
func (s *AuthService) Refresh(ctx context.Context, accessToken, refreshToken string) (*RefreshResult, error) {
	// 1. 解析 access token (不校验过期，只取 claims)
	claims, err := s.km.VerifyJWTLeeway(accessToken)
	if err != nil {
		// 即使过期也尝试提取 claims
		claims = s.km.ExtractClaimsNoVerify(accessToken)
	}
	if claims == nil {
		return nil, fmt.Errorf("无效的 access token")
	}

	// 2. SHA256(refreshToken) 查表
	hash := sha256Hex(refreshToken)

	var (
		dbID      string
		familyID  string
		userID    string
		expiresAt time.Time
		revokedAt *time.Time
	)
	err = s.pool.QueryRow(ctx,
		`SELECT id, family_id, user_id, expires_at, revoked_at
		 FROM assets.refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&dbID, &familyID, &userID, &expiresAt, &revokedAt)
	if err != nil {
		return nil, fmt.Errorf("refresh token 无效")
	}

	// 3. 如果已 revoked → 全 family 吊销
	if revokedAt != nil {
		s.revokeFamily(ctx, familyID)
		return nil, fmt.Errorf("refresh token 已被使用 (疑似重放攻击)，全系列已吊销")
	}

	// 4. 检查过期
	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("refresh token 已过期，请重新登录")
	}

	// 5. 旧行 revoked → INSERT 新行
	_, _ = s.pool.Exec(ctx,
		`UPDATE assets.refresh_tokens SET revoked_at = now() WHERE id = $1`, dbID)

	newRefreshToken, _, err := s.insertRefreshToken(ctx, userID, familyID)
	if err != nil {
		return nil, fmt.Errorf("存储新 refresh token 失败: %w", err)
	}

	// 6. 签发新 access token
	// 需要查用户的 role 和 org_id
	var role, orgID string
	_ = s.pool.QueryRow(ctx,
		`SELECT role, org_id::text FROM assets.users WHERE id = $1`, userID,
	).Scan(&role, &orgID)

	newAccessToken, err := s.km.IssueAccessToken(ctx, userID, role, orgID)
	if err != nil {
		return nil, fmt.Errorf("签发新 token 失败: %w", err)
	}

	return &RefreshResult{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

// Logout 登出 (吊销整 family)
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	hash := sha256Hex(refreshToken)

	var familyID string
	err := s.pool.QueryRow(ctx,
		`SELECT family_id FROM assets.refresh_tokens WHERE token_hash = $1`, hash,
	).Scan(&familyID)
	if err != nil {
		// token 不存在，也算登出成功
		return nil
	}

	s.revokeFamily(ctx, familyID)
	return nil
}

// GetUserByID 根据用户 ID 获取用户信息
func (s *AuthService) GetUserByID(ctx context.Context, userID string) (*UserInfo, error) {
	var u UserInfo
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, role, org_id::text, COALESCE(email,'')
		 FROM assets.users WHERE id = $1 AND status = 'active'`, userID,
	).Scan(&u.ID, &u.Username, &u.Role, &u.OrgID, &u.Email)
	if err != nil {
		return nil, fmt.Errorf("用户未找到")
	}
	return &u, nil
}

// ChangePassword 修改密码
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error {
	// 1. 获取当前密码 hash
	var curHash string
	err := s.pool.QueryRow(ctx,
		`SELECT password_hash FROM assets.users WHERE id = $1`, userID,
	).Scan(&curHash)
	if err != nil {
		return fmt.Errorf("用户未找到")
	}

	// 2. 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(curHash), []byte(oldPassword)); err != nil {
		return fmt.Errorf("旧密码错误")
	}

	// 3. 生成新 hash
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("密码加密失败: %w", err)
	}

	// 4. 更新
	_, err = s.pool.Exec(ctx,
		`UPDATE assets.users SET password_hash = $1, must_change_password = false WHERE id = $2`,
		string(newHash), userID)
	return err
}

// --- internal helpers ---

func (s *AuthService) storeRefreshToken(ctx context.Context, userID, orgID string) (token string, familyID string, err error) {
	token = generateRandomHex(32)
	familyID = uuid.New().String()
	_, _, err = s.insertRefreshToken(ctx, userID, familyID)
	return
}

func (s *AuthService) insertRefreshToken(ctx context.Context, userID, familyID string) (token string, id string, err error) {
	token = generateRandomHex(32)
	hash := sha256Hex(token)
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7d
	var newID string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO assets.refresh_tokens (user_id, token_hash, family_id, expires_at)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, hash, familyID, expiresAt,
	).Scan(&newID)
	return token, newID, err
}

func (s *AuthService) revokeFamily(ctx context.Context, familyID string) {
	_, _ = s.pool.Exec(ctx,
		`UPDATE assets.refresh_tokens SET revoked_at = now()
		 WHERE family_id = $1 AND revoked_at IS NULL`, familyID)
}

// sha256Hex 返回小写 hex 编码的 SHA-256
func sha256Hex(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// generateRandomHex 生成 n 字节的随机 hex 字符串 (长度 2*n)
func generateRandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
