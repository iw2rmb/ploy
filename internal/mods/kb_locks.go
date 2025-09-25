package mods

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/internal/orchestration"
	"github.com/iw2rmb/ploy/internal/utils"
)

// Lock represents an acquired lock with metadata
type Lock struct {
	Key            string
	SessionID      string
	TTL            time.Duration
	Revision       uint64 // JetStream KV revision for CAS operations
	Backend        string // "consul" or "jetstream"
	Owner          string
	AcquiredAt     time.Time
	LeaseExpiresAt time.Time
	client         *consulapi.Client
}

// KBLockManager provides distributed locking for KB operations
type KBLockManager interface {
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, error)
	ReleaseLock(ctx context.Context, lock *Lock) error
	IsLocked(ctx context.Context, key string) (bool, error)
	TryWithLockRetry(ctx context.Context, key string, config *LockConfig, fn func() error) error
}

// NewKBLockManager creates a new KB lock manager using the configured backend.
// Uses JetStream if PLOY_USE_JETSTREAM_KV is truthy, otherwise falls back to Consul.
func NewKBLockManager(kv orchestration.KV) KBLockManager {
	if useJetstreamKV() {
		if mgr, err := NewJetstreamKBLockManager(); err != nil {
			log.Printf("mods: jetstream lock manager unavailable, falling back to Consul: %v", err)
		} else if mgr != nil {
			return mgr
		}
	}
	return NewConsulKBLockManager(kv)
}

// useJetstreamKV checks if JetStream KV should be used for locking
func useJetstreamKV() bool {
	value := strings.ToLower(strings.TrimSpace(utils.Getenv("PLOY_USE_JETSTREAM_KV", "")))
	if value == "" {
		// Fallback to JetStream by default; Consul requires explicit opt-out for emergencies.
		return true
	}
	switch value {
	case "0", "false", "off", "no", "consul", "legacy":
		return false
	case "1", "true", "on", "yes", "jetstream", "js":
		return true
	default:
		return true
	}
}

// ConsulKBLockManager implements KBLockManager using Consul KV
type ConsulKBLockManager struct {
	client *consulapi.Client
	kv     orchestration.KV
}

// NewConsulKBLockManager creates a new Consul-based lock manager
func NewConsulKBLockManager(kv orchestration.KV) *ConsulKBLockManager {
	// Get the consul client from the KV interface
	// We need to create our own client for session management
	cfg := consulapi.DefaultConfig()
	client, _ := consulapi.NewClient(cfg)

	return &ConsulKBLockManager{
		client: client,
		kv:     kv,
	}
}

// AcquireLock attempts to acquire a distributed lock with TTL
func (m *ConsulKBLockManager) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	// Create a session for the lock
	sessionOpts := &consulapi.SessionEntry{
		Name:      fmt.Sprintf("kb-lock-%s", key),
		TTL:       ttl.String(),
		Behavior:  consulapi.SessionBehaviorDelete,
		LockDelay: 1 * time.Second,
	}

	sessionID, _, err := m.client.Session().Create(sessionOpts, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	lockKey := m.buildLockKey(key)

	// Attempt to acquire the lock
	kvPair := &consulapi.KVPair{
		Key:     lockKey,
		Value:   []byte(sessionID),
		Session: sessionID,
	}

	acquired, _, err := m.client.KV().Acquire(kvPair, nil)
	if err != nil {
		// Clean up session if lock acquisition failed
		_, _ = m.client.Session().Destroy(sessionID, nil)
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	if !acquired {
		// Clean up session if lock was not acquired
		_, _ = m.client.Session().Destroy(sessionID, nil)
		return nil, fmt.Errorf("lock already held by another session")
	}

	return &Lock{
		Key:            lockKey,
		SessionID:      sessionID,
		TTL:            ttl,
		Backend:        "consul",
		Owner:          sessionID,
		AcquiredAt:     time.Now(),
		LeaseExpiresAt: time.Now().Add(ttl),
		client:         m.client,
	}, nil
}

// ReleaseLock releases a previously acquired lock
func (m *ConsulKBLockManager) ReleaseLock(ctx context.Context, lock *Lock) error {
	if lock == nil {
		return fmt.Errorf("cannot release nil lock")
	}

	// Release the lock by destroying the session
	_, err := m.client.Session().Destroy(lock.SessionID, nil)
	if err != nil {
		return fmt.Errorf("failed to destroy lock session: %w", err)
	}

	return nil
}

// IsLocked checks if a key is currently locked
func (m *ConsulKBLockManager) IsLocked(ctx context.Context, key string) (bool, error) {
	lockKey := m.buildLockKey(key)

	kvPair, _, err := m.client.KV().Get(lockKey, nil)
	if err != nil {
		return false, fmt.Errorf("failed to check lock status: %w", err)
	}

	return kvPair != nil && kvPair.Session != "", nil
}

// buildLockKey builds the full Consul KV key for a lock
func (m *ConsulKBLockManager) buildLockKey(key string) string {
	return path.Join("kb/locks", key)
}

// TryWithLock attempts to acquire a lock and execute a function with it
// This is a convenience method for common lock-execute-unlock patterns
func (m *ConsulKBLockManager) TryWithLock(ctx context.Context, key string, ttl time.Duration, fn func() error) error {
	lock, err := m.AcquireLock(ctx, key, ttl)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() { _ = m.ReleaseLock(ctx, lock) }()

	return fn()
}

// BuildSignatureLockKey builds a lock key for an error signature
func BuildSignatureLockKey(lang, signature string) string {
	return path.Join(lang, signature)
}

// AcquireSignatureLock is a convenience method for locking error signature operations
func (m *ConsulKBLockManager) AcquireSignatureLock(ctx context.Context, lang, signature string, ttl time.Duration) (*Lock, error) {
	key := BuildSignatureLockKey(lang, signature)
	return m.AcquireLock(ctx, key, ttl)
}

// LockConfig contains configuration for lock operations
type LockConfig struct {
	DefaultTTL    time.Duration
	MaxWaitTime   time.Duration
	RetryInterval time.Duration
	MaxRetries    int
}

// DefaultLockConfig returns reasonable defaults for KB operations
func DefaultLockConfig() *LockConfig {
	return &LockConfig{
		DefaultTTL:    5 * time.Second,
		MaxWaitTime:   10 * time.Second,
		RetryInterval: 100 * time.Millisecond,
		MaxRetries:    3,
	}
}

// TryWithLockRetry attempts to acquire a lock with retries
func (m *ConsulKBLockManager) TryWithLockRetry(ctx context.Context, key string, config *LockConfig, fn func() error) error {
	if config == nil {
		config = DefaultLockConfig()
	}

	var lastErr error
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		lock, err := m.AcquireLock(ctx, key, config.DefaultTTL)
		if err != nil {
			lastErr = err
			// Wait before retrying
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(config.RetryInterval):
				continue
			}
		}

		// Execute function with lock held
		err = fn()
		releaseErr := m.ReleaseLock(ctx, lock)
		if releaseErr != nil {
			// Best-effort: ignore release error; lock TTL will expire
			_ = releaseErr
		}

		if err == nil {
			return nil // Success
		}

		lastErr = err
	}

	return fmt.Errorf("failed after %d attempts, last error: %w", config.MaxRetries, lastErr)
}
