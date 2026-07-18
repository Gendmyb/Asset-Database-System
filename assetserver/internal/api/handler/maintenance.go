// Package handler — 维修/保养工单 + 报废 Handler
// Phase F: 维修/保养工单+报废
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MaintenanceHandler 维修/保养工单处理器
type MaintenanceHandler struct {
	svc  *service.MaintenanceService
	pool *pgxpool.Pool
}

// NewMaintenanceHandler 创建 MaintenanceHandler
func NewMaintenanceHandler(svc *service.MaintenanceService, pool *pgxpool.Pool) *MaintenanceHandler {
	return &MaintenanceHandler{svc: svc, pool: pool}
}

// maintenanceOrderResponse 工单响应 (JSON 序列化友好)
type maintenanceOrderResponse struct {
	ID          string     `json:"id"`
	OrderNo     string     `json:"order_no"`
	AssetID     string     `json:"asset_id"`
	OrgID       string     `json:"org_id"`
	Category    string     `json:"category"`
	Status      string     `json:"status"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	ReportedBy  string     `json:"reported_by"`
	Assignee    *string    `json:"assignee,omitempty"`
	Vendor      *string    `json:"vendor,omitempty"`
	Cost        *float64   `json:"cost,omitempty"`
	Resolution  *string    `json:"resolution,omitempty"`
	PrevStatus  string     `json:"prev_status"`
	StartedAt   *string    `json:"started_at,omitempty"`
	FinishedAt  *string    `json:"finished_at,omitempty"`
	CreatedAt   string     `json:"created_at"`
	UpdatedAt   string     `json:"updated_at"`
	Version     int        `json:"version"`
}

func moToResponse(mo *repository.MaintenanceOrder) maintenanceOrderResponse {
	r := maintenanceOrderResponse{
		ID:          mo.ID,
		OrderNo:     mo.OrderNo,
		AssetID:     mo.AssetID,
		OrgID:       mo.OrgID,
		Category:    mo.Category,
		Status:      mo.Status,
		Title:       mo.Title,
		Description: mo.Description,
		ReportedBy:  mo.ReportedBy,
		Assignee:    mo.Assignee,
		Vendor:      mo.Vendor,
		Cost:        mo.Cost,
		Resolution:  mo.Resolution,
		PrevStatus:  mo.PrevStatus,
		CreatedAt:   mo.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   mo.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		Version:     mo.Version,
	}
	if mo.StartedAt != nil {
		s := mo.StartedAt.Format("2006-01-02T15:04:05Z")
		r.StartedAt = &s
	}
	if mo.FinishedAt != nil {
		s := mo.FinishedAt.Format("2006-01-02T15:04:05Z")
		r.FinishedAt = &s
	}
	return r
}

// CreateMaintenanceOrder POST /api/v1/maintenance-orders
func (h *MaintenanceHandler) CreateMaintenanceOrder(c *gin.Context) {
	var input struct {
		AssetID     string  `json:"asset_id" binding:"required"`
		Category    string  `json:"category" binding:"required"`
		Title       string  `json:"title" binding:"required"`
		Description *string `json:"description"`
		Assignee    *string `json:"assignee"`
		Vendor      *string `json:"vendor"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 校验 category
	if input.Category != "repair" && input.Category != "upkeep" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "category must be 'repair' or 'upkeep'"})
		return
	}

	orgID := c.GetString("org_id")
	userID := c.GetString("user_id")

	mo, err := h.svc.CreateOrder(c.Request.Context(), h.pool, orgID, service.CreateOrderInput{
		AssetID:     input.AssetID,
		Category:    input.Category,
		Title:       input.Title,
		Description: input.Description,
		ReportedBy:  userID,
		Assignee:    input.Assignee,
		Vendor:      input.Vendor,
	})
	if err != nil {
		code := http.StatusInternalServerError
		switch err {
		case service.ErrAssetRetired:
			code = http.StatusConflict
		case service.ErrActiveOrderExists:
			code = http.StatusConflict
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": moToResponse(mo)})
}

// ListMaintenanceOrders GET /api/v1/maintenance-orders
func (h *MaintenanceHandler) ListMaintenanceOrders(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit > 200 {
		limit = 200
	}

	f := repository.MaintenanceFilter{
		OrgID:    c.GetString("org_id"),
		Status:   c.Query("status"),
		Category: c.Query("category"),
		AssetID:  c.Query("asset_id"),
		Cursor:   c.Query("cursor"),
		Limit:    limit,
	}

	rows, nextCursor, hasMore, err := h.svc.ListOrders(c.Request.Context(), h.pool, f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data := make([]maintenanceOrderResponse, len(rows))
	for i, r := range rows {
		data[i] = moToResponse(&r)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"pagination": gin.H{
			"next_cursor": nextCursor,
			"has_more":    hasMore,
		},
	})
}

// GetMaintenanceOrder GET /api/v1/maintenance-orders/:id
func (h *MaintenanceHandler) GetMaintenanceOrder(c *gin.Context) {
	mo, err := h.svc.GetOrder(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": moToResponse(mo)})
}

// StartMaintenanceOrder POST /api/v1/maintenance-orders/:id/start
func (h *MaintenanceHandler) StartMaintenanceOrder(c *gin.Context) {
	if err := h.svc.StartOrder(c.Request.Context(), h.pool, c.GetString("org_id"), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "in_progress"}})
}

// CompleteMaintenanceOrder POST /api/v1/maintenance-orders/:id/complete
func (h *MaintenanceHandler) CompleteMaintenanceOrder(c *gin.Context) {
	var input struct {
		Resolution string  `json:"resolution"`
		Cost       float64 `json:"cost"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		// body may be empty
	}

	if err := h.svc.CompleteOrder(c.Request.Context(), h.pool, c.GetString("org_id"), c.Param("id"), input.Resolution, input.Cost); err != nil {
		code := http.StatusInternalServerError
		if err == service.ErrOrderNotFound {
			code = http.StatusNotFound
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "completed"}})
}

// CancelMaintenanceOrder POST /api/v1/maintenance-orders/:id/cancel
func (h *MaintenanceHandler) CancelMaintenanceOrder(c *gin.Context) {
	if err := h.svc.CancelOrder(c.Request.Context(), h.pool, c.GetString("org_id"), c.Param("id")); err != nil {
		code := http.StatusInternalServerError
		if err == service.ErrOrderNotFound {
			code = http.StatusNotFound
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "canceled"}})
}

// RetireAsset POST /api/v1/assets/:id/retire
func (h *MaintenanceHandler) RetireAsset(c *gin.Context) {
	assetID := c.Param("id")

	var input struct {
		Reason string `json:"reason" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	orgID := c.GetString("org_id")
	userID := c.GetString("user_id")

	if err := h.svc.RetireAsset(c.Request.Context(), h.pool, orgID, assetID, input.Reason, userID); err != nil {
		code := http.StatusInternalServerError
		switch err {
		case service.ErrAssetRetired:
			code = http.StatusConflict
		case service.ErrActiveAssignmentExists:
			code = http.StatusConflict
		case service.ErrActiveOrderForRetire:
			code = http.StatusConflict
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"asset_id": assetID,
		"status":   "retired",
		"reason":   input.Reason,
	}})
}

// Ensure json is used
var _ = json.Marshal
