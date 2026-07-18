// Package middleware — RBAC 角色验证中间件
// Phase C: 真实认证与 RBAC
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RoleLevel 角色等级映射
var roleLevel = map[string]int{
	"viewer":      0,
	"manager":     1,
	"admin":       2,
	"super_admin": 3,
}

// RequireRole 要求最低角色等级
// 必须在 Auth 中间件之后使用 (依赖 c.GetString("role"))
func RequireRole(min string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role == "" {
			role = "viewer"
		}
		if roleLevel[role] < roleLevel[min] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{
				"code":    "FORBIDDEN",
				"message": "权限不足，需要 " + min + " 或更高角色",
			}})
			return
		}
		c.Next()
	}
}
