// Package domain — Agent 领域模型
// 对应架构文档 §6 Agent 采集架构
package domain

import "time"

// AgentStatus Agent 状态枚举
type AgentStatus string

const (
	AgentStatusRegistered AgentStatus = "registered" // 已注册 (未上线)
	AgentStatusOnline     AgentStatus = "online"     // 在线
	AgentStatusOffline    AgentStatus = "offline"    // 离线 (>心跳超时)
	AgentStatusDisabled   AgentStatus = "disabled"   // 已禁用
)

// IsActive 是否处于活跃状态
func (s AgentStatus) IsActive() bool {
	return s == AgentStatusOnline
}

// IsDisabled 是否已禁用
func (s AgentStatus) IsDisabled() bool {
	return s == AgentStatusDisabled
}

// CollectionAgent Agent 领域模型
// 对应架构文档 §6.2 Agent 注册与身份
type CollectionAgent struct {
	ID            string      `json:"id"`
	AgentKey      string      `json:"agent_key"` // 唯一标识 (硬件指纹 hash)
	OrgID         string      `json:"org_id"`    // 所属组织
	Hostname      string      `json:"hostname"`
	OSType        string      `json:"os_type"` // linux/darwin/windows
	OSVersion     string      `json:"os_version"`
	Status        AgentStatus `json:"status"`
	PublicKey     string      `json:"public_key"`  // Ed25519 公钥 (hex)
	CertSerial    string      `json:"cert_serial"` // 证书序列号
	LastHeartbeat *time.Time  `json:"last_heartbeat"`
	IPAddress     string      `json:"ip_address"`
	AgentVersion  string      `json:"agent_version"`
	Tags          []string    `json:"tags"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

// EnrollmentToken Agent 注册令牌
// 用于新 Agent 首次注册时的身份验证
type EnrollmentToken struct {
	Token     string    `json:"token"`
	OrgID     string    `json:"org_id"`
	MaxUses   int       `json:"max_uses"` // 最大使用次数 (0=无限)
	UsedCount int       `json:"used_count"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// IsExpired 令牌是否过期
func (t *EnrollmentToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsExhausted 令牌是否用完
func (t *EnrollmentToken) IsExhausted() bool {
	if t.MaxUses == 0 {
		return false // 无限使用
	}
	return t.UsedCount >= t.MaxUses
}

// IsValid 令牌是否有效
func (t *EnrollmentToken) IsValid() bool {
	return !t.IsExpired() && !t.IsExhausted()
}
