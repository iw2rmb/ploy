package sshtransport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// tunnelState tracks the lifecycle of a single node tunnel.
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

// ensureTunnel lazily establishes or reuses a tunnel for the provided node ID.
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

// observe watches a tunnel's completion signal and schedules retries.
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

// registerFailure records a dial failure and increments backoff.
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

// backoffDuration computes the retry delay based on the failure count.
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

// allocateLocal reserves a localhost port for a tunnel listener.
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
