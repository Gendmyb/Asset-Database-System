// Package assetserver — 嵌入资源 (迁移 SQL 文件)
package assetserver

import "embed"

//go:embed migrations/*.sql
var MigrationsFS embed.FS
