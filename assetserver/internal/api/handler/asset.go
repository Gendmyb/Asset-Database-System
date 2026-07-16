// Package handler — 资产 API 处理器
// 对应架构文档 §6.4 资产路由
package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AssetHandler 资产 CRUD
type AssetHandler struct {
	repo AssetRepository
}

// AssetRepository 资产数据访问接口
type AssetRepository interface {
	List(orgID string, search string, typeID string, status string, cursor string, limit int) ([]Asset, string, bool, error)
	GetByID(id string) (*Asset, error)
	Create(asset *Asset) error
	Update(id string, updates map[string]interface{}, version int) (*Asset, error)
	SoftDelete(id string) error
	GetHistory(assetID string, limit int) ([]AuditLog, error)
}

// Asset 领域模型
type Asset struct {
	ID             string                 `json:"id"`
	AssetTag       string                 `json:"asset_tag"`
	Name           string                 `json:"name"`
	TypeID         string                 `json:"type_id"`
	OrgID          string                 `json:"org_id"`
	SerialNumber   *string                `json:"serial_number"`
	Manufacturer   *string                `json:"manufacturer"`
	Model          *string                `json:"model"`
	LifecycleState string                 `json:"lifecycle_state"`
	Status         string                 `json:"status"`
	Properties     map[string]interface{} `json:"properties"`
	Version        int                    `json:"version"`
	DeletedAt      *time.Time             `json:"deleted_at"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type AuditLog struct {
	ID        int64     `json:"id"`
	AssetID   string    `json:"asset_id"`
	Action    string    `json:"action"`
	PrevHash  string    `json:"prev_hash"`
	Hash      string    `json:"hash"`
	CreatedAt time.Time `json:"created_at"`
}

type PaginatedResponse struct {
	Data       []Asset `json:"data"`
	Pagination struct {
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	} `json:"pagination"`
	RequestID string `json:"request_id"`
}

func NewAssetHandler(repo AssetRepository) *AssetHandler {
	return &AssetHandler{repo: repo}
}

// ListAssets GET /api/v1/assets
func (h *AssetHandler) ListAssets(c *gin.Context) {
	orgID := c.GetString("org_id")
	search := c.Query("search")
	typeID := c.Query("type_id")
	status := c.Query("status")
	cursor := c.Query("cursor")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit > 200 {
		limit = 200
	}

	assets, nextCursor, hasMore, err := h.repo.List(orgID, search, typeID, status, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": err.Error()}})
		return
	}

	resp := PaginatedResponse{Data: assets}
	resp.Pagination.HasMore = hasMore
	if nextCursor != "" {
		resp.Pagination.NextCursor = &nextCursor
	}
	reqID, _ := c.Get("request_id")
	resp.RequestID = reqID.(string)

	c.JSON(http.StatusOK, resp)
}

// CreateAsset POST /api/v1/assets
func (h *AssetHandler) CreateAsset(c *gin.Context) {
	var input struct {
		AssetTag       string                 `json:"asset_tag" binding:"required"`
		Name           string                 `json:"name" binding:"required"`
		TypeID         string                 `json:"type_id" binding:"required"`
		SerialNumber   *string                `json:"serial_number"`
		Manufacturer   *string                `json:"manufacturer"`
		Model          *string                `json:"model"`
		LifecycleState string                 `json:"lifecycle_state"`
		Status         string                 `json:"status"`
		Properties     map[string]interface{} `json:"properties"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": err.Error()}})
		return
	}

	if input.LifecycleState == "" {
		input.LifecycleState = "procurement"
	}
	if input.Status == "" {
		input.Status = "available"
	}

	now := time.Now()
	asset := &Asset{
		ID:             uuid.New().String(),
		AssetTag:       input.AssetTag,
		Name:           input.Name,
		TypeID:         input.TypeID,
		OrgID:          c.GetString("org_id"),
		SerialNumber:   input.SerialNumber,
		Manufacturer:   input.Manufacturer,
		Model:          input.Model,
		LifecycleState: input.LifecycleState,
		Status:         input.Status,
		Properties:     input.Properties,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := h.repo.Create(asset); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": asset})
}

// GetAsset GET /api/v1/assets/:id
func (h *AssetHandler) GetAsset(c *gin.Context) {
	asset, err := h.repo.GetByID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "Asset not found"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": asset})
}

// UpdateAsset PUT /api/v1/assets/:id (乐观锁)
func (h *AssetHandler) UpdateAsset(c *gin.Context) {
	versionStr := c.GetHeader("If-Match")
	if versionStr == "" {
		c.JSON(http.StatusPreconditionRequired, gin.H{
			"error": gin.H{"code": "PRECONDITION_REQUIRED", "message": "If-Match header required"},
		})
		return
	}
	version, err := strconv.Atoi(versionStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "INVALID_HEADER", "message": "Invalid If-Match"}})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"code": "VALIDATION", "message": err.Error()}})
		return
	}

	asset, err := h.repo.Update(c.Param("id"), updates, version)
	if err != nil {
		if err.Error() == "version conflict" {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"code": "VERSION_CONFLICT", "message": "Version conflict"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": asset})
}

// DeleteAsset DELETE /api/v1/assets/:id (软删除)
func (h *AssetHandler) DeleteAsset(c *gin.Context) {
	if err := h.repo.SoftDelete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"code": "NOT_FOUND", "message": "Asset not found"}})
		return
	}
	c.Status(http.StatusNoContent)
}

// GetHistory GET /api/v1/assets/:id/history
func (h *AssetHandler) GetHistory(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	logs, err := h.repo.GetHistory(c.Param("id"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "INTERNAL", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": logs})
}

// EncodeCursor 编码游标
func EncodeCursor(updatedAt time.Time, id string) string {
	data, _ := json.Marshal(map[string]interface{}{"u": updatedAt.Format(time.RFC3339Nano), "i": id})
	return base64.URLEncoding.EncodeToString(data)
}
