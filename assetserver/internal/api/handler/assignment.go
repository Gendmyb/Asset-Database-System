// Package handler — 领用/归还/转移 Handler (PG 真实实现)
package handler

import (
	"net/http"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
)

// AssignmentHandler 领用管理
type AssignmentHandler struct {
	repo *repository.AssignmentRepo
}

func NewAssignmentHandler(repo *repository.AssignmentRepo) *AssignmentHandler {
	return &AssignmentHandler{repo: repo}
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

	assignmentID, err := h.repo.Assign(c.Request.Context(), assetID, orgID, input.AssignedTo, userID, input.Notes)
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

	if err := h.repo.Release(c.Request.Context(), assetID); err != nil {
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

	if err := h.repo.Transfer(c.Request.Context(), assetID, input.ToUserID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{"asset_id": assetID, "to_user_id": input.ToUserID, "status": "transferred"},
	})
}
