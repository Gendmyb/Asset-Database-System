// Package api — 生产模式路由注册
// Phase B §9: 从 server.go 拆分
// Phase C: 新增 RBAC 中间件 + 用户管理路由
package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/handler"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/api/middleware"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// registerProductionRoutes 注册生产模式 (PG) 路由
func registerProductionRoutes(v1 *gin.RouterGroup, pool *pgxpool.Pool) {
	assetRepo := repository.NewAssetRepo()
	assignmentRepo := repository.NewAssignmentRepo()
	dashRepo := repository.NewDashboardRepo()
	userRepo := repository.NewUserRepo()
	settingsRepo := repository.NewSettingsRepo()
	maintenanceRepo := repository.NewMaintenanceRepo()
	stocktakeRepo := repository.NewStocktakeRepo()

	// 确保种子用户存在
	_ = userRepo.EnsureSeedUsers(context.Background(), pool)

	assetV2 := handler.NewAssetV2Handler(assetRepo, settingsRepo, pool)
	assignmentH := handler.NewAssignmentHandler(assignmentRepo, pool)
	maintenanceSvc := service.NewMaintenanceService(maintenanceRepo, assetRepo, assignmentRepo, settingsRepo)
	maintenanceH := handler.NewMaintenanceHandler(maintenanceSvc, pool)
	stocktakeSvc := service.NewStocktakeService(stocktakeRepo, assetRepo, settingsRepo)
	stocktakeH := handler.NewStocktakeHandler(stocktakeSvc, pool)

	// === RBAC 分组 ===
	// viewer+ (默认 — 所有已认证用户)
	viewer := v1.Group("")

	// manager+ 写操作
	manager := v1.Group("")
	manager.Use(middleware.RequireRole("manager"))

	// admin+ 管理操作
	admin := v1.Group("")
	admin.Use(middleware.RequireRole("admin"))

	// ---- viewer+ 读接口 ----

	// 系统设置 (读 + 写都是 admin)
	admin.GET("/settings", func(c *gin.Context) {
		all, err := settingsRepo.GetAll(c.Request.Context(), pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": all})
	})
	admin.PUT("/settings", func(c *gin.Context) {
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
	viewer.GET("/settings/next-tag", func(c *gin.Context) {
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

	// 资产类型 (viewer+)
	viewer.GET("/asset-types", func(c *gin.Context) {
		types, err := dashRepo.ListAssetTypes(c.Request.Context(), pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": types})
	})

	// 用户列表 (viewer+)
	viewer.GET("/users", func(c *gin.Context) {
		users, err := userRepo.ListByOrg(c.Request.Context(), pool, c.GetString("org_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": users})
	})
	viewer.GET("/users/:id", func(c *gin.Context) {
		name, _ := userRepo.GetUsername(c.Request.Context(), pool, c.Param("id"))
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": c.Param("id"), "username": name}})
	})

	// 资产 CRUD
	viewer.GET("/assets", assetV2.ListAssets)
	viewer.GET("/assets/:id", assetV2.GetAsset)
	manager.POST("/assets", assetV2.CreateAsset)
	manager.POST("/assets/batch", assetV2.CreateAssetBatch)
	manager.PUT("/assets/:id", assetV2.UpdateAsset)
	manager.DELETE("/assets/:id", assetV2.DeleteAsset)
	manager.POST("/assets/:id/transition", assetV2.LifecycleTransition)

	// 历史记录 (viewer+)
	viewer.GET("/assets/:id/history", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}})
	})

	// 领用管理 (manager+)
	manager.POST("/assets/:id/assign", assignmentH.Assign)
	manager.POST("/assets/:id/release", assignmentH.Release)
	manager.POST("/assets/:id/transfer", assignmentH.Transfer)
	manager.POST("/assets/:id/borrow", assignmentH.Borrow)

	// 领用查询 (viewer+)
	// 领用查询 (viewer+)
	viewer.GET("/assignments", func(c *gin.Context) {
		var limit int
		if l, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
			limit = l
		} else {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}
		overdue, _ := strconv.ParseBool(c.Query("overdue"))

		f := repository.AssignmentFilter{
			OrgID:      c.GetString("org_id"),
			Status:     c.Query("status"),
			Type:       c.Query("type"),
			AssignedTo: c.Query("assigned_to"),
			Overdue:    overdue,
			Cursor:     c.Query("cursor"),
			Limit:      limit,
		}

		rows, nextCursor, hasMore, err := assignmentRepo.ListAssignments(c.Request.Context(), pool, f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data": rows,
			"pagination": gin.H{
				"next_cursor": nextCursor,
				"has_more":    hasMore,
			},
		})
	})

	viewer.GET("/users/:id/assignments", func(c *gin.Context) {
		var limit int
		if l, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
			limit = l
		} else {
			limit = 50
		}
		if limit > 200 {
			limit = 200
		}

		f := repository.AssignmentFilter{
			OrgID:      c.GetString("org_id"),
			AssignedTo: c.Param("id"),
			Cursor:     c.Query("cursor"),
			Limit:      limit,
		}

		rows, nextCursor, hasMore, err := assignmentRepo.ListAssignments(c.Request.Context(), pool, f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data": rows,
			"pagination": gin.H{
				"next_cursor": nextCursor,
				"has_more":    hasMore,
			},
		})
	})

	viewer.GET("/assets/:id/assignments", func(c *gin.Context) {
		a, err := assignmentRepo.GetActiveAssignment(c.Request.Context(), pool, c.Param("id"))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"data": nil})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": a})
	})

	// 仪表盘 (viewer+)
	viewer.GET("/dashboard/overview", func(c *gin.Context) {
		stats, err := dashRepo.GetStats(c.Request.Context(), pool, c.GetString("org_id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": stats})
	})

	// 组织管理 (admin+)
	orgRepo := repository.NewOrgRepo()
	orgH := handler.NewOrgHandler(orgRepo, pool)
	admin.GET("/organizations", orgH.List)
	admin.POST("/organizations", orgH.Create)
	admin.GET("/organizations/:id", orgH.Get)
	admin.GET("/organizations/:id/subtree", orgH.Subtree)

	// 位置管理 (viewer 读, admin 写)
	locationRepo := repository.NewLocationRepo()
	locationH := handler.NewLocationHandler(locationRepo, pool)
	viewer.GET("/locations", locationH.List)
	viewer.GET("/locations/:id", locationH.Get)
	admin.POST("/locations", locationH.Create)
	admin.PUT("/locations/:id", locationH.Update)
	admin.DELETE("/locations/:id", locationH.Delete)

	// ======== admin 用户管理 ========
	// GET /admin/users — 列出所有用户
	admin.GET("/admin/users", func(c *gin.Context) {
		users, err := userRepo.ListAll(c.Request.Context(), pool)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": users})
	})

	// POST /admin/users — 创建用户
	admin.POST("/admin/users", func(c *gin.Context) {
		var input struct {
			Username string `json:"username" binding:"required"`
			Role     string `json:"role" binding:"required"`
			Email    string `json:"email"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 验证角色
		validRoles := map[string]bool{"super_admin": true, "admin": true, "manager": true, "viewer": true}
		if !validRoles[input.Role] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效角色: " + input.Role})
			return
		}

		// 生成随机密码
		randPwd := uuid.New().String()[:12]
		hash, err := bcrypt.GenerateFromPassword([]byte(randPwd), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
			return
		}

		orgID := c.GetString("org_id")
		if orgID == "" {
			orgID = "00000000-0000-4000-a000-000000000001"
		}

		id, err := userRepo.CreateUser(c.Request.Context(), pool, input.Username, string(hash), input.Role, input.Email, orgID)
		if err != nil {
			// 检查是否 username 重复
			if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
				c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建失败: %v", err)})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"data": gin.H{
			"id":              id,
			"username":        input.Username,
			"role":            input.Role,
			"email":           input.Email,
			"initial_password": randPwd,
		}})
	})

	// PUT /admin/users/:id — 更新用户
	admin.PUT("/admin/users/:id", func(c *gin.Context) {
		var input struct {
			Role   *string `json:"role"`
			Status *string `json:"status"`
			Email  *string `json:"email"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		updates := make(map[string]interface{})
		if input.Role != nil {
			updates["role"] = *input.Role
		}
		if input.Status != nil {
			updates["status"] = *input.Status
		}
		if input.Email != nil {
			updates["email"] = *input.Email
		}
		if len(updates) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "没有提供任何更新字段"})
			return
		}

		if err := userRepo.UpdateUser(c.Request.Context(), pool, c.Param("id"), updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("更新失败: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})

	// POST /admin/users/:id/reset-password — 重置密码
	admin.POST("/admin/users/:id/reset-password", func(c *gin.Context) {
		randPwd := uuid.New().String()[:12]
		hash, err := bcrypt.GenerateFromPassword([]byte(randPwd), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
			return
		}

		if err := userRepo.SetPasswordHash(c.Request.Context(), pool, c.Param("id"), string(hash)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("重置失败: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"new_password": randPwd}})
	})

	// ======== Phase F: 维修/保养工单 ========
	// 工单 CRUD (manager+ 写, viewer+ 读)
	manager.POST("/maintenance-orders", maintenanceH.CreateMaintenanceOrder)
	viewer.GET("/maintenance-orders", maintenanceH.ListMaintenanceOrders)
	viewer.GET("/maintenance-orders/:id", maintenanceH.GetMaintenanceOrder)
	manager.POST("/maintenance-orders/:id/start", maintenanceH.StartMaintenanceOrder)
	manager.POST("/maintenance-orders/:id/complete", maintenanceH.CompleteMaintenanceOrder)
	manager.POST("/maintenance-orders/:id/cancel", maintenanceH.CancelMaintenanceOrder)

	// 报废 (admin+)
	admin.POST("/assets/:id/retire", maintenanceH.RetireAsset)

	// ======== Phase G: 盘点管理 ========
	admin.POST("/stocktakes", stocktakeH.CreatePlan)
	viewer.GET("/stocktakes", stocktakeH.ListPlans)
	viewer.GET("/stocktakes/:id", stocktakeH.GetPlan)
	admin.POST("/stocktakes/:id/start", stocktakeH.StartPlan)
	manager.PUT("/stocktakes/:id/items/:itemId", stocktakeH.UpdateItem)
	manager.POST("/stocktakes/:id/items", stocktakeH.AddSurplusItem)
	admin.POST("/stocktakes/:id/complete", stocktakeH.CompletePlan)
	viewer.GET("/stocktakes/:id/report", stocktakeH.GetPlanReport)
}
