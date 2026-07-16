// Package api — Gin Server (支持 demo 模式: 无 PG 内存存储)
package api

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/handler"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/middleware"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	engine     *gin.Engine
	cfg        *config.Config
	keyManager *crypto.KeyManager
	httpServer *http.Server
	demoStore  *DemoAssetStore
}

type DemoAssetStore struct {
	mu     sync.RWMutex
	assets []map[string]interface{}
}

func (s *DemoAssetStore) List() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.assets
}

func (s *DemoAssetStore) Add(asset map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets = append(s.assets, asset)
}

func NewServer(cfg *config.Config, km *crypto.KeyManager, pool *pgxpool.Pool, demoMode bool) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	engine.Use(
		middleware.RequestID(),
		middleware.Recovery(),
		middleware.StructuredLogging(),
	)

	// 健康检查
	health := handler.NewHealthHandler()
	engine.GET("/healthz", health.Healthz)
	engine.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "mode": map[bool]string{true: "demo", false: "production"}[demoMode]})
	})

	store := &DemoAssetStore{assets: make([]map[string]interface{}, 0)}

	// API v1
	v1 := engine.Group("/api/v1")
	v1.Use(middleware.Auth(km))
	v1.Use(middleware.OrgScope())

	// 资产 (demo 内存存储)
	v1.GET("/assets", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"data": store.List(),
			"pagination": gin.H{"has_more": false},
		})
	})
	v1.POST("/assets", func(c *gin.Context) {
		var input map[string]interface{}
		c.ShouldBindJSON(&input)
		input["id"] = fmt.Sprintf("demo-%d", len(store.List())+1)
		input["version"] = 1
		store.Add(input)
		c.JSON(http.StatusCreated, gin.H{"data": input})
	})
	v1.GET("/assets/:id", func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "DEMO mode: use POST /api/v1/assets first"})
	})
	v1.PUT("/assets/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "updated", "version": 2}})
	})
	v1.DELETE("/assets/:id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	// 仪表盘
	v1.GET("/dashboard/overview", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{
			"total_assets": len(store.List()),
			"by_status":    gin.H{"available": len(store.List())},
		}})
	})

	// 组织
	orgH := handler.NewOrgHandler()
	v1.GET("/organizations", orgH.List)
	v1.POST("/organizations", orgH.Create)

	// 登录 (无认证)
	engine.POST("/api/v1/auth/login", func(c *gin.Context) {
		token, _ := km.IssueAccessToken(c, "user-001", "admin", "org-001")
		refreshToken, _ := km.IssueAccessToken(c, "user-001", "admin", "org-001")
		c.JSON(http.StatusOK, gin.H{
			"access_token":  token,
			"refresh_token": refreshToken,
			"user": gin.H{"id": "user-001", "username": "admin", "role": "super_admin"},
		})
	})

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Routes: %d endpoints", len(engine.Routes()))
	log.Printf("Mode: %s", map[bool]string{true: "DEMO (in-memory)", false: "PRODUCTION"}[demoMode])

	return &Server{
		engine:     engine,
		cfg:        cfg,
		keyManager: km,
		demoStore:  store,
		httpServer: &http.Server{Addr: addr, Handler: engine},
	}
}

func (s *Server) Start() error {
	log.Printf("Listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop() error {
	log.Println("Shutting down...")
	return s.httpServer.Close()
}
