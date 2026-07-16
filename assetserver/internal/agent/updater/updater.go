// Package updater — Agent 自更新 (Phase 7)
// 对应架构文档 §9.7 自更新机制
package updater

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"time"
)

// UpdateInfo 更新信息
type UpdateInfo struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"` // Ed25519 hex
	MinAgentVersion string `json:"min_agent_version"`
}

// CheckUpdate 检查 Agent 更新
// 生产: POST /agents/:id/update-check → 返回 UpdateInfo
func CheckUpdate(serverURL, agentID, currentVersion string, publicKey ed25519.PublicKey) (*UpdateInfo, error) {
	// 简化: 返回固定版本 (生产需实现 HTTP 调用)
	_ = serverURL
	_ = agentID
	_ = currentVersion
	return &UpdateInfo{
		Version: "0.2.0",
		URL:     "https://releases.internal/agent-0.2.0",
		SHA256:  "sha256-placeholder",
	}, nil
}

// VerifySignature Ed25519 签名验证
func VerifySignature(payload []byte, sigHex string, publicKey ed25519.PublicKey) error {
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if !ed25519.Verify(publicKey, payload, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// AutoRollback 启动失败 30 秒自动回滚
func AutoRollback(newBinary, oldBinary string, timeout time.Duration) error {
	// 1. 备份旧二进制
	if err := os.Rename(newBinary, newBinary+".new"); err != nil {
		return err
	}

	// 2. 启动新进程 (简化: 跳过实际 exec)
	// 3. 如果 30 秒内未收到成功信号 → 回滚
	deadline := time.After(timeout)
	successCh := make(chan bool)

	select {
	case <-successCh:
		// 新版本启动成功, 清理旧备份
		os.Remove(oldBinary + ".old")
		return nil
	case <-deadline:
		// 超时, 回滚
		os.Rename(oldBinary+".old", oldBinary)
		return fmt.Errorf("rollback: new binary failed to start within %s", timeout)
	}
}

// CanaryGroup 金丝雀发布分组 (按 org_id 哈希确定性分配)
func CanaryGroup(orgID string, canaryPercent int) bool {
	if canaryPercent <= 0 {
		return false
	}
	if canaryPercent >= 100 {
		return true
	}
	h := crc32.ChecksumIEEE([]byte(orgID))
	return int(h%100) < canaryPercent
}
