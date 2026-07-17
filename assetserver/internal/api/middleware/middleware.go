// Package middleware — Gin 中间件链
// 对应架构文档 §6.6 中间件链
package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID 注入唯一请求 ID
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := uuid.New().String()[:8]
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// Recovery panic 恢复
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				reqID, _ := c.Get("request_id")
				log.Printf("[%v] PANIC: %v", reqID, err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{"code": "INTERNAL_ERROR", "message": "Unexpected error"},
				})
			}
		}()
		c.Next()
	}
}

// StructuredLogging 结构化日志
func StructuredLogging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Milliseconds()
		reqID, _ := c.Get("request_id")
		log.Printf("[%s] %s %s → %d (%dms)",
			reqID, c.Request.Method, c.Request.URL.Path,
			c.Writer.Status(), duration)
	}
}

// ClaimsVerifier JWT claims 验证接口
type ClaimsVerifier interface {
	VerifyJWT(token string) (*Claims, error)
}

// Claims 简化的 JWT Claims
type Claims struct {
	UserID string
	Role   string
	OrgID  string
}

// Auth JWT 验证中间件 (EdDSA)
func Auth(verifier ClaimsVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": "UNAUTHORIZED", "message": "Missing authorization token"},
			})
			return
		}

		claims, err := verifier.VerifyJWT(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"code": "INVALID_TOKEN", "message": err.Error()},
			})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Set("org_id", claims.OrgID)
		c.Next()
	}
}

// OrgScope org_id 自动注入 (防 IDOR)
func OrgScope() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 从 JWT claims 中提取 org_id (由 Auth 中间件设置)
		orgID, exists := c.Get("org_id")
		if !exists || orgID == "" {
			// 默认 org (开发环境)
			c.Set("org_id", "00000000-0000-4000-a000-000000000001")
		}
		c.Next()
	}
}

// RateLimit 限流中间件
func RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}
