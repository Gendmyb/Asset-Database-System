// Collection Agent — 跨平台采集代理入口
// Phase 7: Agent Polish
// 对应架构文档 §9 Agent 采集架构

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/agent"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/agent/collector"
)

const Version = "0.1.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("=== Collection Agent v%s ===", Version)

	// 1. 加载配置
	cfg := agent.DefaultConfig()
	log.Printf("Server: %s, Interval: %s", cfg.ServerURL, cfg.CollectInterval)

	// 2. 生成硬件指纹
	fingerprint := generateFingerprint()
	log.Printf("Fingerprint: %s", fingerprint[:16]+"...")

	// 3. 生成 Ed25519 密钥对
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	log.Printf("Public key: %x...", pub[:8])

	// 4. 注册 (简化: 跳过 HTTP)
	// POST /auth/register-agent { fingerprint, public_key, enrollment_token }

	// 5. 采集循环
	col, _ := collector.NewCollector()
	ticker := time.NewTicker(cfg.CollectInterval)
	defer ticker.Stop()

	log.Println("Agent started, collecting...")
	for range ticker.C {
		data, err := col.Collect("all")
		if err != nil {
			log.Printf("Collect error: %v", err)
			continue
		}
		log.Printf("Collected %d modules", len(data))
		// 6. 上报 (简化: 跳过 HTTP+Ed25519签名)
		// POST /agents/sync { full_snapshot: false, delta_modules: data }
	}
}

func generateFingerprint() string {
	// 生产: SHA256(/etc/machine-id + MAC + hostname)
	return fmt.Sprintf("sha256-%d", time.Now().UnixNano())
}
