// Package api — Gin Server (支持 demo 模式和生产 PG 模式)
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/handler"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/middleware"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/domain"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}
	return result, "", hasMore, nil
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

// NewServer 创建 Gin Server
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

	// 公共路由: 资产类型 + 仪表盘 (两种模式都注册)
	v1.GET("/asset-types", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{
			{"id": "10000000-0000-4000-a000-000000000001", "name": "笔记本电脑", "category": "hardware", "icon": "laptop"},
			{"id": "10000000-0000-4000-a000-000000000002", "name": "服务器", "category": "hardware", "icon": "server"},
			{"id": "10000000-0000-4000-a000-000000000003", "name": "显示器", "category": "hardware", "icon": "monitor"},
			{"id": "10000000-0000-4000-a000-000000000004", "name": "网络设备", "category": "hardware", "icon": "network"},
			{"id": "10000000-0000-4000-a000-000000000005", "name": "打印机", "category": "hardware", "icon": "printer"},
			{"id": "10000000-0000-4000-a000-000000000006", "name": "手机", "category": "hardware", "icon": "phone"},
		}})
	})

	// 用户列表 (两种模式都需要，供领用选择)
	v1.GET("/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{
			{"id": "00000000-0000-4000-a000-000000000010", "username": "admin", "role": "super_admin", "org_id": "00000000-0000-4000-a000-000000000001"},
			{"id": "00000000-0000-4000-a000-000000000020", "username": "张伟", "role": "operator", "org_id": "00000000-0000-4000-a000-000000000001"},
			{"id": "00000000-0000-4000-a000-000000000030", "username": "李娜", "role": "operator", "org_id": "00000000-0000-4000-a000-000000000001"},
			{"id": "00000000-0000-4000-a000-000000000040", "username": "王强", "role": "viewer", "org_id": "00000000-0000-4000-a000-000000000001"},
		}})
	})
	v1.GET("/users/:id", func(c *gin.Context) {
		names := map[string]string{
			"00000000-0000-4000-a000-000000000010": "admin",
			"00000000-0000-4000-a000-000000000020": "张伟",
			"00000000-0000-4000-a000-000000000030": "李娜",
			"00000000-0000-4000-a000-000000000040": "王强",
		}
		name := names[c.Param("id")]
		if name == "" {
			name = "未知用户"
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": c.Param("id"), "username": name}})
	})

	// 系统设置 (两种模式)
	demoSettings := map[string]string{
		"asset_tag_prefix": "AST-",
		"org_name":         "Demo Corp",
	}
	v1.GET("/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": demoSettings})
	})
	v1.PUT("/settings", func(c *gin.Context) {
		var input map[string]string
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		for k, v := range input {
			demoSettings[k] = v
		}
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})
	v1.GET("/settings/next-tag", func(c *gin.Context) {
		orgID := c.GetString("org_id")
		if orgID == "" {
			orgID = "00000000-0000-4000-a000-000000000001"
		}
		prefix := demoSettings["asset_tag_prefix"]
		if prefix == "" {
			prefix = "AST-"
		}
		// 统计当前资产数 +1
		count := 0
		demoRepo.mu.RLock()
		for _, a := range demoRepo.assets {
			if a.OrgID == orgID && a.DeletedAt == nil {
				count++
			}
		}
		demoRepo.mu.RUnlock()
		tag := fmt.Sprintf("%s%03d", prefix, count+1)
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"tag": tag}})
	})

	if demoMode {
		// === DEMO 模式: 使用内存仓库 ===
		demoRepo = NewDemoAssetRepo()
		seedDemoAssets(demoRepo)
		assetHandler := handler.NewAssetHandler(demoRepo)

		// 资产 CRUD
		v1.GET("/assets", assetHandler.ListAssets)
		v1.POST("/assets", assetHandler.CreateAsset)
		v1.GET("/assets/:id", assetHandler.GetAsset)
		v1.PUT("/assets/:id", assetHandler.UpdateAsset)
		v1.DELETE("/assets/:id", assetHandler.DeleteAsset)
		v1.GET("/assets/:id/history", assetHandler.GetHistory)

		// 生命周期状态转换 (DEMO 内存实现)
		v1.POST("/assets/:id/transition", func(c *gin.Context) {
			var input struct {
				To string `json:"to" binding:"required"`
			}
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			asset, err := demoRepo.GetByID(c.Param("id"))
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "资产不存在"})
				return
			}
			if err := domain.ValidateTransition(domain.LifecycleState(asset.LifecycleState), domain.LifecycleState(input.To)); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			_, err = demoRepo.Update(c.Param("id"), map[string]interface{}{"lifecycle_state": input.To}, asset.Version)
			if err != nil {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			updated, _ := demoRepo.GetByID(c.Param("id"))
			c.JSON(http.StatusOK, gin.H{"data": updated})
		})

		// 领用管理 (DEMO 内存实现)
		v1.POST("/assets/:id/assign", func(c *gin.Context) {
			assetID := c.Param("id")
			var input struct {
				AssignedTo string `json:"assigned_to" binding:"required"`
				Notes      string `json:"notes"`
			}
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			asset, err := demoRepo.GetByID(assetID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "资产不存在"})
				return
			}
			if asset.Status != "available" {
				c.JSON(http.StatusConflict, gin.H{"error": "资产当前状态为 " + asset.Status + "，无法领用"})
				return
			}
			_, err = demoRepo.Update(assetID, map[string]interface{}{"status": "assigned"}, asset.Version)
			if err != nil {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusCreated, gin.H{
				"data": gin.H{
					"assignment_id": uuid.New().String(),
					"asset_id":      assetID,
					"assigned_to":   input.AssignedTo,
					"assigned_by":   c.GetString("user_id"),
					"notes":         input.Notes,
					"status":        "active",
				},
			})
		})

		v1.POST("/assets/:id/release", func(c *gin.Context) {
			assetID := c.Param("id")
			asset, err := demoRepo.GetByID(assetID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "资产不存在"})
				return
			}
			if asset.Status != "assigned" {
				c.JSON(http.StatusConflict, gin.H{"error": "资产未被领用，无法归还"})
				return
			}
			_, err = demoRepo.Update(assetID, map[string]interface{}{"status": "available"}, asset.Version)
			if err != nil {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": gin.H{"asset_id": assetID, "status": "released"}})
		})

		v1.POST("/assets/:id/transfer", func(c *gin.Context) {
			assetID := c.Param("id")
			var input struct {
				ToUserID string `json:"to_user_id" binding:"required"`
			}
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			asset, err := demoRepo.GetByID(assetID)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "资产不存在"})
				return
			}
			if asset.Status != "assigned" {
				c.JSON(http.StatusConflict, gin.H{"error": "资产未被领用，无法转移"})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"data": gin.H{
					"asset_id":   assetID,
					"to_user_id": input.ToUserID,
					"from_user":  c.GetString("user_id"),
					"status":     "transferred",
				},
			})
		})

		// 领用查询 (DEMO)
		v1.GET("/assets/:id/assignments", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"data": nil})
		})

		// 仪表盘 (demo 数据)
		v1.GET("/dashboard/overview", func(c *gin.Context) {
			demoRepo.mu.RLock()
			defer demoRepo.mu.RUnlock()
			byStatus := map[string]int64{}
			byLifecycle := map[string]int64{}
			byCategory := map[string]int64{}
			total := int64(0)
			for _, a := range demoRepo.assets {
				if a.DeletedAt != nil {
					continue
				}
				total++
				byStatus[a.Status]++
				byLifecycle[a.LifecycleState]++
				byCategory[a.TypeID]++
			}
			c.JSON(http.StatusOK, gin.H{"data": gin.H{
				"total_assets": total,
				"by_status":    byStatus,
				"by_category":  byCategory,
				"by_lifecycle": byLifecycle,
			}})
		})

	} else {
		// === 生产模式: 使用 PostgreSQL ===
		assetRepo := repository.NewAssetRepo(pool)
		assignmentRepo := repository.NewAssignmentRepo(pool)
		dashRepo := repository.NewDashboardRepo(pool)
		userRepo := repository.NewUserRepo(pool)
		settingsRepo := repository.NewSettingsRepo(pool)

		// 确保种子用户存在
		userRepo.EnsureSeedUsers(context.Background())

		assetV2 := handler.NewAssetV2Handler(assetRepo, settingsRepo)
		assignmentH := handler.NewAssignmentHandler(assignmentRepo)

		// 资产 CRUD (PG)
		v1.GET("/assets", assetV2.ListAssets)
		v1.POST("/assets", assetV2.CreateAsset)
		v1.GET("/assets/:id", assetV2.GetAsset)
		v1.PUT("/assets/:id", assetV2.UpdateAsset)
		v1.DELETE("/assets/:id", assetV2.DeleteAsset)
		v1.POST("/assets/:id/transition", assetV2.LifecycleTransition)

		// 领用管理 (PG)
		v1.POST("/assets/:id/assign", assignmentH.Assign)
		v1.POST("/assets/:id/release", assignmentH.Release)
		v1.POST("/assets/:id/transfer", assignmentH.Transfer)

		// 领用查询 (PG)
		v1.GET("/assets/:id/assignments", func(c *gin.Context) {
			a, err := assignmentRepo.GetActiveAssignment(c.Request.Context(), c.Param("id"))
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"data": nil})
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": a})
		})

		// 仪表盘 (PG 真实数据)
		v1.GET("/dashboard/overview", func(c *gin.Context) {
			stats, err := dashRepo.GetStats(c.Request.Context(), c.GetString("org_id"))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"data": stats})
		})
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

	// 组织管理
	orgH := handler.NewOrgHandler()
	v1.GET("/organizations", orgH.List)
	v1.POST("/organizations", orgH.Create)
	v1.GET("/organizations/:id", orgH.Get)
	v1.GET("/organizations/:id/subtree", orgH.Subtree)

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

		// 简单验证: 开发环境接受 admin/admin
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

func strPtr(s string) *string { return &s }
