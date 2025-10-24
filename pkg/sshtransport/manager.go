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

// Node describes the remote node reachable via SSH and the target API port.
type Node struct {
	ID           string
	Address      string
	SSHPort      int
	APIPort      int
	User         string
	IdentityFile string
}

// Config configures tunnel management for control-plane forwarding.
type Config struct {
	ControlSocketDir string
	LocalAddress     string
	DialTimeout      time.Duration
	MinBackoff       time.Duration
	MaxBackoff       time.Duration
	Factory          TunnelFactory
	Cache            AssignmentCache
	Logger           Logger
}

// AssignmentCache captures the persistence hooks Manager relies on to remember job/node mappings.
type AssignmentCache interface {
	RememberNodes([]Node) error
	RememberJob(jobID, nodeID string, at time.Time) error
	LookupJob(jobID string) (string, bool)
	RemoveJob(jobID string) error
}

// TunnelFactory provisions SSH tunnels to remote nodes.
type TunnelFactory interface {
	Activate(ctx context.Context, node Node, localAddr string) (TunnelHandle, error)
}

// TunnelHandle exposes lifecycle state for an active tunnel.
type TunnelHandle interface {
	LocalAddress() string
	Wait() <-chan error
	Close() error
}

// Logger is the subset of slog.Logger used by the manager.
type Logger interface {
	Debug(msg string, args ...any)
	Error(msg string, args ...any)
}

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

	nodes map[string]Node
	order []string

	tunnels map[string]*tunnelState
	nextIdx int
	closed  bool
}

type tunnelState struct {
	node       Node
	localAddr  string
	handle     TunnelHandle
	wait       <-chan error
	failures   int
	nextRetry  time.Time
	lastErr    error
	lastActive time.Time
}

// ErrNoTargets indicates that no nodes are available for tunnelling.
var ErrNoTargets = errors.New("sshtransport: no tunnel targets configured")

// ErrBackoffActive indicates the caller should retry later, typically falling back to another node.
var ErrBackoffActive = errors.New("sshtransport: backoff active for node")

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

	return &Manager{
		factory: factory,
		cache:   cache,
		logger:  cfg.Logger,
		cfg:     cfg,
		localIP: local,
		minBack: minBack,
		maxBack: maxBack,
		dialTO:  dialTO,
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

	// Close tunnels whose nodes disappeared.
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

	// Build ordered slice cycling from nextIdx.
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

func (m *Manager) ensureTunnel(ctx context.Context, nodeID string) (*tunnelState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, errors.New("sshtransport: manager closed")
	}
	node, ok := m.nodes[nodeID]
	if !ok {
		return nil, fmt.Errorf("sshtransport: unknown node %q", nodeID)
	}
	state, ok := m.tunnels[nodeID]
	if !ok {
		state = &tunnelState{node: node}
		m.tunnels[nodeID] = state
	} else {
		state.node = node
	}
	if state.handle != nil {
		select {
		case <-state.wait:
			// Tunnel completed, fall through to re-establish.
			state.handle = nil
		default:
			return state, nil
		}
	}
	now := time.Now()
	if !state.nextRetry.IsZero() && now.Before(state.nextRetry) {
		return nil, ErrBackoffActive
	}

	localAddr, err := m.allocateLocal()
	if err != nil {
		state.lastErr = err
		state.failures++
		state.nextRetry = now.Add(m.backoffDuration(state.failures))
		return nil, err
	}
	handle, err := m.factory.Activate(ctx, node, localAddr)
	if err != nil {
		state.failures++
		state.lastErr = err
		state.nextRetry = now.Add(m.backoffDuration(state.failures))
		return nil, err
	}
	state.localAddr = handle.LocalAddress()
	state.handle = handle
	state.failures = 0
	state.lastErr = nil
	state.nextRetry = time.Time{}
	state.lastActive = now
	waitCh := handle.Wait()
	state.wait = waitCh
	go m.observe(node.ID, state, waitCh)
	return state, nil
}

func (m *Manager) observe(nodeID string, state *tunnelState, wait <-chan error) {
	err, ok := <-wait
	if !ok {
		err = nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tunnels[nodeID]
	if !ok || current != state {
		return
	}
	state.handle = nil
	state.wait = nil
	state.lastErr = err
	state.failures++
	state.nextRetry = time.Now().Add(m.backoffDuration(state.failures))
}

func (m *Manager) registerFailure(nodeID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.tunnels[nodeID]
	if !ok {
		return
	}
	state.lastErr = err
	state.failures++
	state.nextRetry = time.Now().Add(m.backoffDuration(state.failures))
	if state.handle != nil {
		_ = state.handle.Close()
		state.handle = nil
	}
}

func (m *Manager) backoffDuration(failures int) time.Duration {
	if failures <= 0 {
		return m.minBack
	}
	delay := m.minBack << (failures - 1)
	if delay > m.maxBack {
		delay = m.maxBack
	}
	return delay
}

func (m *Manager) allocateLocal() (string, error) {
	ln, err := net.Listen("tcp", net.JoinHostPort(m.localIP, "0"))
	if err != nil {
		return "", fmt.Errorf("sshtransport: allocate local listener: %w", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		return "", err
	}
	return addr, nil
}

func normaliseNode(node Node) Node {
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

type noopCache struct{}

func (noopCache) RememberNodes([]Node) error                  { return nil }
func (noopCache) RememberJob(string, string, time.Time) error { return nil }
func (noopCache) LookupJob(string) (string, bool)             { return "", false }
func (noopCache) RemoveJob(string) error                      { return nil }
