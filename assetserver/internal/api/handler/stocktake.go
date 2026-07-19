// Package handler — 盘点 Handler
// Phase G: 盘点管理
package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StocktakeHandler 盘点处理器
type StocktakeHandler struct {
	svc  *service.StocktakeService
	pool *pgxpool.Pool
}

// NewStocktakeHandler 创建 StocktakeHandler
func NewStocktakeHandler(svc *service.StocktakeService, pool *pgxpool.Pool) *StocktakeHandler {
	return &StocktakeHandler{svc: svc, pool: pool}
}

// planResponse JSON 响应格式化
type planResponse struct {
	ID              string  `json:"id"`
	PlanNo          string  `json:"plan_no"`
	OrgID           string  `json:"org_id"`
	Name            string  `json:"name"`
	ScopeLocationID *string `json:"scope_location_id,omitempty"`
	ScopeTypeID     *string `json:"scope_type_id,omitempty"`
	Status          string  `json:"status"`
	CreatedBy       string  `json:"created_by"`
	StartedAt       *string `json:"started_at,omitempty"`
	FinishedAt      *string `json:"finished_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

func toPlanResponse(p *repository.StocktakePlan) planResponse {
	r := planResponse{
		ID:              p.ID,
		PlanNo:          p.PlanNo,
		OrgID:           p.OrgID,
		Name:            p.Name,
		ScopeLocationID: p.ScopeLocationID,
		ScopeTypeID:     p.ScopeTypeID,
		Status:          p.Status,
		CreatedBy:       p.CreatedBy,
		CreatedAt:       p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:       p.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if p.StartedAt != nil {
		s := p.StartedAt.Format("2006-01-02T15:04:05Z")
		r.StartedAt = &s
	}
	if p.FinishedAt != nil {
		s := p.FinishedAt.Format("2006-01-02T15:04:05Z")
		r.FinishedAt = &s
	}
	return r
}

// itemResponse JSON 响应格式化
type itemResponse struct {
	ID                 string  `json:"id"`
	PlanID             string  `json:"plan_id"`
	AssetID            *string `json:"asset_id,omitempty"`
	ExpectedLocationID *string `json:"expected_location_id,omitempty"`
	ExpectedStatus     *string `json:"expected_status,omitempty"`
	Result             string  `json:"result"`
	ActualLocationID   *string `json:"actual_location_id,omitempty"`
	SurplusNote        *string `json:"surplus_note,omitempty"`
	CheckedBy          *string `json:"checked_by,omitempty"`
	CheckedAt          *string `json:"checked_at,omitempty"`
	Notes              *string `json:"notes,omitempty"`
}

func toItemResponse(si *repository.StocktakeItem) itemResponse {
	r := itemResponse{
		ID:                 si.ID,
		PlanID:             si.PlanID,
		AssetID:            si.AssetID,
		ExpectedLocationID: si.ExpectedLocationID,
		ExpectedStatus:     si.ExpectedStatus,
		Result:             si.Result,
		ActualLocationID:   si.ActualLocationID,
		SurplusNote:        si.SurplusNote,
		CheckedBy:          si.CheckedBy,
		Notes:              si.Notes,
	}
	if si.CheckedAt != nil {
		s := si.CheckedAt.Format("2006-01-02T15:04:05Z")
		r.CheckedAt = &s
	}
	return r
}

// CreatePlan POST /stocktakes
func (h *StocktakeHandler) CreatePlan(c *gin.Context) {
	var input struct {
		Name            string  `json:"name" binding:"required"`
		ScopeLocationID *string `json:"scope_location_id"`
		ScopeTypeID     *string `json:"scope_type_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	orgID := c.GetString("org_id")
	userID := c.GetString("user_id")

	plan, err := h.svc.CreatePlan(c.Request.Context(), h.pool, orgID, service.CreatePlanInput{
		Name:            input.Name,
		ScopeLocationID: input.ScopeLocationID,
		ScopeTypeID:     input.ScopeTypeID,
		CreatedBy:       userID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toPlanResponse(plan)})
}

// ListPlans GET /stocktakes
func (h *StocktakeHandler) ListPlans(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit > 200 {
		limit = 200
	}

	f := repository.StocktakeFilter{
		OrgID:  c.GetString("org_id"),
		Status: c.Query("status"),
		Cursor: c.Query("cursor"),
		Limit:  limit,
		Scope:  orgScopeFromCtx(c), // G9
	}

	rows, nextCursor, hasMore, err := h.svc.ListPlans(c.Request.Context(), h.pool, f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data := make([]planResponse, len(rows))
	for i, r := range rows {
		data[i] = toPlanResponse(&r)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": data,
		"pagination": gin.H{
			"next_cursor": nextCursor,
			"has_more":    hasMore,
		},
	})
}

