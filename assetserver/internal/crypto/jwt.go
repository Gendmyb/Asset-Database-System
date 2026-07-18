// Package crypto — JWT EdDSA 密钥管理与签发/验证
// 对应架构文档 §7.1 JWT 签名与密钥管理
package crypto

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// CustomClaims 自定义 JWT Claims (含 org_id 和 role)
type CustomClaims struct {
	jwt.RegisteredClaims
	OrgID string `json:"org_id"`
	Role  string `json:"role"`
}

// KeyManager — Vault/KMS 密钥管理
// 生产对接 Vault Transit Engine; 开发使用本地 Ed25519
type KeyManager struct {
	mu           sync.RWMutex
	privateKey   ed25519.PrivateKey
	publicKey    ed25519.PublicKey
	currentKeyID string
	degradedMode bool
}

// NewKeyManager 创建密钥管理器
// seedHex: hex 编码的 32 字节 Ed25519 seed。空字符串则随机生成 (每次重启密钥不同)
func NewKeyManager(seedHex string) (*KeyManager, error) {
	var privKey ed25519.PrivateKey

	if seedHex != "" {
		seed, err := hex.DecodeString(seedHex)
		if err != nil {
			return nil, fmt.Errorf("decode JWT_ED25519_SEED: %w", err)
		}
		if len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("JWT_ED25519_SEED must be %d bytes (got %d)", ed25519.SeedSize, len(seed))
		}
		// Derive Ed25519 private key from seed using SHA-256 (deterministic)
		h := sha256.Sum256(seed)
		privKey = ed25519.NewKeyFromSeed(h[:])
	} else {
		slog.Warn("JWT_ED25519_SEED not set — keys will be random on each restart, invalidating all existing tokens")
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ed25519 key: %w", err)
		}
		_ = pub
		privKey = priv
	}

	return &KeyManager{
		privateKey:   privKey,
		publicKey:    privKey.Public().(ed25519.PublicKey),
		currentKeyID: "kid-" + uuid.New().String()[:8],
	}, nil
}

// GetCurrentKeyID 返回当前 kid
func (km *KeyManager) GetCurrentKeyID() string {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.currentKeyID
}

// GetPrivateKey 获取私钥
func (km *KeyManager) GetPrivateKey() (ed25519.PrivateKey, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()
	if km.privateKey == nil {
		return nil, fmt.Errorf("no private key available")
	}
	return km.privateKey, nil
}

// GetPublicKey 获取公钥
func (km *KeyManager) GetPublicKey() ed25519.PublicKey {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.publicKey
}

// HexEncodePublicKey 公钥 hex (前端/JWK)
func (km *KeyManager) HexEncodePublicKey() string {
	return hex.EncodeToString(km.GetPublicKey())
}

// VerifyJWTLeeway 验证 JWT 但允许过期 (用于 refresh 流程提取 claims)
func (km *KeyManager) VerifyJWTLeeway(tokenString string) (*middleware.Claims, error) {
	pubKey := km.GetPublicKey()

	claims := &CustomClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return pubKey, nil
		},
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithIssuer("asset-db-api"),
		jwt.WithAudience("asset-db"),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("jwt verification: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	orgID := claims.OrgID
	if orgID == "" {
		orgID = "00000000-0000-4000-a000-000000000001"
	}
	role := claims.Role
	if role == "" {
		role = "viewer"
	}

	return &middleware.Claims{
		UserID: claims.Subject,
		OrgID:  orgID,
		Role:   role,
	}, nil
}

// ExtractClaimsNoVerify 不验证即提取 claims (仅用于解析, 不信任)
func (km *KeyManager) ExtractClaimsNoVerify(tokenString string) *middleware.Claims {
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &CustomClaims{})
	if err != nil {
		return nil
	}
	claims, ok := token.Claims.(*CustomClaims)
	if !ok {
		return nil
	}
	orgID := claims.OrgID
	if orgID == "" {
		orgID = "00000000-0000-4000-a000-000000000001"
	}
	role := claims.Role
	if role == "" {
		role = "viewer"
	}
	return &middleware.Claims{
		UserID: claims.Subject,
		OrgID:  orgID,
		Role:   role,
	}
}

// VerifyJWT 实现 middleware.ClaimsVerifier 接口
func (km *KeyManager) VerifyJWT(tokenString string) (*middleware.Claims, error) {
	pubKey := km.GetPublicKey()

	claims := &CustomClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims,
		func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodEd25519); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return pubKey, nil
		},
		jwt.WithValidMethods([]string{"EdDSA"}),
		jwt.WithIssuer("asset-db-api"),
		jwt.WithAudience("asset-db"),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("jwt verification: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	orgID := claims.OrgID
	if orgID == "" {
		orgID = "00000000-0000-4000-a000-000000000001" // fallback
	}
	role := claims.Role
	if role == "" {
		role = "viewer"
	}

	return &middleware.Claims{
		UserID: claims.Subject,
		OrgID:  orgID,
		Role:   role,
	}, nil
}

// IssueAccessToken 签发 access token (含 org_id + role)
func (km *KeyManager) IssueAccessToken(ctx context.Context, userID, role, orgID string) (string, error) {
	privKey, err := km.GetPrivateKey()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "asset-db-api",
			Subject:   userID,
			Audience:  jwt.ClaimStrings{"asset-db"},
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
		},
		OrgID: orgID,
		Role:  role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = km.currentKeyID
	return token.SignedString(privKey)
}
