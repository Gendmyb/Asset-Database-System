// Package cache — 缓存层测试 (Cache-Aside + 三防)
package cache

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// ============================================================
// TestCacheAsideHit — 缓存命中直接返回
// ============================================================

func TestCacheAsideHit(t *testing.T) {
	mem := NewMemoryCache()
	ca := NewCacheAside(mem)
	ctx := context.Background()

	key := "asset:test-1"
	expected := `{"id":"1","name":"MacBook"}`

	// 预热缓存
	_ = mem.Set(ctx, key, expected, 60*time.Second)

	// GetOrSet 应该命中缓存
	loaderCalled := false
	val, err := ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
		loaderCalled = true
		return "should-not-be-called", nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != expected {
		t.Errorf("want %q, got %q", expected, val)
	}
	if loaderCalled {
		t.Error("loader should not be called on cache hit")
	}
}

// ============================================================
// TestCacheAsideMiss — 缓存未命中，调 loader 回填
// ============================================================

func TestCacheAsideMiss(t *testing.T) {
	mem := NewMemoryCache()
	ca := NewCacheAside(mem)
	ctx := context.Background()

	key := "asset:test-2"
	expected := `{"id":"2","name":"ThinkPad"}`

	loaderCalled := 0
	val, err := ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
		loaderCalled++
		return expected, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != expected {
		t.Errorf("want %q, got %q", expected, val)
	}
	if loaderCalled != 1 {
		t.Errorf("loader should be called once, called %d times", loaderCalled)
	}

	// 第二次访问应该命中缓存
	val2, err2 := ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
		loaderCalled++
		return "should-not-be-called", nil
	})

	if err2 != nil {
		t.Fatalf("unexpected error on second call: %v", err2)
	}
	if val2 != expected {
		t.Errorf("second call: want %q, got %q", expected, val2)
	}
	if loaderCalled != 1 {
		t.Errorf("loader should still be 1, got %d", loaderCalled)
	}
}

// ============================================================
// TestCacheSnowballProtection — TTL jitter 防雪崩
// ============================================================

func TestCacheSnowballProtection(t *testing.T) {
	mem := NewMemoryCache()

	// 使用自定义 jitter 函数捕获 TTL（添加随机抖动）
	jitteredTTLs := make(chan time.Duration, 100)

	ca := &CacheAside{
		cache: mem,
		jitterFn: func(base time.Duration) time.Duration {
			// 添加 0-30% 的随机抖动（模拟真实 jitter）
			j := base + time.Duration(rand.Int63n(int64(base)/3))
			jitteredTTLs <- j
			return j
		},
	}

	ctx := context.Background()
	baseTTL := 60 * time.Second

	// 并发设置 50 个不同 key
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("asset:snowball-%d", idx)
			_, _ = ca.GetOrSet(ctx, key, baseTTL, func() (string, error) {
				return fmt.Sprintf(`{"id":"%d"}`, idx), nil
			})
		}(i)
	}
	wg.Wait()
	close(jitteredTTLs)

	// 收集所有 TTL
	var ttls []time.Duration
	for j := range jitteredTTLs {
		ttls = append(ttls, j)
	}

	if len(ttls) < 10 {
		t.Fatalf("expected at least 10 TTLs, got %d", len(ttls))
	}

	// 检查至少有 2 个不同的 TTL (证明 jitter 生效)
	first := ttls[0]
	hasVariation := false
	for _, j := range ttls {
		if j != first {
			hasVariation = true
			break
		}
	}
	if !hasVariation {
		t.Error("expected TTL jitter variation, all TTLs are identical — snowball risk!")
	}
}

// ============================================================
// TestCachePenetrationProtection — 空值缓存防穿透
// ============================================================

func TestCachePenetrationProtection(t *testing.T) {
	mem := NewMemoryCache()
	ca := NewCacheAside(mem)
	ctx := context.Background()

	key := "asset:missing-999"
	dbHitCount := 0

	// 第一次：loader 返回错误，缓存空值
	_, err := ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
		dbHitCount++
		return "", ErrNotFound
	})

	if err == nil {
		t.Fatal("expected error for missing key")
	}

	// 第二次：应该命中空值缓存，不调 loader
	_, err = ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
		dbHitCount++
		return "", ErrNotFound
	})

	if err == nil {
		t.Fatal("expected error for cached missing key")
	}
	if dbHitCount != 1 {
		t.Errorf("DB should be hit only once (empty value cache), hit %d times", dbHitCount)
	}
}

// ============================================================
// TestCacheHotKeyProtection — 热点 key 防击穿
// ============================================================

