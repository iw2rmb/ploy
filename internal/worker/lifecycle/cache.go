package lifecycle

import (
	"sync"
)

// Cache stores the latest lifecycle status snapshot for reuse.
// Stores typed NodeStatus for compile-time safety; use LatestStatus()
// to retrieve the cached status.
//
// Cache methods are nil-safe: calling Store or LatestStatus on a nil
// Cache is a no-op, allowing optional cache usage without nil checks.
type Cache struct {
	mu     sync.RWMutex
	status *NodeStatus
}

// NewCache constructs an empty lifecycle status cache.
func NewCache() *Cache {
	return &Cache{}
}

// Store replaces the cached status with the provided NodeStatus.
func (c *Cache) Store(status NodeStatus) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.status = &status
	c.mu.Unlock()
}

// LatestStatus returns a copy of the cached NodeStatus when available.
func (c *Cache) LatestStatus() (NodeStatus, bool) {
	if c == nil {
		return NodeStatus{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.status == nil {
		return NodeStatus{}, false
	}
	return *c.status, true
}