// GetPlan GET /stocktakes/:id
func (h *StocktakeHandler) GetPlan(c *gin.Context) {
	plan, err := h.svc.GetPlan(c.Request.Context(), h.pool, c.Param("id"), c.GetString("org_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toPlanResponse(plan)})
}

// StartPlan POST /stocktakes/:id/start
func (h *StocktakeHandler) StartPlan(c *gin.Context) {
	if err := h.svc.StartPlan(c.Request.Context(), h.pool, c.GetString("org_id"), c.Param("id")); err != nil {
		code := http.StatusInternalServerError
		switch err {
		case service.ErrPlanNotFound:
			code = http.StatusNotFound
		case service.ErrPlanAlreadyStarted:
			code = http.StatusConflict
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "in_progress"}})
}

// UpdateItem PUT /stocktakes/:id/items/:itemId
func (h *StocktakeHandler) UpdateItem(c *gin.Context) {
	var input struct {
		Result           string  `json:"result" binding:"required"`
		ActualLocationID *string `json:"actual_location_id"`
		Notes            *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	err := h.svc.UpdateItem(c.Request.Context(), h.pool, c.GetString("org_id"),
		c.Param("id"), c.Param("itemId"), service.UpdateItemInput{
			Result:           input.Result,
			ActualLocationID: input.ActualLocationID,
			Notes:            input.Notes,
			CheckedBy:        userID,
		})
	if err != nil {
		code := http.StatusInternalServerError
		switch err {
		case service.ErrPlanNotFound:
			code = http.StatusNotFound
		case service.ErrPlanNotInProgress:
			code = http.StatusConflict
		case service.ErrItemNotFound:
			code = http.StatusNotFound
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// ListItems GET /stocktakes/:id/items
func (h *StocktakeHandler) ListItems(c *gin.Context) {
	result := c.Query("result")
	search := c.Query("search")
	items, err := h.svc.GetItems(c.Request.Context(), h.pool, c.Param("id"), result, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// AddSurplusItem POST /stocktakes/:id/items
func (h *StocktakeHandler) AddSurplusItem(c *gin.Context) {
	var input struct {
		SurplusNote      string  `json:"surplus_note" binding:"required"`
		ActualLocationID *string `json:"actual_location_id"`
		Notes            *string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID := c.GetString("user_id")

	item, err := h.svc.AddSurplusItem(c.Request.Context(), h.pool, c.GetString("org_id"),
		c.Param("id"), service.AddSurplusItemInput{
			SurplusNote:      input.SurplusNote,
			ActualLocationID: input.ActualLocationID,
			Notes:            input.Notes,
			CreatedBy:        userID,
		})
	if err != nil {
		code := http.StatusInternalServerError
		switch err {
		case service.ErrPlanNotFound:
			code = http.StatusNotFound
		case service.ErrPlanNotInProgress:
			code = http.StatusConflict
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": toItemResponse(item)})
}

// CompletePlan POST /stocktakes/:id/complete
func (h *StocktakeHandler) CompletePlan(c *gin.Context) {
	applyMoves, _ := strconv.ParseBool(c.DefaultQuery("apply_moves", "false"))

	if err := h.svc.CompletePlan(c.Request.Context(), h.pool, c.GetString("org_id"),
		c.Param("id"), applyMoves); err != nil {
		code := http.StatusInternalServerError
		switch err {
		case service.ErrPlanNotFound:
			code = http.StatusNotFound
		case service.ErrPlanNotInProgress:
			code = http.StatusConflict
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "completed"}})
}

// GetPlanReport GET /stocktakes/:id/report
func (h *StocktakeHandler) GetPlanReport(c *gin.Context) {
	format := c.DefaultQuery("format", "json")

	report, err := h.svc.GetPlanReport(c.Request.Context(), h.pool, c.GetString("org_id"), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
		return
	}

	if format == "csv" {
		h.renderReportCSV(c, report)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": report})
}

// renderReportCSV 输出 CSV 格式报告
func (h *StocktakeHandler) renderReportCSV(c *gin.Context, report *service.PlanReport) {
	var sb strings.Builder

	// Header
	sb.WriteString("盘点报告\n")
	sb.WriteString(fmt.Sprintf("计划编号,%s\n", report.Plan.PlanNo))
	sb.WriteString(fmt.Sprintf("计划名称,%s\n", report.Plan.Name))
	sb.WriteString(fmt.Sprintf("状态,%s\n", report.Plan.Status))
	sb.WriteString(fmt.Sprintf("总数,%d\n\n", report.Total))

	// Summary
	sb.WriteString("结果统计\n")
	sb.WriteString(fmt.Sprintf("待盘点,%d\n", report.Summary["pending"]))
	sb.WriteString(fmt.Sprintf("完好,%d\n", report.Summary["found"]))
	sb.WriteString(fmt.Sprintf("丢失,%d\n", report.Summary["missing"]))
	sb.WriteString(fmt.Sprintf("移位,%d\n", report.Summary["moved"]))
	sb.WriteString(fmt.Sprintf("盘盈,%d\n\n", report.Summary["surplus"]))

	// Moved details
	if len(report.Moved) > 0 {
		sb.WriteString("移位明细\n")
		sb.WriteString("资产ID,预期位置ID,实际位置ID,备注\n")
		for _, item := range report.Moved {
			assetID := ""
			if item.AssetID != nil {
				assetID = *item.AssetID
			}
			expLoc := ""
			if item.ExpectedLocationID != nil {
				expLoc = *item.ExpectedLocationID
			}
			actLoc := ""
			if item.ActualLocationID != nil {
				actLoc = *item.ActualLocationID
			}
			notes := ""
			if item.Notes != nil {
				notes = *item.Notes
			}
			sb.WriteString(fmt.Sprintf("%s,%s,%s,%s\n", assetID, expLoc, actLoc, notes))
		}
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=stocktake_report_%s.csv", report.Plan.ID))
	c.String(http.StatusOK, sb.String())
}
