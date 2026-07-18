// Package api — DEMO 模式路由注册 (冻结, 仅维护)
// Phase B §9: 从 server.go 拆分
package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/handler"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// registerDemoRoutes 注册 DEMO 模式 (内存) 路由 (冻结不扩展)
func registerDemoRoutes(v1 *gin.RouterGroup, demoRepo *DemoAssetRepo) {
	// 系统设置 (DEMO)
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

	// 资产类型 (DEMO)
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

	// 用户列表 (DEMO)
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

	// 资产 CRUD
	assetHandler := handler.NewAssetHandler(demoRepo)
	v1.GET("/assets", assetHandler.ListAssets)
	v1.POST("/assets", assetHandler.CreateAsset)
	v1.GET("/assets/:id", assetHandler.GetAsset)
	v1.PUT("/assets/:id", assetHandler.UpdateAsset)
	v1.DELETE("/assets/:id", assetHandler.DeleteAsset)
	v1.GET("/assets/:id/history", assetHandler.GetHistory)

	// 生命周期状态转换 (DEMO)
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

	// 领用管理 (DEMO)
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
		updates := map[string]interface{}{"status": "assigned"}
		if asset.Properties == nil {
			asset.Properties = make(map[string]interface{})
		}
		asset.Properties["assigned_to"] = input.ToUserID
		updates["properties"] = asset.Properties
		_, err = demoRepo.Update(assetID, updates, asset.Version)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"asset_id":   assetID,
				"to_user_id": input.ToUserID,
				"from_user":  c.GetString("user_id"),
				"status":     "assigned",
			},
		})
	})

	v1.GET("/assets/:id/assignments", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": nil})
	})

	// 仪表盘 (DEMO)
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

	// 组织管理 (DEMO)
	demoOrgStore := handler.NewOrgStore()
	v1.GET("/organizations", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": demoOrgStore.Tree()})
	})
	v1.POST("/organizations", func(c *gin.Context) {
		var input struct {
			Name     string  `json:"name" binding:"required"`
			ParentID *string `json:"parent_id"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		org, err := demoOrgStore.Add(input.Name, input.ParentID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"data": org})
	})
	v1.GET("/organizations/:id", func(c *gin.Context) {
		org := demoOrgStore.Find(c.Param("id"))
		if org == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": org})
	})
	v1.GET("/organizations/:id/subtree", func(c *gin.Context) {
		org := demoOrgStore.Find(c.Param("id"))
		if org == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		subtree := demoOrgStore.Subtree(org.Path)
		c.JSON(http.StatusOK, gin.H{"data": subtree})
	})

	// 位置管理 (DEMO)
	demoLocStore := handler.NewLocationStore()
	v1.GET("/locations", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": demoLocStore.List(c.GetString("org_id"))})
	})
	v1.POST("/locations", func(c *gin.Context) {
		var input struct {
			Name     string  `json:"name" binding:"required"`
			ParentID *string `json:"parent_id"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		loc := &handler.Location{
			ID:        uuid.New().String(),
			Name:      input.Name,
			ParentID:  input.ParentID,
			OrgID:     c.GetString("org_id"),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		demoLocStore.Add(loc)
		c.JSON(http.StatusCreated, gin.H{"data": loc})
	})
}
