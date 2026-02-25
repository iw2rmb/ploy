package jobchain

import (
	"sort"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// Order reconstructs a linked job chain using job IDs and next-id successor pointers.
// It returns a deterministic order even for malformed/disconnected chains.
func Order[T any](
	items []T,
	idOf func(T) domaintypes.JobID,
	nextOf func(T) *domaintypes.JobID,
) []T {
	if len(items) <= 1 {
		return items
	}

	itemByID := make(map[domaintypes.JobID]T, len(items))
	orderedIDs := make([]domaintypes.JobID, 0, len(items))
	predecessors := make(map[domaintypes.JobID]int, len(items))
	nextByID := make(map[domaintypes.JobID]domaintypes.JobID, len(items))

	for _, item := range items {
		id := idOf(item)
		itemByID[id] = item
		orderedIDs = append(orderedIDs, id)
		predecessors[id] = 0
	}

	for _, item := range items {
		id := idOf(item)
		next := nextOf(item)
		if next == nil || next.IsZero() {
			continue
		}
		nextID := *next
		if _, ok := itemByID[nextID]; !ok {
			continue
		}
		predecessors[nextID]++
		nextByID[id] = nextID
	}

	heads := make([]domaintypes.JobID, 0, len(items))
	for _, id := range orderedIDs {
		if predecessors[id] == 0 {
			heads = append(heads, id)
		}
	}
	sortJobIDs(heads)

	out := make([]T, 0, len(items))
	visited := make(map[domaintypes.JobID]struct{}, len(items))

	walkChain := func(start domaintypes.JobID) {
		current := start
		for {
			if _, seen := visited[current]; seen {
				return
			}
			item, ok := itemByID[current]
			if !ok {
				return
			}
			visited[current] = struct{}{}
			out = append(out, item)

			nextID, ok := nextByID[current]
			if !ok {
				return
			}
			current = nextID
		}
	}

	for _, head := range heads {
		walkChain(head)
	}

	if len(out) == len(items) {
		return out
	}

	remaining := make([]domaintypes.JobID, 0, len(items)-len(out))
	for _, id := range orderedIDs {
		if _, ok := visited[id]; !ok {
			remaining = append(remaining, id)
		}
	}
	sortJobIDs(remaining)
	for _, id := range remaining {
		walkChain(id)
	}

	return out
}

func sortJobIDs(ids []domaintypes.JobID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
}
