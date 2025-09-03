package config

import (
    "sync"
    "time"
)

// Cache is a tiny TTL cache used for configuration snapshots.
type Cache struct {
    mu    sync.RWMutex
    items map[string]cacheItem
    ttl   time.Duration
}

type cacheItem struct {
    v   interface{}
    exp time.Time
}

func NewCache() *Cache {
    return &Cache{items: make(map[string]cacheItem), ttl: 5 * time.Minute}
}

func (c *Cache) SetTTL(ttl time.Duration) { c.ttl = ttl }

func (c *Cache) Get(key string) (interface{}, bool) {
    if c == nil {
        return nil, false
    }
    c.mu.RLock()
    it, ok := c.items[key]
    c.mu.RUnlock()
    if !ok || time.Now().After(it.exp) {
        return nil, false
    }
    return it.v, true
}

func (c *Cache) Set(key string, v interface{}) {
    if c == nil {
        return
    }
    c.mu.Lock()
    c.items[key] = cacheItem{v: v, exp: time.Now().Add(c.ttl)}
    c.mu.Unlock()
}

