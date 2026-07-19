// Package handler — 审批流 & 通知规则管理 Handler (Wave 2 G6/G7)
package handler

import (
	"net/http"
	"strconv"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ApprovalHandler 审批流 Handler
type ApprovalHandler struct {
	svc  *service.ApprovalService
	pool *pgxpool.Pool
}

func NewApprovalHandler(svc *service.ApprovalService, pool *pgxpool.Pool) *ApprovalHandler {
	return &ApprovalHandler{svc: svc, pool: pool}
}

// List GET /admin/approvals?status=pending&limit=50
func (h *ApprovalHandler) List(c *gin.Context) {
	orgID := c.GetString("org_id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	status := c.Query("status")

	rows, err := h.svc.List(c.Request.Context(), orgID, status, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []repository.ApprovalRequestRow{}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// Get GET /admin/approvals/:id
func (h *ApprovalHandler) Get(c *gin.Context) {
	row, err := h.svc.Get(c.Request.Context(), c.Param("id"), c.GetString("org_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "审批请求不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": row})
}

// Approve POST /admin/approvals/:id/approve
func (h *ApprovalHandler) Approve(c *gin.Context) {
	if err := h.svc.Approve(c.Request.Context(), c.Param("id"), c.GetString("org_id"), c.GetString("user_id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": c.Param("id"), "status": "approved"}})
}

// Reject POST /admin/approvals/:id/reject
func (h *ApprovalHandler) Reject(c *gin.Context) {
	var input struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&input)
	if err := h.svc.Reject(c.Request.Context(), c.Param("id"), c.GetString("org_id"), c.GetString("user_id"), input.Reason); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": c.Param("id"), "status": "rejected"}})
}

// ============================================================
// NotifyRuleHandler 通知规则管理 Handler
// ============================================================

// NotifyRuleHandler 通知规则 Handler
type NotifyRuleHandler struct {
	repo *repository.NotifyRepo
	pool *pgxpool.Pool
}

func NewNotifyRuleHandler(repo *repository.NotifyRepo, pool *pgxpool.Pool) *NotifyRuleHandler {
	return &NotifyRuleHandler{repo: repo, pool: pool}
}

// List GET /admin/notify/rules
func (h *NotifyRuleHandler) List(c *gin.Context) {
	rows, err := h.repo.ListRules(c.Request.Context(), h.pool, c.GetString("org_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []repository.NotifyRuleRow{}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// Create POST /admin/notify/rules
func (h *NotifyRuleHandler) Create(c *gin.Context) {
	var input struct {
		EventType string `json:"event_type" binding:"required"`
		Channel   string `json:"channel" binding:"required"`
		Target    string `json:"target"`
		Active    *bool  `json:"active"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validChannels := map[string]bool{"email": true, "dingtalk": true, "wecom": true, "feishu": true}
	if !validChannels[input.Channel] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效渠道: " + input.Channel})
		return
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	row := &repository.NotifyRuleRow{
		OrgID:     c.GetString("org_id"),
		EventType: input.EventType,
		Channel:   input.Channel,
		Target:    input.Target,
		Active:    active,
	}
	id, err := h.repo.CreateRule(c.Request.Context(), h.pool, row)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{"id": id}})
}

// Delete DELETE /admin/notify/rules/:id
// 全局规则 (org_id IS NULL) 仅 super_admin 可删; 组织规则限本组织。
func (h *NotifyRuleHandler) Delete(c *gin.Context) {
	orgID := c.GetString("org_id")
	role := c.GetString("role")
	id := c.Param("id")

	// 先取规则以判断是否为全局规则 (org_id IS NULL)
	rule, err := h.repo.GetRule(c.Request.Context(), h.pool, id, orgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "通知规则不存在"})
		return
	}
	// 全局规则仅 super_admin 可删
	if rule.OrgID == "" && role != "super_admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "全局规则仅超级管理员可删除"})
		return
	}

	if err := h.repo.DeleteRule(c.Request.Context(), h.pool, id, orgID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// ListDeliveries GET /admin/notify/deliveries?limit=50
// 非 super_admin 仅返回本组织投递记录; super_admin 可看全部。
func (h *NotifyRuleHandler) ListDeliveries(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	isSuperAdmin := c.GetString("role") == "super_admin"
	rows, err := h.repo.ListDeliveries(c.Request.Context(), h.pool, c.GetString("org_id"), isSuperAdmin, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []repository.NotifyDeliveryRow{}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}
