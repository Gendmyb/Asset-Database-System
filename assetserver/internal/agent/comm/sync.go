// Package comm — Agent 同步协议
// 对应架构文档 §6.6 Agent ↔ Server 同步协议
package comm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// ModuleEntry 单个模块的增量信息
// 用于增量同步: 只传输变更的模块
type ModuleEntry struct {
	Name     string `json:"name"`     // 模块名称
	Checksum string `json:"checksum"` // 该模块数据的 SHA256
	Data     []byte `json:"data"`     // 模块数据 (JSON/MessagePack 序列化)
	Action   string `json:"action"`   // "add" | "update" | "delete"
}

// SyncPayload 全量同步载荷
// Agent 上报完整快照时使用
type SyncPayload struct {
	AgentKey       string        `json:"agent_key"`
	Sequence       int64         `json:"sequence"`        // 单调递增序列号
	FullSnapshot   bool          `json:"full_snapshot"`   // true = 全量快照
	DeltaModules   []ModuleEntry `json:"delta_modules"`   // 增量/全量模块列表
	PayloadChecksum string       `json:"payload_checksum"` // 整个载荷的 SHA256
	Timestamp      time.Time     `json:"timestamp"`
}

// DeltaPayload 增量同步载荷
// 仅传输变更的模块 (delta mode)
type DeltaPayload struct {
	AgentKey    string        `json:"agent_key"`
	BaseSeq     int64         `json:"base_seq"`     // 基准序列号 (上次同步到的 seq)
	Modules     []ModuleEntry `json:"modules"`      // 变更的模块
	Timestamp   time.Time     `json:"timestamp"`
}

// ComputeChecksum 计算载荷校验和
// 将 payload 序列化为 JSON 后取 SHA256
func ComputeChecksum(payload interface{}) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// NewSyncPayload 创建全量同步载荷
// 所有模块作为 delta_modules 上报，full_snapshot=true
func NewSyncPayload(agentKey string, seq int64, modules []ModuleEntry) (*SyncPayload, error) {
	p := &SyncPayload{
		AgentKey:     agentKey,
		Sequence:     seq,
		FullSnapshot: true,
		DeltaModules: modules,
		Timestamp:    time.Now().UTC(),
	}
	cksum, err := ComputeChecksum(p)
	if err != nil {
		return nil, err
	}
	p.PayloadChecksum = cksum
	return p, nil
}

// NewDeltaPayload 创建增量同步载荷
func NewDeltaPayload(agentKey string, baseSeq int64, modules []ModuleEntry) *DeltaPayload {
	return &DeltaPayload{
		AgentKey:  agentKey,
		BaseSeq:   baseSeq,
		Modules:   modules,
		Timestamp: time.Now().UTC(),
	}
}
