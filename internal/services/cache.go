/*
 * File: cache.go
 * Project: mimoproxy
 * Created: 2026-04-29
 */

package services

import (
	"sync"
	"time"
)

type CacheItem struct {
	Value      interface{}
	Expiration int64
}

type MemoryCache struct {
	items map[string]CacheItem
	mutex sync.RWMutex
}

var GlobalCache = &MemoryCache{
	items: make(map[string]CacheItem),
}

func (c *MemoryCache) Set(key string, value interface{}, duration time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.items[key] = CacheItem{
		Value:      value,
		Expiration: time.Now().Add(duration).Unix(),
	}
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	item, found := c.items[key]
	if !found {
		return nil, false
	}
	if time.Now().Unix() > item.Expiration {
		return nil, false
	}
	return item.Value, true
}

func (c *MemoryCache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.items, key)
}
