// Asset Database System — API Server
// Phase 1: Foundation — JWT EdDSA + 中间件链 + 健康检查
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

	server := api.NewServer(cfg, km, nil)

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
