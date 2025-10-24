package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

// Node is an alias for sshtransport.Node to avoid exposing internal structs twice.
type Node = sshtransport.Node

// Cache persists control-plane node metadata and job assignments for tunnel routing.
type Cache struct {
	mu    sync.Mutex
	path  string
	state cacheState
}

type cacheState struct {
	Version   int                  `json:"version"`
	UpdatedAt time.Time            `json:"updated_at,omitempty"`
	Nodes     map[string]nodeEntry `json:"nodes,omitempty"`
	Jobs      map[string]jobEntry  `json:"jobs,omitempty"`
}

type nodeEntry struct {
	Node      Node      `json:"node"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type jobEntry struct {
	NodeID    string    `json:"node_id"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

const (
	cacheVersion = 1
)

// New constructs a cache backed by the provided filesystem path.
func New(path string) (*Cache, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, errors.New("cache: path required")
	}
	dir := filepath.Dir(trimmed)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cache: ensure directory: %w", err)
	}

	cache := &Cache{path: trimmed, state: cacheState{
		Version: cacheVersion,
		Nodes:   make(map[string]nodeEntry),
		Jobs:    make(map[string]jobEntry),
	}}
	if err := cache.load(); err != nil {
		return nil, err
	}
	return cache, nil
}

// RememberNodes replaces the stored node snapshot with the supplied set.
func (c *Cache) RememberNodes(nodes []Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state.Nodes == nil {
		c.state.Nodes = make(map[string]nodeEntry)
	}

	next := make(map[string]nodeEntry, len(nodes))
	now := time.Now().UTC()
	for _, node := range nodes {
		trimmedID := strings.TrimSpace(node.ID)
		if trimmedID == "" {
			continue
		}
		node.ID = trimmedID
		next[trimmedID] = nodeEntry{
			Node:      normalizeNode(node),
			UpdatedAt: now,
		}
	}
	c.state.Nodes = next
	c.state.UpdatedAt = now
	return c.persistLocked()
}

// Nodes returns the cached node snapshot.
func (c *Cache) Nodes() []Node {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.state.Nodes) == 0 {
		return nil
	}
	nodes := make([]Node, 0, len(c.state.Nodes))
	for _, entry := range c.state.Nodes {
		nodes = append(nodes, entry.Node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}

// RememberJob records the node assignment for the specified job identifier.
func (c *Cache) RememberJob(jobID, nodeID string, at time.Time) error {
	trimmedJob := strings.TrimSpace(jobID)
	if trimmedJob == "" {
		return errors.New("cache: job id required")
	}
	trimmedNode := strings.TrimSpace(nodeID)
	if trimmedNode == "" {
		return errors.New("cache: node id required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state.Jobs == nil {
		c.state.Jobs = make(map[string]jobEntry)
	}
	c.state.Jobs[trimmedJob] = jobEntry{
		NodeID:    trimmedNode,
		UpdatedAt: at.UTC(),
	}
	if c.state.UpdatedAt.Before(at) {
		c.state.UpdatedAt = at.UTC()
	}
	return c.persistLocked()
}

// LookupJob returns the node identifier associated with the job, if present.
func (c *Cache) LookupJob(jobID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.state.Jobs[strings.TrimSpace(jobID)]
	if !ok {
		return "", false
	}
	return entry.NodeID, true
}

// RemoveJob removes the cached assignment for the specified job identifier.
func (c *Cache) RemoveJob(jobID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	trimmed := strings.TrimSpace(jobID)
	if trimmed == "" {
		return nil
	}
	delete(c.state.Jobs, trimmed)
	return c.persistLocked()
}

func (c *Cache) load() error {
	data, err := os.ReadFile(c.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("cache: read state: %w", err)
	}
	var state cacheState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("cache: decode state: %w", err)
	}
	if state.Version != cacheVersion {
		state.Version = cacheVersion
	}
	if state.Nodes == nil {
		state.Nodes = make(map[string]nodeEntry)
	}
	if state.Jobs == nil {
		state.Jobs = make(map[string]jobEntry)
	}
	c.state = state
	return nil
}

func (c *Cache) persistLocked() error {
	c.state.Version = cacheVersion
	data, err := json.MarshalIndent(c.state, "", "  ")
	if err != nil {
		return fmt.Errorf("cache: encode state: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("cache: write temp state: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("cache: activate state: %w", err)
	}
	return nil
}

func normalizeNode(node Node) Node {
	node.ID = strings.TrimSpace(node.ID)
	node.Address = strings.TrimSpace(node.Address)
	if node.SSHPort <= 0 {
		node.SSHPort = 22
	}
	if node.APIPort <= 0 {
		node.APIPort = 8443
	}
	node.User = strings.TrimSpace(node.User)
	return node
}
