package events

import (
	"container/list"
	"sync"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type jobContextCacheEntry struct {
	jobID domaintypes.JobID
	ctx   jobContext
}

type jobContextCache struct {
	mu         sync.Mutex
	maxEntries int
	entries    map[domaintypes.JobID]*list.Element
	lru        *list.List
}

func newJobContextCache(maxEntries int) *jobContextCache {
	return &jobContextCache{
		maxEntries: maxEntries,
		entries:    make(map[domaintypes.JobID]*list.Element, maxEntries),
		lru:        list.New(),
	}
}

func (c *jobContextCache) Get(jobID domaintypes.JobID) (jobContext, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[jobID]
	if !ok {
		return jobContext{}, false
	}

	c.lru.MoveToFront(elem)
	entry := elem.Value.(jobContextCacheEntry)
	return entry.ctx, true
}

func (c *jobContextCache) Set(jobID domaintypes.JobID, ctx jobContext) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.entries[jobID]; ok {
		elem.Value = jobContextCacheEntry{
			jobID: jobID,
			ctx:   ctx,
		}
		c.lru.MoveToFront(elem)
		return
	}

	elem := c.lru.PushFront(jobContextCacheEntry{
		jobID: jobID,
		ctx:   ctx,
	})
	c.entries[jobID] = elem

	if c.lru.Len() <= c.maxEntries {
		return
	}

	last := c.lru.Back()
	if last == nil {
		return
	}
	evicted := last.Value.(jobContextCacheEntry)
	delete(c.entries, evicted.jobID)
	c.lru.Remove(last)
}
