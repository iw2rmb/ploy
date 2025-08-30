package coordination

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
)

// LeaderElection provides distributed leader election using Consul
type LeaderElection struct {
	client       *api.Client
	sessionID    string
	key          string
	ttl          string
	isLeader     bool
	mu           sync.RWMutex
	stopOnce     sync.Once
	stopCh       chan struct{}
	leadershipCh chan bool
	callbacks    LeaderCallbacks
	logger       *log.Logger
}

// LeaderCallbacks defines callbacks for leadership changes
type LeaderCallbacks struct {
	OnStartedLeading func(ctx context.Context) error
	OnStoppedLeading func()
	OnNewLeader      func(identity string)
}

// NewLeaderElection creates a new leader election instance
func NewLeaderElection(consulAddr, key, ttl string, callbacks LeaderCallbacks) (*LeaderElection, error) {
	config := api.DefaultConfig()
	if consulAddr != "" {
		config.Address = consulAddr
	}

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	logger := log.New(os.Stdout, "[leader-election] ", log.LstdFlags|log.Lshortfile)

	return &LeaderElection{
		client:       client,
		key:          key,
		ttl:          ttl,
		stopCh:       make(chan struct{}),
		leadershipCh: make(chan bool, 1),
		callbacks:    callbacks,
		logger:       logger,
	}, nil
}

// Run starts the leader election process
func (le *LeaderElection) Run(ctx context.Context) error {
	// Create session with TTL
	session := le.client.Session()
	sessionOpts := &api.SessionEntry{
		Name:      fmt.Sprintf("ploy-api-%s", getHostname()),
		TTL:       le.ttl,
		Behavior:  api.SessionBehaviorDelete,
		LockDelay: 5 * time.Second,
	}

	sessionID, _, err := session.Create(sessionOpts, nil)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	le.sessionID = sessionID
	le.logger.Printf("Created session %s", sessionID)

	// Start session renewal
	go le.renewSession(ctx, session)

	// Start leader election loop
	go le.electionLoop(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	le.Stop()
	return nil
}

// electionLoop continuously attempts to acquire or maintain leadership
func (le *LeaderElection) electionLoop(ctx context.Context) {
	kv := le.client.KV()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-le.stopCh:
			return
		case <-ticker.C:
			le.attemptLeadership(ctx, kv)
		}
	}
}

// attemptLeadership tries to acquire or verify leadership
func (le *LeaderElection) attemptLeadership(ctx context.Context, kv *api.KV) {
	// Try to acquire leadership
	kvPair := &api.KVPair{
		Key:     le.key,
		Value:   []byte(getHostname()),
		Session: le.sessionID,
	}

	acquired, _, err := kv.Acquire(kvPair, nil)
	if err != nil {
		le.logger.Printf("Failed to acquire lock: %v", err)
		le.setLeader(false)
		return
	}

	if acquired && !le.IsLeader() {
		// We just became the leader
		le.logger.Printf("Acquired leadership")
		le.setLeader(true)

		if le.callbacks.OnStartedLeading != nil {
			go func() {
				if err := le.callbacks.OnStartedLeading(ctx); err != nil {
					le.logger.Printf("OnStartedLeading callback error: %v", err)
				}
			}()
		}
	} else if !acquired && le.IsLeader() {
		// We lost leadership
		le.logger.Printf("Lost leadership")
		le.setLeader(false)

		if le.callbacks.OnStoppedLeading != nil {
			go le.callbacks.OnStoppedLeading()
		}
	} else if !acquired {
		// Check who is the current leader
		kvPair, _, err := kv.Get(le.key, nil)
		if err == nil && kvPair != nil && kvPair.Session != "" {
			leader := string(kvPair.Value)
			if le.callbacks.OnNewLeader != nil {
				go le.callbacks.OnNewLeader(leader)
			}
		}
	}
}

// renewSession keeps the session alive
func (le *LeaderElection) renewSession(ctx context.Context, session *api.Session) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-le.stopCh:
			return
		case <-ticker.C:
			_, _, err := session.Renew(le.sessionID, nil)
			if err != nil {
				le.logger.Printf("Failed to renew session: %v", err)
				// Session might have expired, need to recreate
				le.handleSessionExpiry(ctx, session)
			}
		}
	}
}

// handleSessionExpiry handles session expiration
func (le *LeaderElection) handleSessionExpiry(ctx context.Context, session *api.Session) {
	le.setLeader(false)
	if le.callbacks.OnStoppedLeading != nil {
		go le.callbacks.OnStoppedLeading()
	}

	// Try to recreate session
	sessionOpts := &api.SessionEntry{
		Name:      fmt.Sprintf("ploy-api-%s", getHostname()),
		TTL:       le.ttl,
		Behavior:  api.SessionBehaviorDelete,
		LockDelay: 5 * time.Second,
	}

	sessionID, _, err := session.Create(sessionOpts, nil)
	if err != nil {
		le.logger.Printf("Failed to recreate session: %v", err)
		return
	}

	le.sessionID = sessionID
	le.logger.Printf("Recreated session %s", sessionID)
}

