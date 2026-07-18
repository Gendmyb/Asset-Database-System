// Package handler — 报表/导出 Handler
// Phase H Step 5: 报表 + CSV 导出
package handler

import (
	"bytes"
	"net/http"
	"strconv"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReportHandler 报表/导出处理器
type ReportHandler struct {
	reportSvc *service.ReportService
	depSvc    *service.DepreciationService
	exportSvc *service.ExportService
	pool      *pgxpool.Pool
}

func NewReportHandler(reportSvc *service.ReportService, depSvc *service.DepreciationService, exportSvc *service.ExportService, pool *pgxpool.Pool) *ReportHandler {
	return &ReportHandler{reportSvc: reportSvc, depSvc: depSvc, exportSvc: exportSvc, pool: pool}
}

// GetSummary GET /api/v1/reports/summary
func (h *ReportHandler) GetSummary(c *gin.Context) {
	orgID := c.GetString("org_id")
	asOf := c.Query("as_of")

	summary, err := h.reportSvc.GetSummary(c.Request.Context(), h.pool, orgID, asOf)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": summary})
}

// GetDepreciation GET /api/v1/reports/depreciation
func (h *ReportHandler) GetDepreciation(c *gin.Context) {
	orgID := c.GetString("org_id")
	asOf := c.Query("as_of")

	rows, cursor, hasMore, err := h.depSvc.GetDepreciation(c.Request.Context(), h.pool, orgID, asOf)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []service.DepreciationRow{}
	}
	c.JSON(http.StatusOK, gin.H{
		"data": rows,
		"pagination": gin.H{
			"next_cursor": cursor,
			"has_more":    hasMore,
		},
	})
}

// GetMaintenanceCost GET /api/v1/reports/maintenance-cost
func (h *ReportHandler) GetMaintenanceCost(c *gin.Context) {
	orgID := c.GetString("org_id")
	from := c.Query("from")
	to := c.Query("to")

	rows, err := h.reportSvc.GetMaintenanceCost(c.Request.Context(), h.pool, orgID, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []service.CostRow{}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// GetAssignmentsDue GET /api/v1/reports/assignments-due
func (h *ReportHandler) GetAssignmentsDue(c *gin.Context) {
	orgID := c.GetString("org_id")
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))

	rows, err := h.reportSvc.GetAssignmentsDue(c.Request.Context(), h.pool, orgID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if rows == nil {
		rows = []service.DueRow{}
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

// ExportAssets GET /api/v1/assets/export
func (h *ReportHandler) ExportAssets(c *gin.Context) {
	orgID := c.GetString("org_id")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "2000"))
	if limit > 2000 {
		limit = 2000
	}
	if limit <= 0 {
		limit = 2000
	}

	f := repository.AssetFilter{
		OrgID:        orgID,
		Search:       c.Query("search"),
		TypeID:       c.Query("type_id"),
		Status:       c.Query("status"),
		Lifecycle:    c.Query("lifecycle"),
		Manufacturer: c.Query("manufacturer"),
		Limit:        limit,
	}

	var buf bytes.Buffer
	if err := h.exportSvc.ExportAssetsCSV(c.Request.Context(), h.pool, orgID, &buf, f); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := "assets_export_" + time.Now().Format("20060102_150405") + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportDepreciation GET /api/v1/reports/depreciation/export
func (h *ReportHandler) ExportDepreciation(c *gin.Context) {
	orgID := c.GetString("org_id")
	asOf := c.Query("as_of")

	var buf bytes.Buffer
	if err := h.exportSvc.ExportDepreciationCSV(c.Request.Context(), h.pool, orgID, &buf, asOf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := "depreciation_export_" + time.Now().Format("20060102_150405") + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportStocktakeReport GET /api/v1/stocktakes/:id/report/export
func (h *ReportHandler) ExportStocktakeReport(c *gin.Context) {
	orgID := c.GetString("org_id")
	planID := c.Param("id")

	var buf bytes.Buffer
	if err := h.exportSvc.ExportStocktakeReportCSV(c.Request.Context(), h.pool, orgID, planID, &buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := "stocktake_report_" + planID[:8] + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}
