package sshtransport

import (
	"context"
	"errors"
	"strings"
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
	CommandRunner    CommandRunner
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

// ErrNoTargets indicates that no nodes are available for tunnelling.
var ErrNoTargets = errors.New("sshtransport: no tunnel targets configured")

// ErrBackoffActive indicates the caller should retry later, typically falling back to another node.
var ErrBackoffActive = errors.New("sshtransport: backoff active for node")

// normaliseNode applies default ports and trims whitespace on the provided node.
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

// RememberNodes implements AssignmentCache by discarding the snapshot.
func (noopCache) RememberNodes([]Node) error { return nil }

// RememberJob implements AssignmentCache by ignoring job/node mappings.
func (noopCache) RememberJob(string, string, time.Time) error { return nil }

// LookupJob implements AssignmentCache with no stored job lookups.
func (noopCache) LookupJob(string) (string, bool) { return "", false }

// RemoveJob implements AssignmentCache by ignoring removal requests.
func (noopCache) RemoveJob(string) error { return nil }
