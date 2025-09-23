package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/nats-io/nats.go"
)

// JetstreamKBLockManager implements KBLockManager using NATS JetStream KV
type JetstreamKBLockManager struct {
	conn   *nats.Conn
	bucket nats.KeyValue
	js     nats.JetStreamContext
}

// JetstreamLockData represents the lock data stored in JetStream KV
type JetstreamLockData struct {
	SessionID  string    `json:"session_id"`
	Holder     string    `json:"holder"`
	TTL        int64     `json:"ttl_seconds"`
	AcquiredAt time.Time `json:"acquired_at"`
}

// NewJetstreamKBLockManager creates a new JetStream-based lock manager
func NewJetstreamKBLockManager() (*JetstreamKBLockManager, error) {
	url := utils.Getenv("PLOY_JETSTREAM_URL", "")
	if url == "" {
		url = utils.Getenv("NATS_ADDR", nats.DefaultURL)
	}
	if url == "" {
		return nil, fmt.Errorf("jetstream url not configured")
	}

	opts := []nats.Option{nats.Name("ploy-jetstream-kb-locks")}
	if creds := utils.Getenv("PLOY_JETSTREAM_CREDS", ""); creds != "" {
		opts = append(opts, nats.UserCredentials(creds))
	}
	user := utils.Getenv("PLOY_JETSTREAM_USER", "")
	if user != "" {
		opts = append(opts, nats.UserInfo(user, utils.Getenv("PLOY_JETSTREAM_PASSWORD", "")))
	}

	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to get JetStream context: %w", err)
	}

	bucketName := utils.Getenv("PLOY_JETSTREAM_KV_BUCKET", "ploy_kv")
	bucket, err := js.KeyValue(bucketName)
	if err == nats.ErrBucketNotFound {
		bucket, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: bucketName})
	}
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to get/create KV bucket: %w", err)
	}

	return &JetstreamKBLockManager{
		conn:   conn,
		bucket: bucket,
		js:     js,
	}, nil
}

// AcquireLock attempts to acquire a distributed lock using JetStream KV CAS operations
func (m *JetstreamKBLockManager) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	lockKey := m.buildLockKey(key)

	// Generate a unique session ID for this lock attempt
	sessionID := fmt.Sprintf("kb-lock-%d-%s", time.Now().UnixNano(), key)

	lockData := &JetstreamLockData{
		SessionID:  sessionID,
		Holder:     utils.Getenv("HOSTNAME", "unknown"),
		TTL:        int64(ttl.Seconds()),
		AcquiredAt: time.Now(),
	}

	lockValue, err := json.Marshal(lockData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lock data: %w", err)
	}

	// Try to create the lock (CAS operation with revision 0)
	revision, err := m.bucket.Create(lockKey, lockValue)
	if err != nil {
		if err == nats.ErrKeyExists {
			// Check if the existing lock has expired
			existing, err := m.bucket.Get(lockKey)
			if err != nil {
				return nil, fmt.Errorf("lock already held by another session")
			}

			var existingData JetstreamLockData
			if err := json.Unmarshal(existing.Value(), &existingData); err != nil {
				return nil, fmt.Errorf("lock already held by another session")
			}

			// Check if the lock has expired
			if time.Since(existingData.AcquiredAt) < time.Duration(existingData.TTL)*time.Second {
				return nil, fmt.Errorf("lock already held by another session")
			}

			// Try to update the expired lock
			revision, err = m.bucket.Update(lockKey, lockValue, existing.Revision())
			if err != nil {
				return nil, fmt.Errorf("failed to acquire expired lock: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create lock: %w", err)
		}
	}

	// Publish lock acquisition event
	if err := m.publishLockEvent(ctx, "acquired", key, lockData); err != nil {
		// Log but don't fail the lock acquisition
		fmt.Printf("Warning: failed to publish lock acquisition event: %v\n", err)
	}

	return &Lock{
		Key:       lockKey,
		SessionID: sessionID,
		TTL:       ttl,
		Revision:  revision,
		Backend:   "jetstream",
	}, nil
}

