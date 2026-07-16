// Package lock — 三层锁策略
// 对应架构文档 §8 并发控制与锁策略
package lock

import (
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LockType 锁类型
type LockType string

const (
	Optimistic LockType = "optimistic"  // 乐观锁 (~90%)
	Pessimistic LockType = "pessimistic" // 悲观锁 (~8%)
	Advisory   LockType = "advisory"    // Advisory 锁 (~2%)
)

// ===================================================================
// 乐观锁重试
// ===================================================================

// RetryConfig 乐观锁重试配置
type RetryConfig struct {
	MaxRetries int `default:"3"`
}

var DefaultRetryConfig = RetryConfig{MaxRetries: 3}

// WithOptimisticRetry 执行乐观锁操作 (最多 3 次重试)
// 对应架构文档 §8.2
func WithOptimisticRetry(fn func(version int) (bool, error), currentVersion int, cfg RetryConfig) error {
	version := currentVersion
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		ok, err := fn(version)
		if err != nil {
			return fmt.Errorf("attempt %d: %w", attempt, err)
		}
		if ok {
			return nil
		}
		// 版本冲突, 重试
	}
	return fmt.Errorf("max retries (%d) exceeded", cfg.MaxRetries)
}

// ===================================================================
// 悲观锁 — 全局排序防死锁
// ===================================================================

// SortedAssetIDs 按 UUID 字典序排列 (死锁预防)
// 对应架构文档 §8.3
func SortedAssetIDs(ids []string) []string {
	sorted := make([]string, len(ids))
	copy(sorted, ids)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})
	return sorted
}

// ValidateSortedOrder 验证是否按 UUID 字典序排序
func ValidateSortedOrder(ids []string) error {
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			return fmt.Errorf("assets not in sorted order: %s > %s", ids[i-1], ids[i])
		}
	}
	return nil
}

// ===================================================================
// Advisory 锁 — 非阻塞 + 碰撞检测
// ===================================================================

// TryAdvisoryLock 非阻塞获取 advisory 锁
// 对应架构文档 §8.4
func TryAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, locationID uuid.UUID) (bool, error) {
	lockID := hashUUIDToInt64(locationID, 0)
	var acquired bool
	err := pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID).Scan(&acquired)
	return acquired, err
}

// UnlockAdvisory 释放 advisory 锁
func UnlockAdvisory(ctx context.Context, pool *pgxpool.Pool, locationID uuid.UUID) error {
	lockID := hashUUIDToInt64(locationID, 0)
	_, err := pool.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockID)
	return err
}

// TryAdvisoryLockMulti 多槽 advisory 锁备选 (碰撞 fallback)
// 对应架构文档: 碰撞检测补充方案
func TryAdvisoryLockMulti(ctx context.Context, pool *pgxpool.Pool, locationID uuid.UUID) (bool, error) {
	lockID1 := hashUUIDToInt64(locationID, 0)
	lockID2 := hashUUIDToInt64(locationID, 1)
	var a1, a2 bool
	if err := pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID1).Scan(&a1); err != nil {
		return false, err
	}
	if err := pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockID2).Scan(&a2); err != nil {
		return false, err
	}
	return a1 && a2, nil
}

// DetectCollision 启动时检测 UUID→int64 碰撞
// 对应架构文档 §8.4 碰撞检测
func DetectCollision(ids []uuid.UUID) error {
	seen := make(map[int64]uuid.UUID)
	for _, id := range ids {
		h := hashUUIDToInt64(id, 0)
		if existing, ok := seen[h]; ok {
			return fmt.Errorf("advisory lock collision: %s and %s both hash to %d", id, existing, h)
		}
		seen[h] = id
	}
	return nil
}

func hashUUIDToInt64(id uuid.UUID, seed int) int64 {
	h := fnv.New64a()
	h.Write(id[:])
	binary.Write(h, binary.LittleEndian, int32(seed))
	return int64(h.Sum64())
}
