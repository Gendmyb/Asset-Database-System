// Package handler — 导入 Handler
// Phase H Step 5: CSV 导入
package handler

import (
	"net/http"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ImportHandler 导入处理器
type ImportHandler struct {
	importSvc *service.ImportService
	pool      *pgxpool.Pool
}

func NewImportHandler(importSvc *service.ImportService, pool *pgxpool.Pool) *ImportHandler {
	return &ImportHandler{importSvc: importSvc, pool: pool}
}

// GetTemplate GET /api/v1/assets/import/template
func (h *ImportHandler) GetTemplate(c *gin.Context) {
	template := h.importSvc.GetImportTemplate()

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=asset_import_template.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", template)
}

// roleLevel 角色等级 (复用 middleware 逻辑)
var roleLevel = map[string]int{
	"viewer":      0,
	"manager":     1,
	"admin":       2,
	"super_admin": 3,
}

// ImportAssets POST /api/v1/assets/import
// ?dry_run=true -> PreviewImport (manager+)
// without dry_run  -> ExecuteImport (admin+)
func (h *ImportHandler) ImportAssets(c *gin.Context) {
	orgID := c.GetString("org_id")
	userID := c.GetString("user_id")
	role := c.GetString("role")

	dryRun := c.Query("dry_run") == "true"

	// 执行导入需要 admin+
	if !dryRun && roleLevel[role] < roleLevel["admin"] {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{
			"code":    "FORBIDDEN",
			"message": "执行导入需要 admin 或更高角色",
		}})
		return
	}

	// 读取上传文件
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请上传 CSV 文件 (form field: file)"})
		return
	}
	defer file.Close()

	if dryRun {
		preview, err := h.importSvc.PreviewImport(c.Request.Context(), h.pool, orgID, file)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": preview})
		return
	}

	result, err := h.importSvc.ExecuteImport(c.Request.Context(), h.pool, orgID, userID, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
