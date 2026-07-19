// Package web — 嵌入前端构建产物 (SPA dist)
// 对应架构文档 §13.3 多阶段 Docker 构建 + embed
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist
var distFS embed.FS

// Sub returns the dist sub-filesystem (strips "dist/" prefix) for direct fs operations.
func Sub() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}

// Handler returns an http.Handler that serves the embedded SPA with proper
// client-side routing fallback.
//
// 注意: 裸 http.FileServer 不会做 SPA 回退——直接访问 /login 这类客户端路由时,
// 它会去找 dist/login 这个"文件", 找不到就返回 Go 默认的 "404 page not found"。
// 因此这里手动处理: 请求的静态文件不存在 (或是个目录) 时, 回退到 index.html,
// 交由前端路由器接管 (History API)。
func Handler() http.Handler {
	sub, err := Sub()
	if err != nil {
		// Should never happen since dist is always embedded
		panic("web: failed to sub dist: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))

	// 预读 index.html 用于 SPA 回退
	indexBytes, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic("web: failed to read index.html: " + err.Error())
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 归一化路径: 去掉前导 '/', 根路径直接当作 index.html
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}

		// 静态文件存在且不是目录 -> 交给 FileServer (带正确的 Content-Type/缓存头)
		if info, statErr := fs.Stat(sub, p); statErr == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// 否则回退到 index.html, 让前端路由器处理 (如 /login, /assets/xxx, /admin/users)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexBytes)
	})
}
