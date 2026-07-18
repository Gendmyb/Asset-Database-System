// Package db — 数据库迁移执行器 (自写 runner，无外部依赖)
// 对应 Phase B 数据层地基改造 Step 1
package db

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	assetserver "github.com/Gendmyb/Asset-Database-System/assetserver"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunMigrations 执行所有未应用的迁移文件 (按文件名排序)
// 幂等: 已执行的迁移不会重复运行
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// 1. 确保 assets schema 存在 (001_init.sql 可能尚未执行)
	if _, err := pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS assets`); err != nil {
		return fmt.Errorf("ensure assets schema: %w", err)
	}

	// 2. 确保 schema_migrations 表存在
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS assets.schema_migrations (
			version     VARCHAR(255) PRIMARY KEY,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	// 3. 读取嵌入的迁移文件
	entries, err := assetserver.MigrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// 4. 逐文件应用未执行的迁移
	for _, f := range files {
		version := strings.TrimSuffix(f, ".sql")

		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM assets.schema_migrations WHERE version = $1)`, version,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			log.Printf("Migration %s already applied, skipping", version)
			continue
		}

		log.Printf("Applying migration: %s", f)
		sqlBytes, err := assetserver.MigrationsFS.ReadFile("migrations/" + f)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", f, err)
		}

		// 在事务内执行: 锁表 → 执行 SQL → 记录版本
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", f, err)
		}
		defer tx.Rollback(ctx)

		if _, err := tx.Exec(ctx, `LOCK TABLE assets.schema_migrations IN EXCLUSIVE MODE`); err != nil {
			return fmt.Errorf("lock schema_migrations: %w", err)
		}

		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("run migration %s: %w", f, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO assets.schema_migrations (version) VALUES ($1)`, version,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", f, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", f, err)
		}

		log.Printf("Migration %s applied successfully", version)
	}

	return nil
}
