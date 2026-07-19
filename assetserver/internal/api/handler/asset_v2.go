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
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// isDuplicateKeyErr 判断是否为唯一约束冲突 (SQLSTATE 23505)
func isDuplicateKeyErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "23505") ||
		strings.Contains(msg, "unique constraint")
}

// AssetV2Handler Phase 2 资产处理器 (集成真实 Repository)
type AssetV2Handler struct {
	repo         *repository.AssetRepo
	settingsRepo *repository.SettingsRepo
	svc          *service.AssetService
	pool         *pgxpool.Pool
}

func NewAssetV2Handler(repo *repository.AssetRepo, settingsRepo *repository.SettingsRepo, pool *pgxpool.Pool) *AssetV2Handler {
	return &AssetV2Handler{
		repo:         repo,
		settingsRepo: settingsRepo,
		svc:          service.NewAssetService(repo, settingsRepo),
		pool:         pool,
	}
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
	// Phase E: 采购/折旧字段
	PurchasePrice      *float64   `json:"purchase_price,omitempty"`
	PurchaseDate       *time.Time `json:"purchase_date,omitempty"`
	Supplier           *string    `json:"supplier,omitempty"`
	WarrantyUntil      *time.Time `json:"warranty_until,omitempty"`
	DepreciationMethod string     `json:"depreciation_method"`
	UsefulLifeMonths   *int       `json:"useful_life_months,omitempty"`
	SalvageValue       float64    `json:"salvage_value"`
	ManagedBy          *string    `json:"managed_by,omitempty"`
	RetiredAt          *time.Time `json:"retired_at,omitempty"`
	RetireReason       *string    `json:"retire_reason,omitempty"`
}

