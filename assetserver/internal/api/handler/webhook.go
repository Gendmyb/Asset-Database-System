// Package handler — webhook management HTTP handlers
package handler

import (
	"net/http"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WebhookHandler webhook management HTTP handlers
type WebhookHandler struct {
	repo *repository.WebhookRepo
	pool *pgxpool.Pool
}

// NewWebhookHandler creates a new WebhookHandler
func NewWebhookHandler(repo *repository.WebhookRepo, pool *pgxpool.Pool) *WebhookHandler {
	return &WebhookHandler{repo: repo, pool: pool}
}

// ListEndpoints GET /admin/webhooks
func (h *WebhookHandler) ListEndpoints(c *gin.Context) {
	orgID := c.GetString("org_id")
	endpoints, err := h.repo.ListEndpoints(c.Request.Context(), h.pool, orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if endpoints == nil {
		endpoints = []repository.WebhookEndpointRow{}
	}
	c.JSON(http.StatusOK, gin.H{"data": endpoints})
}

// CreateEndpoint POST /admin/webhooks
func (h *WebhookHandler) CreateEndpoint(c *gin.Context) {
	var input struct {
		URL    string   `json:"url" binding:"required"`
		Secret string   `json:"secret"`
		Events []string `json:"events"`
		Active *bool    `json:"active"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	active := true
	if input.Active != nil {
		active = *input.Active
	}
	events := input.Events
	if len(events) == 0 {
		events = []string{"*"}
	}

	row := &repository.WebhookEndpointRow{
		OrgID:  c.GetString("org_id"),
		URL:    input.URL,
		Secret: []byte(input.Secret),
		Events: events,
		Active: active,
	}

	id, err := h.repo.CreateEndpoint(c.Request.Context(), h.pool, row)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{"id": id}})
}

// GetEndpoint GET /admin/webhooks/:id
func (h *WebhookHandler) GetEndpoint(c *gin.Context) {
	row, err := h.repo.GetEndpoint(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "webhook endpoint not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": row})
}

// UpdateEndpoint PUT /admin/webhooks/:id
func (h *WebhookHandler) UpdateEndpoint(c *gin.Context) {
	var input struct {
		URL    *string  `json:"url"`
		Events []string `json:"events"`
		Active *bool    `json:"active"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.repo.UpdateEndpoint(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"),
		input.URL, input.Events, input.Active); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// DeleteEndpoint DELETE /admin/webhooks/:id
func (h *WebhookHandler) DeleteEndpoint(c *gin.Context) {
	if err := h.repo.DeleteEndpoint(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// ListDeliveries GET /admin/webhooks/:id/deliveries
func (h *WebhookHandler) ListDeliveries(c *gin.Context) {
	deliveries, err := h.repo.ListDeliveries(c.Request.Context(), h.pool, c.Param("id"), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if deliveries == nil {
		deliveries = []repository.WebhookDeliveryRow{}
	}
	c.JSON(http.StatusOK, gin.H{"data": deliveries})
}
