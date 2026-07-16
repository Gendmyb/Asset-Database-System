// Package api — Gin Server (Phase 1-5 完整路由)
package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/handler"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/middleware"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/lock"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	engine     *gin.Engine
	cfg        *config.Config
	keyManager *crypto.KeyManager
	httpServer *http.Server
}

func NewServer(cfg *config.Config, km *crypto.KeyManager, pool *pgxpool.Pool) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	// ===== 中间件链 =====
	engine.Use(
		middleware.RequestID(),
		middleware.Recovery(),
		middleware.StructuredLogging(),
		middleware.RateLimit(),
	)

	// ===== 健康检查 (无需认证) =====
	health := handler.NewHealthHandler()
	engine.GET("/healthz", health.Healthz)
	engine.GET("/readyz", health.Readyz(
		func() error { return pool.Ping(nil) },
		nil,
	))

	// ===== API v1 (需认证) =====
	v1 := engine.Group("/api/v1")
	v1.Use(middleware.Auth(km))
	v1.Use(middleware.OrgScope())

	// --- 资产 (Phase 2) ---
	assetRepo := repository.NewAssetRepo(pool)
	assetV2 := handler.NewAssetV2Handler(assetRepo)
	v1.GET("/assets", assetV2.ListAssets)
	v1.POST("/assets", assetV2.CreateAsset)
	v1.GET("/assets/:id", assetV2.GetAsset)
	v1.PUT("/assets/:id", assetV2.UpdateAsset)
	v1.DELETE("/assets/:id", assetV2.DeleteAsset)
	v1.POST("/assets/:id/transition", assetV2.LifecycleTransition)

	// --- 领用/归还/转移 (Phase 2) ---
	assignH := handler.NewAssignmentHandler(assetRepo)
	v1.POST("/assets/:id/assign", assignH.Assign)
	v1.POST("/assets/:id/release", assignH.Release)
	v1.POST("/assets/:id/transfer", assignH.Transfer)

	// --- 仪表盘 (Phase 5) ---
	dashH := handler.NewDashboardHandler(nil) // TODO: 注入 DashboardQuerier
	v1.GET("/dashboard/overview", dashH.Overview)
	v1.GET("/dashboard/agents", dashH.AgentHealth)

	// --- 位置 (Phase 5) ---
	locH := handler.NewLocationHandler()
	v1.GET("/locations", locH.List)
	v1.POST("/locations", locH.Create)

	// --- 组织 (Phase 5) ---
	orgH := handler.NewOrgHandler()
	v1.GET("/organizations", orgH.List)
	v1.POST("/organizations", orgH.Create)
	v1.GET("/organizations/:id", orgH.Get)
	v1.GET("/organizations/:id/subtree", orgH.Subtree)

	// --- 管理 (Phase 5, super_admin only) ---
	admin := v1.Group("/admin")
	admin.GET("/users", func(c *gin.Context) { c.JSON(200, gin.H{"data": []interface{}{}}) })

	// Advisor lock 碰撞检测
	_ = lock.DetectCollision

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Routes: %d endpoints registered", len(engine.Routes()))
	log.Printf("JWT public key: %s...", km.HexEncodePublicKey()[:32])

	return &Server{
		engine:     engine,
		cfg:        cfg,
		keyManager: km,
		httpServer: &http.Server{
			Addr:    addr,
			Handler: engine,
		},
	}
}

func (s *Server) Start() error {
	log.Printf("API Server listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop() error {
	log.Println("Shutting down...")
	return s.httpServer.Close()
}