// IsLeader returns true if this instance is the current leader
func (le *LeaderElection) IsLeader() bool {
	le.mu.RLock()
	defer le.mu.RUnlock()
	return le.isLeader
}

// setLeader updates the leadership status
func (le *LeaderElection) setLeader(isLeader bool) {
	le.mu.Lock()
	defer le.mu.Unlock()
	le.isLeader = isLeader

	// Non-blocking send to leadership channel
	select {
	case le.leadershipCh <- isLeader:
	default:
	}
}

// LeadershipChannel returns a channel that receives leadership changes
func (le *LeaderElection) LeadershipChannel() <-chan bool {
	return le.leadershipCh
}

// Stop stops the leader election
func (le *LeaderElection) Stop() {
	le.stopOnce.Do(func() {
		close(le.stopCh)

		// Release leadership if we have it
		if le.IsLeader() {
			le.setLeader(false)
			if le.callbacks.OnStoppedLeading != nil {
				le.callbacks.OnStoppedLeading()
			}
		}

		// Destroy session
		if le.sessionID != "" {
			session := le.client.Session()
			_, err := session.Destroy(le.sessionID, nil)
			if err != nil {
				le.logger.Printf("Failed to destroy session: %v", err)
			}
		}

		close(le.leadershipCh)
	})
}

// getHostname returns the hostname for identification
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// CoordinationManager manages coordination tasks that require leadership
type CoordinationManager struct {
	leader    *LeaderElection
	ttlWorker *TTLCleanupWorker
	mu        sync.RWMutex
	logger    *log.Logger
	metrics   MetricsRecorder
}

// MetricsRecorder defines the interface for recording coordination metrics
type MetricsRecorder interface {
	SetLeaderStatus(isLeader bool)
	RecordLeadershipChange(changeType string)
}

// NewCoordinationManager creates a new coordination manager
func NewCoordinationManager(consulAddr string) (*CoordinationManager, error) {
	return NewCoordinationManagerWithMetrics(consulAddr, nil)
}

// NewCoordinationManagerWithMetrics creates a new coordination manager with metrics
func NewCoordinationManagerWithMetrics(consulAddr string, metrics MetricsRecorder) (*CoordinationManager, error) {
	logger := log.New(os.Stdout, "[coordination] ", log.LstdFlags|log.Lshortfile)

	cm := &CoordinationManager{
		logger:  logger,
		metrics: metrics,
	}

	// Define leader callbacks
	callbacks := LeaderCallbacks{
		OnStartedLeading: func(ctx context.Context) error {
			logger.Println("Started leading, initializing coordination tasks")
			if cm.metrics != nil {
				cm.metrics.SetLeaderStatus(true)
				cm.metrics.RecordLeadershipChange("gained")
			}
			return cm.startCoordinationTasks(ctx)
		},
		OnStoppedLeading: func() {
			logger.Println("Stopped leading, stopping coordination tasks")
			if cm.metrics != nil {
				cm.metrics.SetLeaderStatus(false)
				cm.metrics.RecordLeadershipChange("lost")
			}
			cm.stopCoordinationTasks()
		},
		OnNewLeader: func(identity string) {
			logger.Printf("New leader elected: %s", identity)
		},
	}

	// Create leader election
	leader, err := NewLeaderElection(
		consulAddr,
		"ploy/api/leader",
		"15s",
		callbacks,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create leader election: %w", err)
	}

	cm.leader = leader
	return cm, nil
}

// Run starts the coordination manager
func (cm *CoordinationManager) Run(ctx context.Context) error {
	return cm.leader.Run(ctx)
}

// IsLeader returns true if this instance is the leader
func (cm *CoordinationManager) IsLeader() bool {
	return cm.leader.IsLeader()
}

// startCoordinationTasks starts tasks that should only run on the leader
func (cm *CoordinationManager) startCoordinationTasks(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Start TTL cleanup worker
	cm.ttlWorker = NewTTLCleanupWorker()
	go cm.ttlWorker.Run(ctx)

	// Add other coordination tasks here as needed
	// - Periodic health checks
	// - Resource cleanup
	// - Certificate renewal coordination
	// - etc.

	return nil
}

// stopCoordinationTasks stops all coordination tasks
func (cm *CoordinationManager) stopCoordinationTasks() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.ttlWorker != nil {
		cm.ttlWorker.Stop()
		cm.ttlWorker = nil
	}

	// Stop other coordination tasks here
}

// Stop stops the coordination manager
func (cm *CoordinationManager) Stop() {
	defer func() {
		if r := recover(); r != nil {
			cm.logger.Printf("PANIC during coordination manager stop: %v", r)
		}
	}()
	cm.stopCoordinationTasks()
	cm.leader.Stop()
}