// ReleaseLock releases a previously acquired lock
func (m *JetstreamKBLockManager) ReleaseLock(ctx context.Context, lock *Lock) error {
	if lock == nil {
		return fmt.Errorf("cannot release nil lock")
	}

	if lock.Backend != "jetstream" {
		return fmt.Errorf("cannot release non-jetstream lock with jetstream manager")
	}

	// Get the current lock to verify ownership
	existing, err := m.bucket.Get(lock.Key)
	if err != nil {
		if err == nats.ErrKeyNotFound {
			// Lock already released or expired
			return nil
		}
		return fmt.Errorf("failed to get lock for release: %w", err)
	}

	var existingData JetstreamLockData
	if err := json.Unmarshal(existing.Value(), &existingData); err != nil {
		return fmt.Errorf("failed to unmarshal existing lock data: %w", err)
	}

	// Verify we own this lock
	if existingData.SessionID != lock.SessionID {
		return fmt.Errorf("cannot release lock owned by different session")
	}

	// Delete the lock using the revision to ensure we still own it
	err = m.bucket.Delete(lock.Key)
	if err != nil && err != nats.ErrKeyNotFound {
		return fmt.Errorf("failed to delete lock: %w", err)
	}

	// Publish lock release event
	if err := m.publishLockEvent(ctx, "released", lock.Key, &existingData); err != nil {
		// Log but don't fail the lock release
		fmt.Printf("Warning: failed to publish lock release event: %v\n", err)
	}

	return nil
}

// IsLocked checks if a key is currently locked
func (m *JetstreamKBLockManager) IsLocked(ctx context.Context, key string) (bool, error) {
	lockKey := m.buildLockKey(key)

	existing, err := m.bucket.Get(lockKey)
	if err != nil {
		if err == nats.ErrKeyNotFound {
			return false, nil
		}
		return false, fmt.Errorf("failed to check lock status: %w", err)
	}

	var lockData JetstreamLockData
	if err := json.Unmarshal(existing.Value(), &lockData); err != nil {
		// If we can't unmarshal, assume it's locked
		return true, nil
	}

	// Check if the lock has expired
	if time.Since(lockData.AcquiredAt) >= time.Duration(lockData.TTL)*time.Second {
		// Lock has expired, clean it up
		_ = m.bucket.Delete(lockKey)
		return false, nil
	}

	return true, nil
}

// TryWithLockRetry attempts to acquire a lock with retries, handling JetStream-specific errors
func (m *JetstreamKBLockManager) TryWithLockRetry(ctx context.Context, key string, config *LockConfig, fn func() error) error {
	if config == nil {
		config = DefaultLockConfig()
	}

	var lastErr error
	for attempt := 0; attempt < config.MaxRetries; attempt++ {
		// Add exponential backoff with jitter to reduce thundering herd
		if attempt > 0 {
			delay := time.Duration(attempt) * config.RetryInterval
			jitter := time.Duration(float64(delay) * 0.1) // 10% jitter
			totalDelay := delay + jitter
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(totalDelay):
			}
		}

		lock, err := m.AcquireLock(ctx, key, config.DefaultTTL)
		if err != nil {
			lastErr = err
			// Check for JetStream-specific contention signals
			if err == nats.ErrKeyExists || containsString(err.Error(), "lock already held") {
				// Continue to next retry on contention
				continue
			}
			// For other errors, still retry
			continue
		}

		// Execute function with lock held
		err = fn()
		releaseErr := m.ReleaseLock(ctx, lock)
		if releaseErr != nil {
			// Best-effort: ignore release error; lock TTL will expire
			fmt.Printf("Warning: failed to release lock %s: %v\n", lock.Key, releaseErr)
		}

		if err == nil {
			return nil // Success
		}

		lastErr = err
	}

	return fmt.Errorf("failed after %d attempts, last error: %w", config.MaxRetries, lastErr)
}

// buildLockKey builds the full JetStream KV key for a lock
func (m *JetstreamKBLockManager) buildLockKey(key string) string {
	return path.Join("kb/locks", key)
}

// publishLockEvent publishes a lock state change event
func (m *JetstreamKBLockManager) publishLockEvent(ctx context.Context, event, key string, lockData *JetstreamLockData) error {
	// Remove the "kb/locks/" prefix from the key for cleaner event subjects
	cleanKey := key
	if strings.HasPrefix(key, "kb/locks/") {
		cleanKey = strings.TrimPrefix(key, "kb/locks/")
	}
	subject := fmt.Sprintf("kb.lock.%s.%s", event, cleanKey)

	eventData := map[string]interface{}{
		"event":       event,
		"key":         key,
		"session_id":  lockData.SessionID,
		"holder":      lockData.Holder,
		"ttl":         lockData.TTL,
		"acquired_at": lockData.AcquiredAt,
		"timestamp":   time.Now(),
	}

	eventBytes, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Use core NATS for event publishing (no need for JetStream persistence for events)
	err = m.conn.Publish(subject, eventBytes)
	return err
}

// Close closes the JetStream connection
func (m *JetstreamKBLockManager) Close() error {
	if m.conn != nil {
		m.conn.Close()
	}
	return nil
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
