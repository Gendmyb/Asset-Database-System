// Package handler — API 处理器
// 对应架构文档 §6.4 API 路由表
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查
type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Healthz 存活探针 — 进程存活即 200
func (h *HealthHandler) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readyz 就绪探针 — PG + Redis 可达
func (h *HealthHandler) Readyz(dbCheck func() error, redisCheck func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if dbCheck != nil {
			if err := dbCheck(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status": "not ready",
					"reason": "database unreachable",
				})
				return
			}
		}
		// Redis 检查可选 (允许降级)
		if redisCheck != nil {
			if err := redisCheck(); err != nil {
				c.JSON(http.StatusOK, gin.H{
					"status": "degraded",
					"reason": "redis unreachable",
				})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}
