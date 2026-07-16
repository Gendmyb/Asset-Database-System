// Package handler — Dashboard API (Phase 5)
package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// DashboardQuerier 仪表盘查询接口
type DashboardQuerier interface {
	TotalAssets(ctx context.Context, orgID string) (int64, error)
	AssetsByStatus(ctx context.Context, orgID string) (map[string]int64, error)
	AssetsByCategory(ctx context.Context, orgID string) (map[string]int64, error)
	AssetsByLifecycle(ctx context.Context, orgID string) (map[string]int64, error)
	AssetTrend(ctx context.Context, orgID string, days int) ([]TrendPoint, error)
	AgentStats(ctx context.Context, orgID string) (*AgentStats, error)
}

type TrendPoint struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type AgentStats struct {
	Online  int64 `json:"online"`
	Offline int64 `json:"offline"`
	Total   int64 `json:"total"`
}

// DashboardHandler Phase 5 仪表盘
type DashboardHandler struct {
	querier DashboardQuerier
}

func NewDashboardHandler(q DashboardQuerier) *DashboardHandler {
	return &DashboardHandler{querier: q}
}

// Overview GET /api/v1/dashboard/overview
func (h *DashboardHandler) Overview(c *gin.Context) {
	orgID := c.GetString("org_id")
	ctx := c.Request.Context()

	total, _ := h.querier.TotalAssets(ctx, orgID)
	byStatus, _ := h.querier.AssetsByStatus(ctx, orgID)
	byCategory, _ := h.querier.AssetsByCategory(ctx, orgID)
	byLifecycle, _ := h.querier.AssetsByLifecycle(ctx, orgID)
	trend, _ := h.querier.AssetTrend(ctx, orgID, 30)

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"total_assets":       total,
		"by_status":          byStatus,
		"by_category":        byCategory,
		"by_lifecycle":       byLifecycle,
		"trend_30d":          trend,
	}})
}

// AgentHealth GET /api/v1/dashboard/agents
func (h *DashboardHandler) AgentHealth(c *gin.Context) {
	orgID := c.GetString("org_id")
	stats, err := h.querier.AgentStats(c.Request.Context(), orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": stats})
}
