// Package lock — 锁策略测试
package lock

import (
	"testing"

	"github.com/google/uuid"
)

func TestSortedAssetIDs(t *testing.T) {
	ids := []string{"z", "a", "m", "b"}
	sorted := SortedAssetIDs(ids)

	// 原切片不变
	if ids[0] != "z" {
		t.Error("original slice should not be mutated")
	}

	// 排序正确
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1] > sorted[i] {
			t.Errorf("not sorted: %s > %s at %d", sorted[i-1], sorted[i], i)
		}
	}

	if err := ValidateSortedOrder(sorted); err != nil {
		t.Errorf("should be valid: %v", err)
	}
}

func TestAdvisoryCollisionDetection(t *testing.T) {
	// 生成 1000 个 UUID 并检测碰撞
	var ids []uuid.UUID
	for i := 0; i < 1000; i++ {
		ids = append(ids, uuid.New())
	}

	err := DetectCollision(ids)
	if err != nil {
		t.Logf("Collision detected (expected at ~1/2^64 prob): %v", err)
	}
}

func TestSortedDeadlockPrevention(t *testing.T) {
	// 模拟两个 goroutine 反向 transfer 的场景
	ids1 := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
	}
	ids2 := []string{
		"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		"550e8400-e29b-41d4-a716-446655440000",
	}

	sorted1 := SortedAssetIDs(ids1)
	sorted2 := SortedAssetIDs(ids2)

	// 两个 goroutine 排序后应得到相同顺序
	if sorted1[0] != sorted2[0] || sorted1[1] != sorted2[1] {
		t.Error("deadlock prevention failed: different goroutines get different lock orders")
	}
}
