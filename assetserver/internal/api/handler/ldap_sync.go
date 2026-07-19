// Package handler — LDAP 手动同步 Handler
// Wave 1 G1
package handler

import (
	"net/http"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/auth/ldap"
	"github.com/gin-gonic/gin"
)

// LDAPSyncHandler LDAP 同步处理器
type LDAPSyncHandler struct {
	sync *ldap.SyncService
}

// NewLDAPSyncHandler 构造处理器
func NewLDAPSyncHandler(sync *ldap.SyncService) *LDAPSyncHandler {
	return &LDAPSyncHandler{sync: sync}
}

// Sync POST /api/v1/admin/ldap/sync (admin+)
// 手动触发一次单向 AD -> DB 同步; 结果含统计摘要, 已写入审计日志。
func (h *LDAPSyncHandler) Sync(c *gin.Context) {
	if h.sync == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "LDAP 未启用"})
		return
	}
	actorID := c.GetString("user_id")
	orgID := c.GetString("org_id")
	if orgID == "" {
		orgID = "00000000-0000-4000-a000-000000000001"
	}
	result, err := h.sync.RunSyncOnce(c.Request.Context(), actorID, orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
