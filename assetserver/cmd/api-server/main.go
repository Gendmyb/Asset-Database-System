// Asset Database System — API Server
// 支持 demo 模式 (DEMO=true 跳过 PostgreSQL, 使用内存存储)
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/db"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("=== Asset Database System ===")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	km, err := crypto.NewKeyManager(cfg.Auth.Ed25519Seed)
	if err != nil {
		log.Fatalf("Key manager error: %v", err)
	}
	log.Printf("Ed25519 Key: kid=%s", km.GetCurrentKeyID())

	// 事件总线 — 挂日志消费者 (后续 Phase I 挂 webhook consumer)
	eventTypes := []string{
		event.EventAssetCreated, event.EventAssetUpdated, event.EventAssetDeleted,
		event.EventAssetAssigned, event.EventAssetReleased, event.EventAssetTransferred,
		event.EventLifecycleChanged,
	}
	for _, et := range eventTypes {
		ch, _ := event.DefaultBus.Subscribe(context.Background(), et)
		go func(evtCh <-chan *event.Event) {
			for evt := range evtCh {
				log.Printf("[EVENT] %s asset=%s org=%s user=%s", evt.Type, evt.AssetID, evt.OrgID, evt.UserID)
			}
		}(ch)
	}

	demoMode := os.Getenv("DEMO") == "true"

	var pool *pgxpool.Pool
	if !demoMode {
		pool, err = repository.NewPool(context.Background(), &cfg.Database)
		if err != nil {
			log.Fatalf("Database connection error: %v", err)
		}
		defer pool.Close()
		log.Println("PostgreSQL connected successfully")

		if err := db.RunMigrations(context.Background(), pool); err != nil {
			log.Fatalf("Migration error: %v", err)
		}
	} else {
		log.Println("⚠️  DEMO mode: in-memory stores, no PostgreSQL required")
	}

	server := api.NewServer(cfg, km, pool, demoMode)

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
