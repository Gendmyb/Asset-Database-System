// Package web — 嵌入前端构建产物 (SPA dist)
// 对应架构文档 §13.3 多阶段 Docker 构建 + embed
package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var distFS embed.FS

// Sub returns the dist sub-filesystem (strips "dist/" prefix) for direct fs operations.
func Sub() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// Handler returns an http.Handler that serves the embedded SPA with proper
// client-side routing fallback. Use with NoRoute or as a catch-all.
func Handler() http.Handler {
	sub, err := Sub()
	if err != nil {
		// Should never happen since dist is always embedded
		panic("web: failed to sub dist: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}
