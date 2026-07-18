// Package handler — 资产 Handler (Phase 2: 集成 pgx Repository + 锁)
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/domain"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AssetV2Handler Phase 2 资产处理器 (集成真实 Repository)
type AssetV2Handler struct {
	repo         *repository.AssetRepo
	settingsRepo *repository.SettingsRepo
}

func NewAssetV2Handler(repo *repository.AssetRepo, settingsRepo *repository.SettingsRepo) *AssetV2Handler {
	return &AssetV2Handler{repo: repo, settingsRepo: settingsRepo}
}

// AssetResponse 统一响应
type AssetResponse struct {
	ID             string          `json:"id"`
	AssetTag       string          `json:"asset_tag"`
	Name           string          `json:"name"`
	TypeID         string          `json:"type_id"`
	OrgID          string          `json:"org_id"`
	SerialNumber   *string         `json:"serial_number"`
	Manufacturer   *string         `json:"manufacturer"`
	Model          *string         `json:"model"`
	LifecycleState string          `json:"lifecycle_state"`
	Status         string          `json:"status"`
	Properties     json.RawMessage `json:"properties"`
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func rowToResponse(r *repository.AssetRow) AssetResponse {
	return AssetResponse{
		ID: r.ID, AssetTag: r.AssetTag, Name: r.Name,
		TypeID: r.TypeID, OrgID: r.OrgID,
		SerialNumber: r.SerialNumber, Manufacturer: r.Manufacturer, Model: r.Model,
		LifecycleState: r.LifecycleState, Status: r.Status,
		Properties: r.Properties, Version: r.Version,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

// ListAssets GET /api/v1/assets
func (h *AssetV2Handler) ListAssets(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit > 200 {
		limit = 200
	}

	f := repository.AssetFilter{
		OrgID:        c.GetString("org_id"),
		Search:       c.Query("search"),
		TypeID:       c.Query("type_id"),
		Status:       c.Query("status"),
		Lifecycle:    c.Query("lifecycle"),
		Manufacturer: c.Query("manufacturer"),
		Cursor:       c.Query("cursor"),
		Limit:        limit,
	}

	rows, nextCursor, hasMore, err := h.repo.List(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data := make([]AssetResponse, len(rows))
	for i, r := range rows {
		data[i] = rowToResponse(&r)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"pagination": gin.H{
			"next_cursor": nextCursor,
			"has_more":    hasMore,
		},
	})
}

// GetAsset GET /api/v1/assets/:id
func (h *AssetV2Handler) GetAsset(c *gin.Context) {
	row, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rowToResponse(row)})
}

// CreateAsset POST /api/v1/assets
func (h *AssetV2Handler) CreateAsset(c *gin.Context) {
	var input struct {
		AssetTag       string          `json:"asset_tag"`
		Name           string          `json:"name" binding:"required"`
		TypeID         string          `json:"type_id" binding:"required"`
		SerialNumber   *string         `json:"serial_number"`
		Manufacturer   *string         `json:"manufacturer"`
		Model          *string         `json:"model"`
		LifecycleState string          `json:"lifecycle_state"`
		Properties     json.RawMessage `json:"properties"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 自动生成编号
	if input.AssetTag == "" && h.settingsRepo != nil {
		orgID, _ := c.Get("org_id")
		oid, _ := orgID.(string)
		if oid == "" {
			oid = "00000000-0000-4000-a000-000000000001"
		}
		tag, _ := h.settingsRepo.NextAssetTag(c.Request.Context(), oid)
		input.AssetTag = tag
	}

	now := time.Now()
	row := &repository.AssetRow{
		ID: uuid.New().String(), AssetTag: input.AssetTag, Name: input.Name,
		TypeID: input.TypeID, OrgID: c.GetString("org_id"),
		SerialNumber: input.SerialNumber, Manufacturer: input.Manufacturer,
		Model: input.Model, LifecycleState: "procurement", Status: "available",
		Properties: input.Properties, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}

	if err := h.repo.Create(c.Request.Context(), row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": rowToResponse(row)})
}

// UpdateAsset PUT /api/v1/assets/:id (乐观锁)
func (h *AssetV2Handler) UpdateAsset(c *gin.Context) {
	vStr := strings.Trim(c.GetHeader("If-Match"), "\"")
	version, err := strconv.Atoi(vStr)
	if err != nil {
		c.JSON(http.StatusPreconditionRequired, gin.H{"error": "If-Match header required"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 生命周期状态转换校验
	if newState, ok := updates["lifecycle_state"].(string); ok && newState != "" {
		current, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if err := domain.ValidateTransition(
			domain.LifecycleState(current.LifecycleState),
			domain.LifecycleState(newState),
		); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	row, err := h.repo.UpdateWithRetry(c.Request.Context(), c.Param("id"), updates, version)
	if err != nil {
		if strings.Contains(err.Error(), "version conflict") {
			c.JSON(http.StatusConflict, gin.H{"error": "version conflict"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rowToResponse(row)})
}

// DeleteAsset DELETE /api/v1/assets/:id (软删除)
func (h *AssetV2Handler) DeleteAsset(c *gin.Context) {
	if err := h.repo.SoftDelete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// LifecycleTransition POST /api/v1/assets/:id/transition (悲观锁 + 状态校验)
func (h *AssetV2Handler) LifecycleTransition(c *gin.Context) {
	var input struct {
		To string `json:"to" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 悲观锁获取资产
	row, err := h.repo.LockForUpdate(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	// 验证状态转换
	if err := domain.ValidateTransition(
		domain.LifecycleState(row.LifecycleState),
		domain.LifecycleState(input.To),
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.repo.UpdateWithRetry(c.Request.Context(), c.Param("id"),
		map[string]interface{}{"lifecycle_state": input.To}, row.Version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rowToResponse(updated)})
}
