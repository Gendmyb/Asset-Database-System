// Package webfs — 嵌入前端构建产物 (SPA dist)
// 对应架构文档 §13.3 多阶段 Docker 构建 + embed
package webfs

import (
	"embed"
	"net/http"
)

//go:embed ../../web/dist
var distFS embed.FS

// Handler returns an http.FileSystem for the embedded frontend assets.
// In the Docker multi-stage build, web/dist is populated by the web-builder stage.
// For local development, run 'npm run build' in the web/ directory first,
// then copy/symlink the output to assetserver/web/dist/.
func Handler() http.FileSystem {
	return http.FS(distFS)
}
