// Package handler — 用户批量导入 Handler
// Wave 1 G2
package handler

import (
	"net/http"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserImportHandler 用户导入处理器
type UserImportHandler struct {
	svc  *service.UserImportService
	pool *pgxpool.Pool
}

// NewUserImportHandler 构造处理器
func NewUserImportHandler(svc *service.UserImportService, pool *pgxpool.Pool) *UserImportHandler {
	return &UserImportHandler{svc: svc, pool: pool}
}

// GetTemplate GET /api/v1/admin/users/import/template
// 返回 UTF-8 BOM CSV 模板 (admin+)
func (h *UserImportHandler) GetTemplate(c *gin.Context) {
	tmpl := h.svc.GetUserImportTemplate()
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=user_import_template.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", tmpl)
}

// ImportUsers POST /api/v1/admin/users/import
// ?dry_run=true -> 仅预览校验; 否则事务内导入 (admin+)
//
// 响应中 generated_password 字段为 CSV 未提供密码时系统生成的随机密码,
// 仅返回给调用 admin, 不写入审计/日志。前端应提示 admin 安全分发。
func (h *UserImportHandler) ImportUsers(c *gin.Context) {
	orgID := c.GetString("org_id")
	if orgID == "" {
		orgID = "00000000-0000-4000-a000-000000000001"
	}
	userID := c.GetString("user_id")
	dryRun := c.Query("dry_run") == "true"

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传 CSV 文件 (form field: file)"})
		return
	}
	defer file.Close()

	if dryRun {
		preview, err := h.svc.PreviewImport(c.Request.Context(), h.pool, file)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": preview})
		return
	}

	result, err := h.svc.ExecuteImport(c.Request.Context(), h.pool, userID, orgID, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
