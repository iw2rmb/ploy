package sshtransport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager maintains persistent SSH tunnels keyed by node identifier.
type Manager struct {
	mu sync.Mutex

	factory TunnelFactory
	cache   AssignmentCache
	logger  Logger
	cfg     Config
	localIP string
	minBack time.Duration
	maxBack time.Duration
	dialTO  time.Duration
	runner  CommandRunner

	nodes map[string]Node
	order []string

	tunnels map[string]*tunnelState
	nextIdx int
	closed  bool
}

// NewManager constructs a tunnel manager using the supplied configuration.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.ControlSocketDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("sshtransport: resolve home dir: %w", err)
		}
		cfg.ControlSocketDir = filepath.Join(home, ".ploy", "tunnels")
	}
	if err := os.MkdirAll(cfg.ControlSocketDir, 0o755); err != nil {
		return nil, fmt.Errorf("sshtransport: ensure control socket directory: %w", err)
	}

	local := strings.TrimSpace(cfg.LocalAddress)
	if local == "" {
		local = "127.0.0.1"
	}
	minBack := cfg.MinBackoff
	if minBack <= 0 {
		minBack = 500 * time.Millisecond
	}
	maxBack := cfg.MaxBackoff
	if maxBack <= 0 {
		maxBack = 30 * time.Second
	}
	if maxBack < minBack {
		maxBack = minBack
	}
	dialTO := cfg.DialTimeout
	if dialTO <= 0 {
		dialTO = 5 * time.Second
	}

	factory := cfg.Factory
	if factory == nil {
		factory = &sshFactory{
			controlSocketDir: cfg.ControlSocketDir,
			localBind:        local,
		}
	}

	cache := cfg.Cache
	if cache == nil {
		cache = noopCache{}
	}

	runner := cfg.CommandRunner
	if runner == nil {
		runner = defaultCommandRunner{}
	}

	return &Manager{
		factory: factory,
		cache:   cache,
		logger:  cfg.Logger,
		cfg:     cfg,
		localIP: local,
		minBack: minBack,
		maxBack: maxBack,
		dialTO:  dialTO,
		runner:  runner,
		nodes:   make(map[string]Node),
		order:   nil,
		tunnels: make(map[string]*tunnelState),
	}, nil
}

// SetNodes updates the node snapshot used for tunnel selection and persistence.
func (m *Manager) SetNodes(nodes []Node) error {
	normalised := make([]Node, 0, len(nodes))
	seen := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		node = normaliseNode(node)
		if node.ID == "" || node.Address == "" {
			continue
		}
		if _, ok := seen[node.ID]; ok {
			continue
		}
		seen[node.ID] = struct{}{}
		normalised = append(normalised, node)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("sshtransport: manager closed")
	}

	cur := m.nodes
	next := make(map[string]Node, len(normalised))
	order := make([]string, 0, len(normalised))
	for _, node := range normalised {
		next[node.ID] = node
		order = append(order, node.ID)
	}

	for id := range cur {
		if _, ok := next[id]; !ok {
			if t := m.tunnels[id]; t != nil && t.handle != nil {
				_ = t.handle.Close()
			}
			delete(m.tunnels, id)
		}
	}

	m.nodes = next
	m.order = order
	if m.nextIdx >= len(order) {
		m.nextIdx = 0
	}
	return m.cache.RememberNodes(normalised)
}

// HasTargets reports whether any tunnel targets are currently configured.
func (m *Manager) HasTargets() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.order) > 0
}

// Close tears down all active tunnels.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	for id, tunnel := range m.tunnels {
		if tunnel != nil && tunnel.handle != nil {
			_ = tunnel.handle.Close()
		}
		delete(m.tunnels, id)
	}
	return nil
}

// DialContext dials the target address, routing the connection through a node-specific tunnel.
func (m *Manager) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	candidates, err := m.selectCandidates(ctx)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, nodeID := range candidates {
		conn, err := m.dialThroughNode(ctx, network, nodeID)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if errors.Is(err, ErrBackoffActive) {
			continue
		}
		if netErr := (&net.OpError{}); errors.As(err, &netErr) {
			continue
		}
	}
	if lastErr == nil {
		lastErr = ErrNoTargets
	}
	return nil, lastErr
}

// selectCandidates derives the dial order for the current context.
func (m *Manager) selectCandidates(ctx context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.order) == 0 {
		return nil, ErrNoTargets
	}

	jobID, jobOK := JobFromContext(ctx)
	if jobOK {
		if nodeID, ok := m.cache.LookupJob(jobID); ok && nodeID != "" {
			return []string{nodeID}, nil
		}
	}
	if nodeID, ok := NodeFromContext(ctx); ok && nodeID != "" {
		return []string{nodeID}, nil
	}

	order := make([]string, 0, len(m.order))
	if m.nextIdx >= len(m.order) {
		m.nextIdx = 0
	}
	for i := 0; i < len(m.order); i++ {
		idx := (m.nextIdx + i) % len(m.order)
		order = append(order, m.order[idx])
	}
	m.nextIdx = (m.nextIdx + 1) % len(m.order)
	return order, nil
}

// dialThroughNode reuses or establishes a tunnel for the node then dials through it.
func (m *Manager) dialThroughNode(ctx context.Context, network, nodeID string) (net.Conn, error) {
	state, err := m.ensureTunnel(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: m.dialTO}
	conn, err := dialer.DialContext(ctx, network, state.localAddr)
	if err != nil {
		m.registerFailure(nodeID, err)
		return nil, err
	}
	state.lastActive = time.Now()
	return conn, nil
}
