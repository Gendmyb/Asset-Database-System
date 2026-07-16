// Package api — Gin Server 组装
package api

import (
	"fmt"
	"log"
	"net/http"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/handler"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/middleware"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/crypto"
	"github.com/gin-gonic/gin"
)

type Server struct {
	engine     *gin.Engine
	cfg        *config.Config
	keyManager *crypto.KeyManager
	httpServer *http.Server
}

func NewServer(cfg *config.Config, km *crypto.KeyManager, assetRepo handler.AssetRepository) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()

	// 中间件链
	engine.Use(
		middleware.RequestID(),
		middleware.Recovery(),
		middleware.StructuredLogging(),
		middleware.RateLimit(),
	)

	// 健康检查 (无需认证)
	health := handler.NewHealthHandler()
	engine.GET("/healthz", health.Healthz)
	engine.GET("/readyz", health.Readyz(nil, nil))

	// API v1 (需认证)
	v1 := engine.Group("/api/v1")
	v1.Use(middleware.Auth(km))
	v1.Use(middleware.OrgScope())

	// 资产路由
	ah := handler.NewAssetHandler(assetRepo)
	v1.GET("/assets", ah.ListAssets)
	v1.POST("/assets", ah.CreateAsset)
	v1.GET("/assets/:id", ah.GetAsset)
	v1.PUT("/assets/:id", ah.UpdateAsset)
	v1.DELETE("/assets/:id", ah.DeleteAsset)
	v1.GET("/assets/:id/history", ah.GetHistory)

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
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
	log.Printf("JWT public key: %s...", s.keyManager.HexEncodePublicKey()[:32])
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop() error {
	log.Println("Shutting down...")
	return s.httpServer.Close()
}
