package queue

import (
	"time"

	"github.com/iw2rmb/ploy/internal/storage/openrewrite"
)

// Job represents a transformation job in the queue
type Job struct {
	ID        string                    `json:"id"`
	Priority  int                       `json:"priority"`
	TarData   []byte                    `json:"-"` // Excluded from JSON
	Recipe    openrewrite.RecipeConfig  `json:"recipe"`
	CreatedAt time.Time                 `json:"created_at"`
	Retries   int                       `json:"retries"`
	index     int                       // The index of the item in the heap
}

// JobHeap is a min-heap of Jobs with custom priority ordering
type JobHeap []*Job

// Len returns the number of jobs in the heap
func (h JobHeap) Len() int { return len(h) }

// Less returns true if job i has higher priority than job j
// Higher priority values come first, then older jobs
func (h JobHeap) Less(i, j int) bool {
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority // Higher priority first
	}
	return h[i].CreatedAt.Before(h[j].CreatedAt) // Older jobs first
}

// Swap swaps two jobs in the heap
func (h JobHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

// Push adds a job to the heap
func (h *JobHeap) Push(x interface{}) {
	n := len(*h)
	job := x.(*Job)
	job.index = n
	*h = append(*h, job)
}

// Pop removes and returns the highest priority job from the heap
func (h *JobHeap) Pop() interface{} {
	old := *h
	n := len(old)
	job := old[n-1]
	old[n-1] = nil  // avoid memory leak
	job.index = -1  // for safety
	*h = old[0 : n-1]
	return job
}