// Package handler — 领用/归还/转移 Handler
package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/lock"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AssignmentHandler 领用管理
type AssignmentHandler struct {
	repo *repository.AssetRepo
}

func NewAssignmentHandler(repo *repository.AssetRepo) *AssignmentHandler {
	return &AssignmentHandler{repo: repo}
}

// Assign POST /api/v1/assets/:id/assign (悲观锁)
func (h *AssignmentHandler) Assign(c *gin.Context) {
	assetID := c.Param("id")

	// 悲观锁获取资产
	asset, err := h.repo.LockForUpdate(c.Request.Context(), assetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found"})
		return
	}

	if asset.Status != "available" {
		c.JSON(http.StatusConflict, gin.H{
			"error": fmt.Sprintf("asset is %s", asset.Status),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"assignment_id": uuid.New().String(),
		"asset_id":      assetID,
		"status":        "active",
	})
}

// Release POST /api/v1/assets/:id/release
func (h *AssignmentHandler) Release(c *gin.Context) {
	assetID := c.Param("id")
	now := time.Now()

	_, err := h.repo.LockForUpdate(c.Request.Context(), assetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found"})
		return
	}
	_ = now

	c.JSON(http.StatusOK, gin.H{"asset_id": assetID, "status": "released"})
}

// Transfer POST /api/v1/assets/:id/transfer (字典序锁定防死锁)
func (h *AssignmentHandler) Transfer(c *gin.Context) {
	var input struct {
		ToUserID string `json:"to_user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	assetID := c.Param("id")
	ids := lock.SortedAssetIDs([]string{assetID})

	if err := lock.ValidateSortedOrder(ids); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_, err := h.repo.LockAssetsSorted(c.Request.Context(), ids)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	_ = input

	c.JSON(http.StatusOK, gin.H{
		"asset_id":   assetID,
		"to_user_id": input.ToUserID,
		"status":     "transferred",
	})
}
