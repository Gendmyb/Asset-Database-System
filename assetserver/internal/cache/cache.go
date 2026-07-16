// Package cache — 缓存层 (Cache-Aside + 三防)
// 对应架构文档 §11 缓存策略
package cache

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

// Cache 缓存接口
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// CacheAside 模式: GetOrSet
// 查缓存 → 命中返回 / 未命中调 loader → 回填 → 返回
type CacheAside struct {
	cache    Cache
	mu       sync.Map // key → *sync.Mutex (防击穿)
	jitterFn func(time.Duration) time.Duration
}

func NewCacheAside(c Cache) *CacheAside {
	return &CacheAside{
		cache: c,
		jitterFn: func(base time.Duration) time.Duration {
			j := time.Duration(rand.Int63n(int64(base) / 3))
			return base + j
		},
	}
}

const nullMarker = "__NULL__"
const nullTTL = 30 * time.Second

// GetOrSet Cache-Aside + 三防 (雪崩/击穿/穿透)
func (ca *CacheAside) GetOrSet(ctx context.Context, key string, ttl time.Duration, loader func() (string, error)) (string, error) {
	// 1. 查缓存
	val, err := ca.cache.Get(ctx, key)
	if err == nil {
		if val == nullMarker {
			return "", ErrNotFound
		}
		return val, nil
	}

	// 2. 防击穿: 热点 key 互斥锁
	muI, _ := ca.mu.LoadOrStore(key, &sync.Mutex{})
	mu := muI.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Double check
	val, err = ca.cache.Get(ctx, key)
	if err == nil {
		mu.Unlock()
		if val == nullMarker {
			return "", ErrNotFound
		}
		return val, nil
	}
	mu.Unlock()

	// 3. 调 loader
	loaded, err := loader()
	if err != nil {
		// 空值缓存 30s 防穿透
		ca.cache.Set(ctx, key, nullMarker, nullTTL)
		return "", err
	}

	// 4. 回填 (TTL + jitter 防雪崩)
	jitteredTTL := ca.jitterFn(ttl)
	ca.cache.Set(ctx, key, loaded, jitteredTTL)
	return loaded, nil
}

// WriteInvalidate 写入时失效 + 延迟双删
func (ca *CacheAside) WriteInvalidate(ctx context.Context, key string) {
	ca.cache.Del(ctx, key)
	time.Sleep(500 * time.Millisecond) // 延迟双删
	ca.cache.Del(ctx, key)
}

var ErrNotFound = &cacheError{"cache: not found"}

type cacheError struct{ msg string }
func (e *cacheError) Error() string { return e.msg }
