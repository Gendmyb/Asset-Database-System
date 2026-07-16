// Package agent — Agent 硬件指纹与身份密钥
// 对应架构文档 §6.2 Agent 注册与身份
package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateFingerprint 生成 Agent 硬件指纹
// SHA256(machineID + macAddr + hostname) → 固定32字节 → hex 64字符
// 用作 Agent 的唯一标识 (AgentKey)
func GenerateFingerprint(machineID, macAddr, hostname string) string {
	h := sha256.New()
	h.Write([]byte(machineID))
	h.Write([]byte(macAddr))
	h.Write([]byte(hostname))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateKeyPair 生成 Ed25519 密钥对
// Agent 注册时生成，私钥本地保存，公钥上报服务器
func GenerateKeyPair() (publicKeyHex string, privateKey ed25519.PrivateKey, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	return hex.EncodeToString(pub), priv, nil
}

// SignData 使用 Ed25519 私钥对数据签名
func SignData(privateKey ed25519.PrivateKey, data []byte) ([]byte, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}
	return ed25519.Sign(privateKey, data), nil
}

// VerifySignature 使用 Ed25519 公钥验证签名
// pubKeyHex 为 hex 编码的公钥
func VerifySignature(pubKeyHex string, data, signature []byte) (bool, error) {
	pubKey, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return false, fmt.Errorf("decode public key: %w", err)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key size: %d", len(pubKey))
	}
	return ed25519.Verify(ed25519.PublicKey(pubKey), data, signature), nil
}
