// Asset Database System — API Server
// 支持 demo 模式 (DEMO=true 跳过 PostgreSQL, 使用内存存储)
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("=== Asset Database System ===")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	km, err := crypto.NewKeyManager(nil)
	if err != nil {
		log.Fatalf("Key manager error: %v", err)
	}
	log.Printf("Ed25519 Key: kid=%s", km.GetCurrentKeyID())

	demoMode := os.Getenv("DEMO") == "true"
	if demoMode {
		log.Println("⚠️  DEMO mode: in-memory stores, no PostgreSQL required")
	}

	server := api.NewServer(cfg, km, nil, demoMode)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		server.Stop()
		os.Exit(0)
	}()

	if err := server.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
