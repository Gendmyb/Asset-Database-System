// Package agent — Agent 配置 (Phase 7)
package agent

import (
	"encoding/json"
	"os"
	"time"
)

// Config Agent 配置
type Config struct {
	ServerURL       string        `json:"server_url"`
	MTLSCertFile    string        `json:"mtls_cert_file"`
	MTLSKeyFile     string        `json:"mtls_key_file"`
	CACertFile      string        `json:"ca_cert_file"`
	CollectInterval time.Duration `json:"collect_interval"`
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`
	SamplingTiers   SamplingTiers  `json:"sampling_tiers"`
	OfflineMaxSize  int            `json:"offline_max_size"`
	OfflineMaxRetries int          `json:"offline_max_retries"`
}

type SamplingTiers struct {
	Critical     time.Duration `json:"critical"`     // 5min
	Standard     time.Duration `json:"standard"`     // 15min
	LowPriority  time.Duration `json:"low_priority"` // 30min
}

func DefaultConfig() *Config {
	return &Config{
		ServerURL:         "https://asset-db.internal",
		MTLSCertFile:      "/etc/agent/client.crt",
		MTLSKeyFile:       "/etc/agent/client.key",
		CACertFile:        "/etc/agent/ca.crt",
		CollectInterval:   5 * time.Minute,
		HeartbeatInterval: 60 * time.Second,
		SamplingTiers: SamplingTiers{
			Critical:    5 * time.Minute,
			Standard:    15 * time.Minute,
			LowPriority: 30 * time.Minute,
		},
		OfflineMaxSize:    10000,
		OfflineMaxRetries: 100,
	}
}

// LoadConfig 从 JSON 文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// 环境变量覆盖
	if v := os.Getenv("AGENT_SERVER_URL"); v != "" {
		cfg.ServerURL = v
	}
	return cfg, nil
}
