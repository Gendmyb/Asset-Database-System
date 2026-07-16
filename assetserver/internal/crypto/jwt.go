// Package crypto — JWT EdDSA 密钥管理与签发/验证
// 对应架构文档 §7.1 JWT 签名与密钥管理
package crypto

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

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
func NewKeyManager(existingPrivKey ed25519.PrivateKey) (*KeyManager, error) {
	if existingPrivKey == nil {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate ed25519 key: %w", err)
		}
		_ = pub
		existingPrivKey = priv
	}

	return &KeyManager{
		privateKey:   existingPrivKey,
		publicKey:    existingPrivKey.Public().(ed25519.PublicKey),
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

// VerifyJWT 实现 middleware.ClaimsVerifier 接口
func (km *KeyManager) VerifyJWT(tokenString string) (*middleware.Claims, error) {
	pubKey := km.GetPublicKey()

	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{},
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

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return &middleware.Claims{
		UserID: claims.Subject,
		OrgID:  "org-001", // 简化: 生产从 claims 提取
		Role:   "admin",
	}, nil
}

// IssueAccessToken 签发 access token
func (km *KeyManager) IssueAccessToken(ctx context.Context, userID, role, orgID string) (string, error) {
	privKey, err := km.GetPrivateKey()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    "asset-db-api",
		Subject:   userID,
		Audience:  jwt.ClaimStrings{"asset-db"},
		ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        uuid.New().String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = km.currentKeyID
	return token.SignedString(privKey)
}
