// Package api — Gin Server (支持 demo 模式和生产 PG 模式)
// Phase B §9: 路由拆分为 routes.go / routes_demo.go
package api

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/handler"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/middleware"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/Gendmyb/Asset-Database-System/assetserver/web"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	engine     *gin.Engine
	cfg        *config.Config
	keyManager *crypto.KeyManager
	httpServer *http.Server
	demoRepo   *DemoAssetRepo
}

// DemoAssetRepo 演示模式内存仓库 (实现 handler.AssetRepository 接口)
type DemoAssetRepo struct {
	mu      sync.RWMutex
	assets  map[string]*handler.Asset
	history map[string][]handler.AuditLog
}

func NewDemoAssetRepo() *DemoAssetRepo {
	return &DemoAssetRepo{
		assets:  make(map[string]*handler.Asset),
		history: make(map[string][]handler.AuditLog),
	}
}

func (r *DemoAssetRepo) List(orgID string, search string, typeID string, status string, cursor string, limit int) ([]handler.Asset, string, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []handler.Asset
	for _, a := range r.assets {
		if a.OrgID != orgID || a.DeletedAt != nil {
			continue
		}
		if search != "" && !containsFold(a.Name, search) && !containsFold(a.AssetTag, search) {
			continue
		}
		if typeID != "" && a.TypeID != typeID {
			continue
		}
		if status != "" && a.Status != status {
			continue
		}
		result = append(result, *a)
	}

	sortAssetsDesc(result)

	offset := 0
	if cursor != "" {
		if n, err := strconv.Atoi(cursor); err == nil && n > 0 {
			offset = n
		}
	}
	total := len(result)
	if offset >= total {
		return []handler.Asset{}, "", false, nil
	}
	end := offset + limit
	hasMore := end < total
	if end > total {
		end = total
	}
	page := result[offset:end]
	nextCursor := ""
	if hasMore {
		nextCursor = strconv.Itoa(end)
	}
	return page, nextCursor, hasMore, nil
}

func (r *DemoAssetRepo) GetByID(id string) (*handler.Asset, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.assets[id]
	if !ok || a.DeletedAt != nil {
		return nil, fmt.Errorf("asset not found")
	}
	cp := *a
	return &cp, nil
}

func (r *DemoAssetRepo) Create(asset *handler.Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assets[asset.ID] = asset
	return nil
}

func (r *DemoAssetRepo) Update(id string, updates map[string]interface{}, version int) (*handler.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	a, ok := r.assets[id]
	if !ok || a.DeletedAt != nil {
		return nil, fmt.Errorf("not found")
	}
	if a.Version != version {
		return nil, fmt.Errorf("version conflict")
	}

	setStr := func(field *string, key string) {
		if v, ok := updates[key]; ok && v != nil {
			s, is := v.(string)
			if is {
				*field = s
			}
		}
	}
	setStrPtr := func(field **string, key string) {
		if v, ok := updates[key]; ok && v != nil {
			s, is := v.(string)
			if is {
				*field = &s
			}
		}
	}

	setStr(&a.Name, "name")
	setStrPtr(&a.SerialNumber, "serial_number")
	setStrPtr(&a.Manufacturer, "manufacturer")
	setStrPtr(&a.Model, "model")
	setStr(&a.LifecycleState, "lifecycle_state")
	setStr(&a.Status, "status")

	a.Version++
	a.UpdatedAt = time.Now()

	cp := *a
	return &cp, nil
}

func (r *DemoAssetRepo) SoftDelete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.assets[id]
	if !ok || a.DeletedAt != nil {
		return fmt.Errorf("asset not found")
	}
	now := time.Now()
	a.DeletedAt = &now
	return nil
}

func (r *DemoAssetRepo) GetHistory(assetID string, limit int) ([]handler.AuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.history[assetID], nil
}

