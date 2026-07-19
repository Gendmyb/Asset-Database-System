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
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/auth/ldap"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/db"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/event"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/notify"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/scheduler"
	internalservice "github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
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

	// Event bus — log consumer
	eventTypes := []string{
		event.EventAssetCreated, event.EventAssetUpdated, event.EventAssetDeleted,
		event.EventAssetAssigned, event.EventAssetReleased, event.EventAssetTransferred,
		event.EventAssetBorrowed,
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

		// Phase I: Start webhook dispatcher
		whRepo := repository.NewWebhookRepo()
		whDispatcher := internalservice.NewWebhookDispatcher(pool, whRepo)
		go whDispatcher.Start(context.Background())

		// Wave 2 G6: Start notify dispatcher (邮件 + 机器人 webhook)
		notifyRepo := repository.NewNotifyRepo()
		notifyNotifiers := []notify.Notifier{
			notify.NewEmailNotifier(cfg.Notify.SMTP),
			notify.NewDingTalkNotifier(cfg.Notify.DingTalkWebhook),
			notify.NewWeComNotifier(cfg.Notify.WeComWebhook),
			notify.NewFeishuNotifier(cfg.Notify.FeishuWebhook),
		}
		notifyDispatcher := internalservice.NewNotifyDispatcher(pool, notifyRepo, notifyNotifiers, cfg.Notify.Enable)
		go notifyDispatcher.Start(context.Background())

		// Wave 1 G4: 启动调度器 (到期提醒 + LDAP 同步)
		startScheduler(cfg, pool)
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

// startScheduler 启动 G4 调度器 (到期提醒 + LDAP 同步)
// 间隔由 SCHEDULER_INTERVAL 控制; 未配置则不启动。
func startScheduler(cfg *config.Config, pool *pgxpool.Pool) {
	if cfg.Scheduler.Interval <= 0 {
		log.Println("Scheduler: disabled (SCHEDULER_INTERVAL not set)")
		return
	}

	assetRepo := repository.NewAssetRepo()
	assignmentRepo := repository.NewAssignmentRepo()
	scanner := scheduler.NewRepoScanner(assetRepo, assignmentRepo, pool)
	auditW := scheduler.NewPoolAuditWriter(pool)

	// LDAP 同步器 (仅启用时注入)
	var ldapSyncer scheduler.LDAPSyncer
	if cfg.Scheduler.EnableLDAP && cfg.LDAP.Enable {
		ldapSyncer = &ldapAdapter{
			svc: ldap.NewSyncService(ldap.NewClient(cfg.LDAP), pool),
		}
		log.Println("Scheduler: LDAP sync enabled")
	} else if cfg.Scheduler.EnableLDAP {
		log.Println("Scheduler: LDAP sync requested but LDAP not configured, skipping")
	}

	sched := scheduler.New(
		scheduler.Config{
			Interval:       cfg.Scheduler.Interval,
			WarrantyDays:   cfg.Scheduler.WarrantyDays,
			EnableLDAPSync: cfg.Scheduler.EnableLDAP,
			DefaultOrgID:   "00000000-0000-4000-a000-000000000001",
		},
		scanner,
		event.DefaultBus,
		auditW,
		ldapSyncer,
	)
	go sched.Run(context.Background())
	log.Printf("Scheduler: started (interval=%s, warranty_days=%d)",
		cfg.Scheduler.Interval, cfg.Scheduler.WarrantyDays)
}

// ldapAdapter 适配 ldap.SyncService.RunSyncOnce (*SyncResult 返回值) 到 scheduler.LDAPSyncer
type ldapAdapter struct {
	svc *ldap.SyncService
}

func (a *ldapAdapter) RunSyncOnce(ctx context.Context, actorID, orgID string) error {
	_, err := a.svc.RunSyncOnce(ctx, actorID, orgID)
	return err
}
