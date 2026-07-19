// Package handler — 领用/归还/转移 Handler (PG 真实实现)
package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AssignmentHandler 领用管理
type AssignmentHandler struct {
	repo         *repository.AssignmentRepo
	svc          *service.AssignmentService
	pool         *pgxpool.Pool
	settingsRepo *repository.SettingsRepo
	approvalSvc  *service.ApprovalService
}

func NewAssignmentHandler(repo *repository.AssignmentRepo, pool *pgxpool.Pool) *AssignmentHandler {
	return &AssignmentHandler{
		repo: repo,
		svc:  service.NewAssignmentService(repo),
		pool: pool,
	}
}

// WithApproval 注入审批服务与设置仓库, 启用领用审批门
func (h *AssignmentHandler) WithApproval(settingsRepo *repository.SettingsRepo, approvalSvc *service.ApprovalService) *AssignmentHandler {
	h.settingsRepo = settingsRepo
	h.approvalSvc = approvalSvc
	return h
}

// Assign POST /api/v1/assets/:id/assign
func (h *AssignmentHandler) Assign(c *gin.Context) {
	assetID := c.Param("id")

	var input struct {
		AssignedTo string `json:"assigned_to" binding:"required"`
		Notes      string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	orgID := c.GetString("org_id")
	userID := c.GetString("user_id")

	// 审批门: 若启用领用审批, 创建 pending 审批请求, 待通过后才执行领用
	if h.approvalSvc != nil && service.IsApprovalEnabled(c.Request.Context(), h.settingsRepo, h.pool, "assignment") {
		id, err := h.approvalSvc.Create(c.Request.Context(), service.CreateInput{
			ResourceType: service.ApprovalAssignment,
			ResourceID:   assetID,
			RequesterID:  userID,
			OrgID:        orgID,
			Payload: map[string]string{
				"assigned_to": input.AssignedTo,
				"notes":       input.Notes,
			},
		})
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"data": gin.H{
			"approval_id": id,
			"asset_id":    assetID,
			"status":      "pending_approval",
		}})
		return
	}

	assignmentID, err := h.svc.Assign(c.Request.Context(), h.pool, assetID, orgID, input.AssignedTo, userID, input.Notes)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"assignment_id": assignmentID,
			"asset_id":      assetID,
			"assigned_to":   input.AssignedTo,
			"status":        "active",
		},
	})
}

// Release POST /api/v1/assets/:id/release
func (h *AssignmentHandler) Release(c *gin.Context) {
	assetID := c.Param("id")

	var input struct {
		ReturnNotes string `json:"return_notes"`
	}
	// Body is optional for release
	if err := c.ShouldBindJSON(&input); err != nil {
		// ignore parse errors — body may be empty or invalid
	}

	orgID := c.GetString("org_id")
	actorID := c.GetString("user_id")

	if err := h.svc.Release(c.Request.Context(), h.pool, assetID, orgID, actorID, input.ReturnNotes); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"asset_id": assetID, "status": "released"},
	})
}

// Transfer POST /api/v1/assets/:id/transfer
func (h *AssignmentHandler) Transfer(c *gin.Context) {
	assetID := c.Param("id")

	var input struct {
		ToUserID string `json:"to_user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(string)

	if err := h.svc.Transfer(c.Request.Context(), h.pool, assetID, c.GetString("org_id"), input.ToUserID, uid); err != nil {
		if strings.Contains(err.Error(), "借用中") {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"asset_id": assetID, "to_user_id": input.ToUserID, "status": "transferred"},
	})
}

// Borrow POST /api/v1/assets/:id/borrow
// Phase E: 借用资产
func (h *AssignmentHandler) Borrow(c *gin.Context) {
	assetID := c.Param("id")

	var input struct {
		AssignedTo string `json:"assigned_to" binding:"required"`
		DueDate    string `json:"due_date" binding:"required"`
		Notes      string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dueDate, err := time.Parse("2006-01-02", input.DueDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid due_date format, expected YYYY-MM-DD"})
		return
	}

	orgID := c.GetString("org_id")
	userID := c.GetString("user_id")

	assignmentID, err := h.svc.BorrowAsset(c.Request.Context(), h.pool, assetID, orgID, input.AssignedTo, userID, dueDate, input.Notes)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"data": gin.H{
			"assignment_id": assignmentID,
			"asset_id":      assetID,
			"assigned_to":   input.AssignedTo,
			"due_date":      input.DueDate,
			"status":        "active",
		},
	})
}
