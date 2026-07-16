// Package repository — PostgreSQL 数据访问层 (pgx/v5)
package repository

import (
	"context"
	"fmt"
	"log"

	"github.com/Gendmyb/Asset-Database-System/assetserver/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool 创建 pgx 连接池
func NewPool(ctx context.Context, cfg *config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = int32(cfg.MinConns)
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	log.Printf("PostgreSQL connected: %s:%s/%s (pool: %d-%d)",
		cfg.Host, cfg.Port, cfg.Name, cfg.MinConns, cfg.MaxConns)
	return pool, nil
}
