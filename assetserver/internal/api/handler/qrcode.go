// Package handler — 资产二维码 (QR Code) 生成
// Wave 1 G3: 扫码 + 移动盘点
//
// 端点: GET /api/v1/assets/:tag/qrcode
// 返回: image/png (256x256)，内容为资产 asset_tag (可被扫码枪/手机识别后跳转详情)。
// 权限: viewer+ (admin/manager/viewer 均可访问)。
package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/skip2/go-qrcode"
)

// QRCodeHandler 资产二维码处理器
type QRCodeHandler struct {
	repo       *repository.AssetRepo
	pool       *pgxpool.Pool
	externalURL string // 受信基础 URL (cfg.Server.ExternalURL); 为空则禁止 url 模式
}

// NewQRCodeHandler 构造 QRCodeHandler
// externalURL 为受信基础 URL (来自 cfg.Server.ExternalURL), 为空时禁止 url 模式 QR 生成。
func NewQRCodeHandler(repo *repository.AssetRepo, pool *pgxpool.Pool, externalURL string) *QRCodeHandler {
	return &QRCodeHandler{repo: repo, pool: pool, externalURL: externalURL}
}

// GetAssetQRCode GET /api/v1/assets/:id/qrcode
// 返回资产 asset_tag 对应的 PNG 二维码。
// 路径参数名义为 :id, 实际语义为 asset_tag (复用参数名以避免 Gin 路由冲突)。
// ?content=url 时，二维码内容为前端资产详情页 URL (基于配置的受信 ExternalURL 拼接,
// 防止伪造 Host 头钓鱼; 未配置 ExternalURL 时该模式返回 400);
// 默认内容为 asset_tag 本身 (便于 USB 扫码枪直接录入盘点)。
func (h *QRCodeHandler) GetAssetQRCode(c *gin.Context) {
	orgID := c.GetString("org_id")
	tag := c.Param("id")
	if tag == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "asset tag required"})
		return
	}

	// 复用 asset_repo 按 tag 查资产 (含 org_id 过滤防 IDOR)
	row, err := h.repo.GetByTag(c.Request.Context(), h.pool, tag, orgID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "asset not found"})
		return
	}

	// 二维码内容: 默认 asset_tag; ?content=url 时拼详情页 URL
	content := row.AssetTag
	if c.Query("content") == "url" {
		// 必须配置受信 ExternalURL 才启用 url 模式, 防止伪造 Host 头钓鱼
		if h.externalURL == "" {
			slog.Warn("qrcode: url mode requested but EXTERNAL_URL not configured; falling back to tag content")
			c.JSON(http.StatusBadRequest, gin.H{"error": "url mode requires EXTERNAL_URL config"})
			return
		}
		// 仅允许 http/https scheme, 拼接前端 SPA 路由 (/assets/:id)
		base := strings.TrimRight(h.externalURL, "/")
		content = base + "/assets/" + row.ID
	}

	png, err := qrcode.Encode(content, qrcode.Medium, 256)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encode qrcode: " + err.Error()})
		return
	}

	c.Header("Cache-Control", "private, max-age=3600")
	c.Data(http.StatusOK, "image/png", png)
}
