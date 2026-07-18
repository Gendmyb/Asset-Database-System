// Package api — 生产模式路由注册
// Phase B §9: 从 server.go 拆分
package api

import (
	"context"
	"net/http"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/handler"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// registerProductionRoutes 注册生产模式 (PG) 路由
func registerProductionRoutes(v1 *gin.RouterGroup, pool *pgxpool.Pool) {
	assetRepo := repository.NewAssetRepo()
	assignmentRepo := repository.NewAssignmentRepo()
	dashRepo := repository.NewDashboardRepo()
	userRepo := repository.NewUserRepo()
	settingsRepo := repository.NewSettingsRepo()

	// 确保种子用户存在
	userRepo.EnsureSeedUsers(context.Background(), pool)

	assetV2 := handler.NewAssetV2Handler(assetRepo, settingsRepo, pool)
	assignmentH := handler.NewAssignmentHandler(assignmentRepo, pool)

	// 系统设置 (PG)
	v1.GET("/settings", func(c *gin.Context) {
		all, err := settingsRepo.GetAll(c.Request.Context(), pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": all})
	})
	v1.PUT("/settings", func(c *gin.Context) {
		var input map[string]string
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		for k, v := range input {
			if err := settingsRepo.Set(c.Request.Context(), pool, k, v); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})
	v1.GET("/settings/next-tag", func(c *gin.Context) {
		orgID := c.GetString("org_id")
		if orgID == "" {
			orgID = "00000000-0000-4000-a000-000000000001"
		}
		tag, err := settingsRepo.NextAssetTag(c.Request.Context(), pool, orgID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"tag": tag}})
	})

	// 资产类型 (PG)
	v1.GET("/asset-types", func(c *gin.Context) {
		types, err := dashRepo.ListAssetTypes(c.Request.Context(), pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": types})
	})

	// 用户列表 (PG)
	v1.GET("/users", func(c *gin.Context) {
		users, err := userRepo.ListByOrg(c.Request.Context(), pool, c.GetString("org_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": users})
	})
	v1.GET("/users/:id", func(c *gin.Context) {
		name, _ := userRepo.GetUsername(c.Request.Context(), pool, c.Param("id"))
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": c.Param("id"), "username": name}})
	})

	// 资产 CRUD (PG)
	v1.GET("/assets", assetV2.ListAssets)
	v1.POST("/assets", assetV2.CreateAsset)
	v1.GET("/assets/:id", assetV2.GetAsset)
	v1.PUT("/assets/:id", assetV2.UpdateAsset)
	v1.DELETE("/assets/:id", assetV2.DeleteAsset)
	v1.POST("/assets/:id/transition", assetV2.LifecycleTransition)

	// 历史记录
	v1.GET("/assets/:id/history", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
	})

	// 领用管理 (PG)
	v1.POST("/assets/:id/assign", assignmentH.Assign)
	v1.POST("/assets/:id/release", assignmentH.Release)
	v1.POST("/assets/:id/transfer", assignmentH.Transfer)

	// 领用查询 (PG)
	v1.GET("/assets/:id/assignments", func(c *gin.Context) {
		a, err := assignmentRepo.GetActiveAssignment(c.Request.Context(), pool, c.Param("id"))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": a})
	})

	// 仪表盘 (PG)
	v1.GET("/dashboard/overview", func(c *gin.Context) {
		stats, err := dashRepo.GetStats(c.Request.Context(), pool, c.GetString("org_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": stats})
	})

	// 组织管理 (PG)
	orgRepo := repository.NewOrgRepo()
	orgH := handler.NewOrgHandler(orgRepo, pool)
	v1.GET("/organizations", orgH.List)
	v1.POST("/organizations", orgH.Create)
	v1.GET("/organizations/:id", orgH.Get)
	v1.GET("/organizations/:id/subtree", orgH.Subtree)

	// 位置管理 (PG)
	locationRepo := repository.NewLocationRepo()
	locationH := handler.NewLocationHandler(locationRepo, pool)
	v1.GET("/locations", locationH.List)
	v1.POST("/locations", locationH.Create)
	v1.GET("/locations/:id", locationH.Get)
	v1.PUT("/locations/:id", locationH.Update)
	v1.DELETE("/locations/:id", locationH.Delete)
}
