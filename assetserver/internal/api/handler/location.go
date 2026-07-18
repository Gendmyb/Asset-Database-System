// Package handler — Location CRUD (PG 生产模式)
// Phase B: 生产模式走 PG repo
package handler

import (
	"net/http"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
)

// Location 位置模型 (API 响应)
type Location struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      *string   `json:"code"`
	ParentID  *string   `json:"parent_id"`
	OrgID     string    `json:"org_id"`
	Notes     *string   `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LocationHandler 位置管理
type LocationHandler struct {
	repo *repository.LocationRepo
	pool repository.DBTX
}

func NewLocationHandler(repo *repository.LocationRepo, pool repository.DBTX) *LocationHandler {
	return &LocationHandler{repo: repo, pool: pool}
}

// List GET /api/v1/locations
func (h *LocationHandler) List(c *gin.Context) {
	rows, err := h.repo.ListByOrg(c.Request.Context(), h.pool, c.GetString("org_id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data := make([]Location, len(rows))
	for i, r := range rows {
		data[i] = rowToLocation(&r)
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// Create POST /api/v1/locations
func (h *LocationHandler) Create(c *gin.Context) {
	var input struct {
		Name     string  `json:"name" binding:"required"`
		Code     *string `json:"code"`
		ParentID *string `json:"parent_id"`
		Notes    *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	loc, err := h.repo.Create(c.Request.Context(), h.pool, repository.CreateLocationInput{
		Name:     input.Name,
		Code:     input.Code,
		OrgID:    c.GetString("org_id"),
		ParentID: input.ParentID,
		Notes:    input.Notes,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": rowToLocation(loc)})
}

// Get GET /api/v1/locations/:id
func (h *LocationHandler) Get(c *gin.Context) {
	loc, err := h.repo.GetByID(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rowToLocation(loc)})
}

// Update PUT /api/v1/locations/:id
func (h *LocationHandler) Update(c *gin.Context) {
	var input struct {
		Name     *string `json:"name"`
		Code     *string `json:"code"`
		ParentID *string `json:"parent_id"`
		Notes    *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.repo.Update(c.Request.Context(), h.pool, c.Param("id"),
		c.GetString("org_id"), input.Name, input.Code, input.Notes, input.ParentID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// Delete DELETE /api/v1/locations/:id
func (h *LocationHandler) Delete(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func rowToLocation(r *repository.LocationRow) Location {
	return Location{
		ID:        r.ID,
		Name:      r.Name,
		Code:      r.Code,
		ParentID:  r.ParentID,
		OrgID:     r.OrgID,
		Notes:     r.Notes,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}
