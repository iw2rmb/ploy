package lifecycle

import (
	"sync"
)

// Cache stores the latest lifecycle status snapshot for reuse.
type Cache struct {
	mu     sync.RWMutex
	status map[string]any
}

// NewCache constructs an empty lifecycle status cache.
func NewCache() *Cache {
	return &Cache{}
}

// Store replaces the cached status with the provided map.
func (c *Cache) Store(status map[string]any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.status = cloneAnyMap(status)
	c.mu.Unlock()
}

// LatestStatus returns a deep copy of the cached status when available.
func (c *Cache) LatestStatus() (map[string]any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.status) == 0 {
		return nil, false
	}
	return cloneAnyMap(c.status), true
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