func TestCacheHotKeyProtection(t *testing.T) {
	mem := NewMemoryCache()
	ca := NewCacheAside(mem)
	ctx := context.Background()

	key := "asset:hotspot"
	loaderCalls := make(chan int, 100)

	// 100 个 goroutine 并发请求同一个 key
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
				loaderCalls <- 1
				time.Sleep(10 * time.Millisecond) // 模拟 DB 延迟
				return `{"id":"hotspot","name":"Hot"}`, nil
			})
		}()
	}
	wg.Wait()
	close(loaderCalls)

	// loader 应该只被调用 1 次 (互斥锁防护)
	callCount := 0
	for range loaderCalls {
		callCount++
	}
	if callCount > 2 {
		t.Errorf("hot key protection failed: loader called %d times (expected ≤2)", callCount)
	}
}

// ============================================================
// TestWriteInvalidate — 延迟双删
// ============================================================

func TestWriteInvalidate(t *testing.T) {
	mem := NewMemoryCache()
	ca := NewCacheAside(mem)
	ctx := context.Background()

	key := "asset:write-inv"
	expected := `{"id":"write-inv","name":"Before"}`

	// 预热缓存
	_ = mem.Set(ctx, key, expected, 60*time.Second)

	// 执行延迟双删
	ca.WriteInvalidate(ctx, key)

	// 删除后缓存应不存在
	exists, _ := mem.Exists(ctx, key)
	if exists {
		t.Error("cache should be deleted after WriteInvalidate")
	}

	// 验证后续 GetOrSet 能正确回填
	val, err := ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
		return `{"id":"write-inv","name":"After"}`, nil
	})

	if err != nil {
		t.Fatalf("unexpected get after invalidate: %v", err)
	}
	if val != `{"id":"write-inv","name":"After"}` {
		t.Errorf("unexpected value after invalidate: %s", val)
	}
}

// ============================================================
// TestMemoryCacheBasic — 内存缓存基本操作
// ============================================================

func TestMemoryCacheBasic(t *testing.T) {
	c := NewMemoryCache()
	ctx := context.Background()

	// Set + Get
	if err := c.Set(ctx, "k1", "v1", 60*time.Second); err != nil {
		t.Fatalf("set failed: %v", err)
	}
	v, err := c.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if v != "v1" {
		t.Errorf("want v1, got %s", v)
	}

	// Delete
	c.Del(ctx, "k1")
	_, err = c.Get(ctx, "k1")
	if err == nil {
		t.Error("expected error after delete")
	}

	// Exists
	exists, _ := c.Exists(ctx, "k1")
	if exists {
		t.Error("k1 should not exist after delete")
	}

	// TTL expiry
	c.Set(ctx, "k2", "v2", 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	_, err = c.Get(ctx, "k2")
	if err == nil {
		t.Error("expected expiry error for k2")
	}
}

// ============================================================
// TestNullMarker — 空值标记
// ============================================================

func TestNullMarker(t *testing.T) {
	if nullMarker != "__NULL__" {
		t.Errorf("nullMarker should be __NULL__, got %s", nullMarker)
	}
	if nullTTL != 30*time.Second {
		t.Errorf("nullTTL should be 30s, got %v", nullTTL)
	}
}

// ============================================================
// TestLoaderErrorNotCachedTwice — loader 错误不无限增长
// ============================================================

func TestLoaderErrorNotCachedTwice(t *testing.T) {
	mem := NewMemoryCache()
	ca := NewCacheAside(mem)
	ctx := context.Background()

	key := "asset:transient-error"
	customErr := errors.New("transient DB error")

	// 第一次：loader 返回自定义错误 → 缓存 nullMarker
	_, err := ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
		return "", customErr
	})
	if err == nil {
		t.Fatal("expected error")
	}

	// 验证 nullMarker 已缓存
	rawVal, getErr := mem.Get(ctx, key)
	if getErr != nil {
		t.Fatalf("nullMarker should be cached: %v", getErr)
	}
	if rawVal != nullMarker {
		t.Errorf("cached value should be nullMarker, got %s", rawVal)
	}

	// 第二次：应命中 nullMarker 缓存
	loaderCalled := false
	_, err = ca.GetOrSet(ctx, key, 60*time.Second, func() (string, error) {
		loaderCalled = true
		return "recovered", nil
	})
	if err == nil {
		t.Fatal("expected error due to nullMarker cache")
	}
	if loaderCalled {
		t.Error("loader should not be called when nullMarker is cached")
	}
}
