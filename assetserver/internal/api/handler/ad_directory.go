// Package handler — AD 目录集成管理 API (Wave 3 T7)
// 端点:
//   GET    /admin/ad-groups        — 列出所有组映射
//   POST   /admin/ad-groups        — 新建组映射
//   PUT    /admin/ad-groups/:id    — 更新组映射
//   DELETE /admin/ad-groups/:id    — 删除组映射
//   GET    /admin/ldap/status      — LDAP 连通状态与上次同步结果
//   POST   /admin/users/:id/link-ad — 将本地用户链接到 AD 账号
//   DELETE /admin/users/:id/link-ad — 解除 AD 链接 (转为本地用户)
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DirectoryHandler AD 目录集成管理 handler
type DirectoryHandler struct {
	groupRepo *repository.ADGroupRepo
	userRepo  *repository.UserRepo
	pool      *pgxpool.Pool
	// ldapStatus 回调查询 LDAP 状态 (注入, 避免循环依赖)
	ldapStatus func(ctx context.Context) map[string]interface{}
}

// NewDirectoryHandler 构造 Directory handler
func NewDirectoryHandler(pool *pgxpool.Pool) *DirectoryHandler {
	return &DirectoryHandler{
		groupRepo: repository.NewADGroupRepo(),
		userRepo:  repository.NewUserRepo(),
		pool:      pool,
	}
}

// SetLDAPStatusCallback 注入 LDAP 状态查询回调
func (h *DirectoryHandler) SetLDAPStatusCallback(cb func(ctx context.Context) map[string]interface{}) {
	h.ldapStatus = cb
}

// === 组映射 CRUD ===

// ListGroupMappings GET /admin/ad-groups
func (h *DirectoryHandler) ListGroupMappings(c *gin.Context) {
	mappings, err := h.groupRepo.ListAll(c.Request.Context(), h.pool)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if mappings == nil {
		mappings = []repository.GroupMapping{}
	}
	c.JSON(http.StatusOK, gin.H{"data": mappings})
}

// CreateGroupMapping POST /admin/ad-groups
func (h *DirectoryHandler) CreateGroupMapping(c *gin.Context) {
	var input struct {
		GroupDN   string `json:"group_dn" binding:"required"`
		GroupName string `json:"group_name"`
		Role      string `json:"role" binding:"required"`
		DataScope string `json:"data_scope"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	validRoles := map[string]bool{"super_admin": true, "admin": true, "manager": true, "viewer": true}
	if !validRoles[input.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效角色: " + input.Role})
		return
	}
	if input.DataScope == "" {
		input.DataScope = "inherit"
	}
	if input.DataScope != "inherit" && input.DataScope != "self" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data_scope 必须是 inherit 或 self"})
		return
	}
	m, err := h.groupRepo.Create(c.Request.Context(), h.pool, input.GroupDN, input.GroupName, input.Role, input.DataScope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": m})
}

// UpdateGroupMapping PUT /admin/ad-groups/:id
func (h *DirectoryHandler) UpdateGroupMapping(c *gin.Context) {
	var input struct {
		Role        *string `json:"role"`
		DataScope   *string `json:"data_scope"`
		SyncEnabled *bool   `json:"sync_enabled"`
		GroupName   *string `json:"group_name"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Role != nil {
		validRoles := map[string]bool{"super_admin": true, "admin": true, "manager": true, "viewer": true}
		if !validRoles[*input.Role] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "无效角色: " + *input.Role})
			return
		}
	}
	if input.DataScope != nil && *input.DataScope != "inherit" && *input.DataScope != "self" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "data_scope 必须是 inherit 或 self"})
		return
	}
	m, err := h.groupRepo.Update(c.Request.Context(), h.pool, c.Param("id"),
		input.Role, input.DataScope, input.SyncEnabled, input.GroupName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": m})
}

// DeleteGroupMapping DELETE /admin/ad-groups/:id
func (h *DirectoryHandler) DeleteGroupMapping(c *gin.Context) {
	if err := h.groupRepo.Delete(c.Request.Context(), h.pool, c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// === LDAP 状态 ===

// GetLDAPStatus GET /admin/ldap/status
func (h *DirectoryHandler) GetLDAPStatus(c *gin.Context) {
	if h.ldapStatus == nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"enabled": false}})
		return
	}
	status := h.ldapStatus(c.Request.Context())
	c.JSON(http.StatusOK, gin.H{"data": status})
}

// === 用户 AD 链接 ===

// LinkAD POST /admin/users/:id/link-ad — 将本地用户链接到 AD 账号
func (h *DirectoryHandler) LinkAD(c *gin.Context) {
	var input struct {
		ExternalID string `json:"external_id" binding:"required"`
		DN         string `json:"dn"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID := c.Param("id")
	// 更新 source 为 'ldap' 并设置 external_id/dn (保留 id/角色/领用历史)
	tag, err := h.pool.Exec(c.Request.Context(),
		`UPDATE assets.users SET
		   source = 'ldap', external_id = $2, dn = COALESCE(NULLIF($3,''), dn),
		   manual_override = true, updated_at = now()
		 WHERE id = $1 AND source = 'local'`,
		userID, input.ExternalID, input.DN,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在或已是 LDAP 用户"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// UnlinkAD DELETE /admin/users/:id/link-ad — 解除 AD 链接 (转为本地用户)
func (h *DirectoryHandler) UnlinkAD(c *gin.Context) {
	userID := c.Param("id")
	tag, err := h.pool.Exec(c.Request.Context(),
		`UPDATE assets.users SET
		   source = 'local', external_id = NULL, dn = NULL,
		   manual_override = false, updated_at = now()
		 WHERE id = $1 AND source = 'ldap'`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在或不是 LDAP 用户"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": "ok"})
}

// ensure time import is used
var _ = time.Now