func containsFold(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			c1, c2 := s[i+j], sub[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 32
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 32
			}
			if c1 != c2 {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func sortAssetsDesc(assets []handler.Asset) {
	for i := 0; i < len(assets); i++ {
		for j := i + 1; j < len(assets); j++ {
			if assets[i].UpdatedAt.Before(assets[j].UpdatedAt) {
				assets[i], assets[j] = assets[j], assets[i]
			}
		}
	}
}

// NewServer 创建 Gin Server (依赖注入入口)
func NewServer(cfg *config.Config, km *crypto.KeyManager, pool *pgxpool.Pool, demoMode bool) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	engine.Use(
		middleware.RequestID(),
		middleware.Recovery(),
		middleware.StructuredLogging(),
	)

	// 健康检查 (无需认证)
	healthH := handler.NewHealthHandler()
	engine.GET("/healthz", healthH.Healthz)
	engine.GET("/readyz", func(c *gin.Context) {
		mode := "production"
		if demoMode {
			mode = "demo"
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "mode": mode})
	})

	// API v1 (需要认证)
	v1 := engine.Group("/api/v1")
	v1.Use(middleware.Auth(km))
	v1.Use(middleware.OrgScope())

	var demoRepo *DemoAssetRepo

	if demoMode {
		demoRepo = NewDemoAssetRepo()
		seedDemoAssets(demoRepo)
		registerDemoRoutes(v1, demoRepo)
	} else {
		registerProductionRoutes(v1, pool)
	}

	// Agent 状态 (轻量)
	v1.GET("/dashboard/agents", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{"online": 0, "offline": 0, "total": 0},
		})
	})
	v1.GET("/agents", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
	})

	// 登录 (无需认证)
	engine.POST("/api/v1/auth/login", func(c *gin.Context) {
		var input struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
			return
		}
		if input.Username != "admin" || input.Password != "admin" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		orgUUID := "00000000-0000-4000-a000-000000000001"
		userUUID := "00000000-0000-4000-a000-000000000010"
		token, _ := km.IssueAccessToken(c, userUUID, "super_admin", orgUUID)
		c.JSON(http.StatusOK, gin.H{
			"access_token": token,
			"user": gin.H{
				"id":       userUUID,
				"username": "admin",
				"role":     "super_admin",
				"org_id":   orgUUID,
			},
		})
	})

	// 静态文件服务 (生产模式: 嵌入前端 SPA)
	if !demoMode {
		engine.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			if strings.HasPrefix(path, "/api") || path == "/healthz" || path == "/readyz" {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.FileFromFS(path, web.Handler())
		})
	}

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Routes: %d endpoints", len(engine.Routes()))
	log.Printf("Mode: %s", map[bool]string{true: "DEMO (in-memory)", false: "PRODUCTION"}[demoMode])

	return &Server{
		engine:     engine,
		cfg:        cfg,
		keyManager: km,
		demoRepo:   demoRepo,
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

func strPtr(s string) *string { return &s }

// seedDemoAssets 预置演示数据
func seedDemoAssets(repo *DemoAssetRepo) {
	now := time.Now()
	orgUUID := "00000000-0000-4000-a000-000000000001"
	demo := []handler.Asset{
		{
			ID: "demo-001", AssetTag: "AST-001", Name: "MacBook Pro 16\" M4",
			TypeID: "10000000-0000-4000-a000-000000000001", OrgID: orgUUID,
			Manufacturer: strPtr("Apple Inc."), Model: strPtr("MacBookPro18,1"),
			SerialNumber: strPtr("C02ZJ12345"), LifecycleState: "utilization",
			Status: "assigned", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "demo-002", AssetTag: "AST-002", Name: "Dell PowerEdge R750",
			TypeID: "10000000-0000-4000-a000-000000000002", OrgID: orgUUID,
			Manufacturer: strPtr("Dell Technologies"), Model: strPtr("PowerEdge R750"),
			SerialNumber: strPtr("DELL789012"), LifecycleState: "utilization",
			Status: "available", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "demo-003", AssetTag: "AST-003", Name: "Dell UltraSharp U2723QE",
			TypeID: "10000000-0000-4000-a000-000000000003", OrgID: orgUUID,
			Manufacturer: strPtr("Dell Technologies"), Model: strPtr("U2723QE"),
			SerialNumber: strPtr("MON345678"), LifecycleState: "deployment",
			Status: "available", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "demo-004", AssetTag: "AST-004", Name: "ThinkPad X1 Carbon Gen 12",
			TypeID: "10000000-0000-4000-a000-000000000001", OrgID: orgUUID,
			Manufacturer: strPtr("Lenovo"), Model: strPtr("21KC"),
			SerialNumber: strPtr("LEN901234"), LifecycleState: "procurement",
			Status: "maintenance", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "demo-005", AssetTag: "AST-005", Name: "Cisco Catalyst 9300",
			TypeID: "10000000-0000-4000-a000-000000000004", OrgID: orgUUID,
			Manufacturer: strPtr("Cisco Systems"), Model: strPtr("C9300-48P"),
			SerialNumber: strPtr("CIS567890"), LifecycleState: "retirement",
			Status: "maintenance", Version: 1, CreatedAt: now, UpdatedAt: now,
		},
	}
	for _, a := range demo {
		asset := a
		repo.assets[asset.ID] = &asset
	}
}
