package lifecycle

import (
	"sync"
)

// Cache stores the latest lifecycle status snapshot for reuse.
// Now stores typed NodeStatus instead of map[string]any for type safety.
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

// LatestStatusMap returns the cached status as map[string]any for backward compatibility.
// This preserves the existing SnapshotSource interface used by status.Provider.
func (c *Cache) LatestStatusMap() (map[string]any, bool) {
	status, ok := c.LatestStatus()
	if !ok {
		return nil, false
	}
	return status.ToMap(), true
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		switch typed := value.(type) {
		case map[string]any:
			dst[key] = cloneAnyMap(typed)
		case []any:
			dst[key] = cloneAnySlice(typed)
		default:
			dst[key] = typed
		}
	}
	return dst
}

func cloneAnySlice(src []any) []any {
	if len(src) == 0 {
		return nil
	}
	out := make([]any, len(src))
	for idx, value := range src {
		switch typed := value.(type) {
		case map[string]any:
			out[idx] = cloneAnyMap(typed)
		case []any:
			out[idx] = cloneAnySlice(typed)
		default:
			out[idx] = typed
		}
	}
	return out
}
