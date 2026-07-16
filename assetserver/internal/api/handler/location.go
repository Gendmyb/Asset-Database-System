// Package handler — Location CRUD (Phase 5)
package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Location 位置模型
type Location struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id"`
	OrgID     string    `json:"org_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LocationHandler 位置管理
type LocationHandler struct {
	store *LocationStore
}

func NewLocationHandler() *LocationHandler {
	return &LocationHandler{store: NewLocationStore()}
}

// List GET /api/v1/locations
func (h *LocationHandler) List(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": h.store.List(c.GetString("org_id"))})
}

// Create POST /api/v1/locations
func (h *LocationHandler) Create(c *gin.Context) {
	var input struct {
		Name     string  `json:"name" binding:"required"`
		ParentID *string `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	loc := &Location{
		ID:        uuid.New().String(),
		Name:      input.Name,
		ParentID:  input.ParentID,
		OrgID:     c.GetString("org_id"),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	h.store.Add(loc)
	c.JSON(http.StatusCreated, gin.H{"data": loc})
}

// LocationStore 内存存储 (TODO: PostgreSQL)
type LocationStore struct {
	items []*Location
}

func NewLocationStore() *LocationStore {
	return &LocationStore{items: make([]*Location, 0)}
}

func (s *LocationStore) List(orgID string) []*Location {
	var result []*Location
	for _, l := range s.items {
		if l.OrgID == orgID {
			result = append(result, l)
		}
	}
	if result == nil {
		result = []*Location{}
	}
	return result
}

func (s *LocationStore) Add(l *Location) {
	s.items = append(s.items, l)
}
