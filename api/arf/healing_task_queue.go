package arf

import (
	"container/heap"
	"context"
	"sync"
	"time"
)

// HealingTask represents a healing attempt to be coordinated
type HealingTask struct {
	TransformID string
	AttemptPath string
	Errors      []string
	ParentPath  string
	Priority    int // Lower values = higher priority
	SubmittedAt time.Time
	ExecuteFn   func(context.Context) error
}

// TaskQueue implements a priority queue for healing tasks
type TaskQueue struct {
	items []*HealingTask
	mutex sync.RWMutex
}

// PushTask adds a task to the queue
func (tq *TaskQueue) PushTask(task *HealingTask) {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	heap.Push(tq, task)
}

// PopTask removes and returns the highest priority task
func (tq *TaskQueue) PopTask() *HealingTask {
	tq.mutex.Lock()
	defer tq.mutex.Unlock()

	if len(tq.items) == 0 {
		return nil
	}

	return heap.Pop(tq).(*HealingTask)
}

// Size returns the queue length (with lock for external access)
func (tq *TaskQueue) Size() int {
	tq.mutex.RLock()
	defer tq.mutex.RUnlock()
	return len(tq.items)
}

// Heap interface implementation for TaskQueue (methods called by heap package)

// Len implements heap.Interface (assumes caller holds appropriate locks)
func (tq *TaskQueue) Len() int {
	return len(tq.items)
}

// Less implements heap.Interface (lower priority value = higher priority)
func (tq *TaskQueue) Less(i, j int) bool {
	// Lower priority value means higher priority
	if tq.items[i].Priority != tq.items[j].Priority {
		return tq.items[i].Priority < tq.items[j].Priority
	}

	// If same priority, use submission time (FIFO)
	return tq.items[i].SubmittedAt.Before(tq.items[j].SubmittedAt)
}

// Swap implements heap.Interface
func (tq *TaskQueue) Swap(i, j int) {
	tq.items[i], tq.items[j] = tq.items[j], tq.items[i]
}

// Push implements heap.Interface
func (tq *TaskQueue) Push(x interface{}) {
	tq.items = append(tq.items, x.(*HealingTask))
}

// Pop implements heap.Interface
func (tq *TaskQueue) Pop() interface{} {
	old := tq.items
	n := len(old)
	item := old[n-1]
	tq.items = old[0 : n-1]
	return item
}
