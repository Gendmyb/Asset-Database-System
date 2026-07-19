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
// Wave 1 G5: 支持 format=xlsx 切换 Excel 导出
func (h *ReportHandler) ExportDepreciation(c *gin.Context) {
	orgID := c.GetString("org_id")
	asOf := c.Query("as_of")
	format := c.DefaultQuery("format", "csv")

	if format == "xlsx" {
		data, err := h.exportSvc.ExportDepreciationXLSX(c.Request.Context(), h.pool, orgID, asOf)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		filename := "depreciation_export_" + time.Now().Format("20060102_150405") + ".xlsx"
		c.Header("Content-Disposition", "attachment; filename="+filename)
		c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
		return
	}

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
	format := c.DefaultQuery("format", "csv")

	if format == "xlsx" {
		data, err := h.exportSvc.ExportStocktakeReportXLSX(c.Request.Context(), h.pool, orgID, planID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		filename := "stocktake_report_" + sanitizeFilename(planID) + ".xlsx"
		c.Header("Content-Disposition", "attachment; filename="+filename)
		c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
		return
	}

	var buf bytes.Buffer
	if err := h.exportSvc.ExportStocktakeReportCSV(c.Request.Context(), h.pool, orgID, planID, &buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := "stocktake_report_" + sanitizeFilename(planID) + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "text/csv; charset=utf-8", buf.Bytes())
}

// ExportAssetsXLSX GET /api/v1/reports/assets.xlsx
// Wave 1 G5: Excel 导出 (与 CSV 端点并列)
func (h *ReportHandler) ExportAssetsXLSX(c *gin.Context) {
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

	data, err := h.exportSvc.ExportAssetsXLSX(c.Request.Context(), h.pool, orgID, f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := "assets_export_" + time.Now().Format("20060102_150405") + ".xlsx"
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

// sanitizeFilename 对用于 Content-Disposition filename 的标识符做白名单过滤:
// 仅保留 [a-zA-Z0-9-], 剔除其他字符 (含路径分隔符 / \ 、空格等), 防止头注入与路径穿越。
// 空结果回退为 "unknown"。
func sanitizeFilename(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-':
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}
