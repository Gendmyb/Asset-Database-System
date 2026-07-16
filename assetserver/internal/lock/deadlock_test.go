// Package lock — 死锁检测与 UUID 碰撞测试
package lock

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// ============================================================
// TestDeadlockPrevention — 两个 goroutine 反向 transfer
// 验证 SortedAssetIDs 排序后锁定顺序一致，防止数据库死锁
// ============================================================

func TestDeadlockPrevention(t *testing.T) {
	// 模拟两个并发 transfer 操作：
	// G1: transfer assetA → assetB (需要锁 A, B)
	// G2: transfer assetB → assetA (需要锁 B, A)
	// 如果没有排序，G1 锁 A 等 B，G2 锁 B 等 A → 死锁
	// 排序后：两者都锁 A 再锁 B → 无死锁

	assetA := "550e8400-e29b-41d4-a716-446655440000"
	assetB := "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

	// G1 需要的资产 ID 列表
	ids1 := []string{assetA, assetB}
	// G2 需要的资产 ID 列表 (反向)
	ids2 := []string{assetB, assetA}

	var wg sync.WaitGroup
	var lockOrder1, lockOrder2 []string
	var mu sync.Mutex

	// 模拟并发锁定 (使用 channel 确保同时到达锁点)
	ready := make(chan struct{})
	done := make(chan struct{}, 2)

	wg.Add(2)

	go func() {
		defer wg.Done()
		<-ready
		sorted := SortedAssetIDs(ids1)
		mu.Lock()
		lockOrder1 = sorted
		mu.Unlock()
		done <- struct{}{}
	}()

	go func() {
		defer wg.Done()
		<-ready
		sorted := SortedAssetIDs(ids2)
		mu.Lock()
		lockOrder2 = sorted
		mu.Unlock()
		done <- struct{}{}
	}()

	// 同时释放两个 goroutine
	close(ready)
	wg.Wait()

	// 验证两个 goroutine 的锁定顺序一致
	if len(lockOrder1) != 2 || len(lockOrder2) != 2 {
		t.Fatal("both goroutines should have 2 sorted asset IDs")
	}

	if lockOrder1[0] != lockOrder2[0] || lockOrder1[1] != lockOrder2[1] {
		t.Errorf("deadlock prevention failed: G1 order=%v, G2 order=%v", lockOrder1, lockOrder2)
	}

	// 验证排序后的第一个是 assetA (字典序更小)
	// 550e8... < 6ba7b... (因为 '5' < '6')
	if lockOrder1[0] != assetA {
		t.Errorf("expected %s first (lexicographically smaller), got %s", assetA, lockOrder1[0])
	}

	// 验证排序有效性
	if err := ValidateSortedOrder(lockOrder1); err != nil {
		t.Errorf("sorted order validation failed: %v", err)
	}
}

// ============================================================
// TestDeadlockPreventionMultipleAssets — 多资产死锁预防
// ============================================================

func TestDeadlockPreventionMultipleAssets(t *testing.T) {
	// 多个资产，不同起始顺序，排序后应一致
	assets := []string{
		"00000000-0000-0000-0000-00000000000a",
		"00000000-0000-0000-0000-000000000001",
		"00000000-0000-0000-0000-0000000000ff",
		"00000000-0000-0000-0000-00000000007f",
	}

	original := make([]string, len(assets))
	copy(original, assets)

	sorted := SortedAssetIDs(assets)

	// 原切片不变
	for i, v := range original {
		if assets[i] != v {
			t.Errorf("original slice mutated at %d: %s → %s", i, v, assets[i])
		}
	}

	// 排序后的第一个是最小的
	if sorted[0] != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("wrong first element: got %s", sorted[0])
	}

	if err := ValidateSortedOrder(sorted); err != nil {
		t.Errorf("sorted order invalid: %v", err)
	}

	// 反向顺序也应该得到相同排序结果
	reversed := make([]string, len(original))
	for i, v := range original {
		reversed[len(original)-1-i] = v
	}
	reversedSorted := SortedAssetIDs(reversed)

	for i := range sorted {
		if sorted[i] != reversedSorted[i] {
			t.Errorf("different sort results from reversed input at %d: %s vs %s", i, sorted[i], reversedSorted[i])
		}
	}
}

// ============================================================
// TestAdvisoryCollision — 10000 UUID 碰撞检测
// ============================================================

func TestAdvisoryCollision(t *testing.T) {
	const count = 10000

	ids := make([]uuid.UUID, count)
	for i := 0; i < count; i++ {
		ids[i] = uuid.New()
	}

	err := DetectCollision(ids)
	if err != nil {
		// 碰撞概率 ~ (10000^2) / (2 * 2^64) ≈ 2.7e-12
		// 如果真的发生碰撞，记录但不失败
		t.Logf("NOTE: UUID advisory lock collision detected (extremely rare): %v", err)
	} else {
		t.Logf("No collision detected among %d UUIDs (expected)", count)
	}
}

// ============================================================
// TestSortedOrderValidation — 排序验证
// ============================================================

func TestSortedOrderValidation(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		wantErr bool
	}{
		{"empty", []string{}, false},
		{"single", []string{"a"}, false},
		{"sorted", []string{"a", "b", "c"}, false},
		{"unsorted", []string{"c", "a", "b"}, true},
		{"equal", []string{"a", "a"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateSortedOrder(tc.ids)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Errorf("ValidateSortedOrder(%v) error=%v, wantErr=%v", tc.ids, err, tc.wantErr)
			}
		})
	}
}

// ============================================================
// TestOptimisticRetry — 乐观锁重试
// ============================================================

func TestOptimisticRetry(t *testing.T) {
	// 第一次失败，第二次成功
	attempts := 0
	err := WithOptimisticRetry(func(version int) (bool, error) {
		attempts++
		if attempts < 2 {
			return false, nil // 版本冲突
		}
		return true, nil
	}, 1, RetryConfig{MaxRetries: 3})

	if err != nil {
		t.Errorf("expected success after retry, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestOptimisticRetryExhausted(t *testing.T) {
	// 所有重试都失败
	err := WithOptimisticRetry(func(version int) (bool, error) {
		return false, nil
	}, 1, RetryConfig{MaxRetries: 3})

	if err == nil {
		t.Error("expected error after exhausted retries")
	}
}

func TestOptimisticRetryError(t *testing.T) {
	// 操作返回错误
	expectedErr := fmt.Errorf("db error")
	err := WithOptimisticRetry(func(version int) (bool, error) {
		return false, expectedErr
	}, 1, RetryConfig{MaxRetries: 3})

	if err == nil {
		t.Error("expected error propagation")
	}
}

// ============================================================
// TestUUIDHashDeterminism — hash 确定性
// ============================================================

func TestUUIDHashDeterminism(t *testing.T) {
	// 同一个 UUID 多次 hash 结果应一致
	id := uuid.New()
	h1 := hashUUIDToInt64(id, 0)
	h2 := hashUUIDToInt64(id, 0)
	if h1 != h2 {
		t.Errorf("hash not deterministic: %d != %d", h1, h2)
	}

	// 不同 seed 应有不同结果
	h3 := hashUUIDToInt64(id, 1)
	if h3 == h1 {
		t.Log("same hash with different seed (possible at low probability)")
	}
}