func rowToResponse(r *repository.AssetRow) AssetResponse {
	return AssetResponse{
		ID: r.ID, AssetTag: r.AssetTag, Name: r.Name,
		TypeID: r.TypeID, OrgID: r.OrgID,
		SerialNumber: r.SerialNumber, Manufacturer: r.Manufacturer, Model: r.Model,
		LifecycleState: r.LifecycleState, Status: r.Status,
		Properties: r.Properties, Version: r.Version,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		// Phase E
		PurchasePrice:      r.PurchasePrice,
		PurchaseDate:       r.PurchaseDate,
		Supplier:           r.Supplier,
		WarrantyUntil:      r.WarrantyUntil,
		DepreciationMethod: r.DepreciationMethod,
		UsefulLifeMonths:   r.UsefulLifeMonths,
		SalvageValue:       r.SalvageValue,
		ManagedBy:          r.ManagedBy,
		RetiredAt:          r.RetiredAt,
		RetireReason:       r.RetireReason,
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

	rows, nextCursor, hasMore, err := h.repo.List(c.Request.Context(), h.pool, f)
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
	row, err := h.repo.GetByID(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"))
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
		// Phase E: 采购字段
		PurchasePrice      *float64 `json:"purchase_price"`
		PurchaseDate       *string  `json:"purchase_date"`
		Supplier           *string  `json:"supplier"`
		WarrantyUntil      *string  `json:"warranty_until"`
		DepreciationMethod string   `json:"depreciation_method"`
		UsefulLifeMonths   *int     `json:"useful_life_months"`
		SalvageValue       *float64 `json:"salvage_value"`
		ManagedBy          *string  `json:"managed_by"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	orgID := c.GetString("org_id")
	userID := c.GetString("user_id")

	// Parse date strings
	var purchaseDate *time.Time
	if input.PurchaseDate != nil && *input.PurchaseDate != "" {
		parsed, err := time.Parse("2006-01-02", *input.PurchaseDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid purchase_date format, expected YYYY-MM-DD"})
			return
		}
		purchaseDate = &parsed
	}
	var warrantyUntil *time.Time
	if input.WarrantyUntil != nil && *input.WarrantyUntil != "" {
		parsed, err := time.Parse("2006-01-02", *input.WarrantyUntil)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid warranty_until format, expected YYYY-MM-DD"})
			return
		}
		warrantyUntil = &parsed
	}

	depreciationMethod := input.DepreciationMethod
	if depreciationMethod == "" {
		depreciationMethod = "none"
	}
	salvageValue := 0.0
	if input.SalvageValue != nil {
		salvageValue = *input.SalvageValue
	}

	svcInput := service.CreateAssetInput{
		Name:           input.Name,
		AssetTag:       input.AssetTag,
		TypeID:         input.TypeID,
		OrgID:          orgID,
		SerialNumber:   input.SerialNumber,
		Manufacturer:   input.Manufacturer,
		Model:          input.Model,
		LifecycleState: input.LifecycleState,
		Properties:     input.Properties,
		ActorID:        userID,
		// Phase E
		PurchasePrice:      input.PurchasePrice,
		PurchaseDate:       purchaseDate,
		Supplier:           input.Supplier,
		WarrantyUntil:      warrantyUntil,
		DepreciationMethod: depreciationMethod,
		UsefulLifeMonths:   input.UsefulLifeMonths,
		SalvageValue:       salvageValue,
		ManagedBy:          input.ManagedBy,
	}

	row, err := h.svc.CreateAsset(c.Request.Context(), h.pool, svcInput)
	if err != nil {
		if isDuplicateKeyErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "资产编号已存在，请更换或留空自动生成"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": rowToResponse(row)})
}

// CreateAssetBatch POST /api/v1/assets/batch
// Phase E: 批量创建资产
func (h *AssetV2Handler) CreateAssetBatch(c *gin.Context) {
	var input struct {
		Name           string          `json:"name" binding:"required"`
		TypeID         string          `json:"type_id" binding:"required"`
		Count          int             `json:"count" binding:"required"`
		SerialNumber   *string         `json:"serial_number"`
		Manufacturer   *string         `json:"manufacturer"`
		Model          *string         `json:"model"`
		LifecycleState string          `json:"lifecycle_state"`
		Properties     json.RawMessage `json:"properties"`
		// Phase E: 采购字段
		PurchasePrice      *float64 `json:"purchase_price"`
		PurchaseDate       *string  `json:"purchase_date"`
		Supplier           *string  `json:"supplier"`
		WarrantyUntil      *string  `json:"warranty_until"`
		DepreciationMethod string   `json:"depreciation_method"`
		UsefulLifeMonths   *int     `json:"useful_life_months"`
		SalvageValue       *float64 `json:"salvage_value"`
		ManagedBy          *string  `json:"managed_by"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.Count <= 0 || input.Count > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "count must be between 1 and 100"})
		return
	}

	orgID := c.GetString("org_id")
	userID := c.GetString("user_id")

	// Parse date strings
	var purchaseDate *time.Time
	if input.PurchaseDate != nil && *input.PurchaseDate != "" {
		parsed, err := time.Parse("2006-01-02", *input.PurchaseDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid purchase_date format"})
			return
		}
		purchaseDate = &parsed
	}
	var warrantyUntil *time.Time
	if input.WarrantyUntil != nil && *input.WarrantyUntil != "" {
		parsed, err := time.Parse("2006-01-02", *input.WarrantyUntil)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid warranty_until format"})
			return
		}
		warrantyUntil = &parsed
	}

	depreciationMethod := input.DepreciationMethod
	if depreciationMethod == "" {
		depreciationMethod = "none"
	}
	salvageValue := 0.0
	if input.SalvageValue != nil {
		salvageValue = *input.SalvageValue
	}

	svcInput := service.CreateAssetInput{
		Name:           input.Name,
		TypeID:         input.TypeID,
		OrgID:          orgID,
		SerialNumber:   input.SerialNumber,
		Manufacturer:   input.Manufacturer,
		Model:          input.Model,
		LifecycleState: input.LifecycleState,
		Properties:     input.Properties,
		ActorID:        userID,
		PurchasePrice:      input.PurchasePrice,
		PurchaseDate:       purchaseDate,
		Supplier:           input.Supplier,
		WarrantyUntil:      warrantyUntil,
		DepreciationMethod: depreciationMethod,
		UsefulLifeMonths:   input.UsefulLifeMonths,
		SalvageValue:       salvageValue,
		ManagedBy:          input.ManagedBy,
	}

	assets, err := h.svc.CreateAssetBatch(c.Request.Context(), h.pool, svcInput, input.Count)
	if err != nil {
		if isDuplicateKeyErr(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "资产编号冲突，请重试"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data := make([]AssetResponse, len(assets))
	for i, a := range assets {
		data[i] = rowToResponse(a)
	}

	c.JSON(http.StatusCreated, gin.H{
		"data":  gin.H{"assets": data, "count": len(data)},
	})
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
		current, err := h.repo.GetByID(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"))
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

	row, err := h.repo.UpdateWithRetry(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"), updates, version)
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
	if err := h.repo.SoftDelete(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id")); err != nil {
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
	row, err := h.repo.LockForUpdate(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"))
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

	updated, err := h.repo.UpdateWithRetry(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"),
		map[string]interface{}{"lifecycle_state": input.To}, row.Version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rowToResponse(updated)})
}
