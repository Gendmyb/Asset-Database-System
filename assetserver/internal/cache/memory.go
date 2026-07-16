// Package cache — 内存缓存实现
package cache

import (
	"context"
	"sync"
	"time"
)

type item struct {
	value  string
	expiry time.Time
}

// MemoryCache 内存缓存 (开发环境)
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]item
}

func NewMemoryCache() *MemoryCache {
	c := &MemoryCache{items: make(map[string]item)}
	go c.cleanupLoop()
	return c
}

func (c *MemoryCache) Get(ctx context.Context, key string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	it, ok := c.items[key]
	if !ok || (it.expiry.Before(time.Now()) && !it.expiry.IsZero()) {
		return "", ErrNotFound
	}
	return it.value, nil
}

func (c *MemoryCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = item{value: value, expiry: time.Now().Add(ttl)}
	return nil
}

func (c *MemoryCache) Del(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
	return nil
}

func (c *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	it, ok := c.items[key]
	if !ok {
		return false, nil
	}
	return it.expiry.After(time.Now()), nil
}

func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.items {
			if !v.expiry.IsZero() && v.expiry.Before(now) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}
