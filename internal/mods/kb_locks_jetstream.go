package mods

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/nats-io/nats.go"
)

const (
	kbLockBucketEnv          = "MODS_KB_LOCK_BUCKET"
	kbLockEventReplicasEnv   = "MODS_KB_LOCK_EVENT_REPLICAS"
	defaultKBLockBucket      = "mods_kb_locks"
	kbLockEventStreamName    = "mods_kb_lock_events"
	kbLockEventSubjectPrefix = "mods.kb.lock"
	kbLockKeyPrefix          = "writers"
)

var (
	ErrLockHeld = errors.New("mods: kb lock already held")
)

// JetstreamKBLockManager implements KBLockManager using NATS JetStream KV
// for optimistic CAS locking and subject-based notifications.
type JetstreamKBLockManager struct {
	conn   *nats.Conn
	bucket nats.KeyValue
	js     nats.JetStreamContext
}

// JetstreamLockRecord represents the lock metadata stored in JetStream KV.
type JetstreamLockRecord struct {
	KBID           string    `json:"kb_id"`
	Owner          string    `json:"owner"`
	OwnerInstance  string    `json:"owner_instance,omitempty"`
	LeaseSeconds   int64     `json:"lease_seconds"`
	AcquiredAt     time.Time `json:"acquired_at"`
	LeaseExpiresAt time.Time `json:"lease_expires_at"`
}

func (r JetstreamLockRecord) expired(now time.Time) bool {
	if r.LeaseExpiresAt.IsZero() {
		if r.LeaseSeconds <= 0 {
			return false
		}
		return r.AcquiredAt.Add(time.Duration(r.LeaseSeconds) * time.Second).Before(now)
	}
	return now.After(r.LeaseExpiresAt)
}

// NewJetstreamKBLockManager creates a new JetStream-based lock manager.
func NewJetstreamKBLockManager() (*JetstreamKBLockManager, error) {
	url := strings.TrimSpace(utils.Getenv("PLOY_JETSTREAM_URL", ""))
	if url == "" {
		url = strings.TrimSpace(ResolveInfraFromEnv().JetStreamURL)
	}
	if url == "" {
		url = strings.TrimSpace(utils.Getenv("NATS_ADDR", nats.DefaultURL))
	}
	if url == "" {
		return nil, fmt.Errorf("jetstream url not configured")
	}

	opts := []nats.Option{nats.Name("ploy-mods-kb-locks")}
	if creds := strings.TrimSpace(utils.Getenv("PLOY_JETSTREAM_CREDS", "")); creds != "" {
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
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	bucketName := utils.Getenv(kbLockBucketEnv, defaultKBLockBucket)
	bucket, err := js.KeyValue(bucketName)
	if err == nats.ErrBucketNotFound {
		bucket, err = js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:      bucketName,
			Description: "Mods KB lock leases",
			History:     1,
			TTL:         10 * time.Minute,
			Replicas:    1,
		})
	}
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to get/create lock bucket %q: %w", bucketName, err)
	}

	if err := ensureLockEventStream(js); err != nil {
		conn.Close()
		return nil, err
	}

	return &JetstreamKBLockManager{conn: conn, bucket: bucket, js: js}, nil
}

func ensureLockEventStream(js nats.JetStreamContext) error {
	if _, err := js.StreamInfo(kbLockEventStreamName); err == nil {
		return nil
	}

	replicas := 1
	if raw := utils.Getenv(kbLockEventReplicasEnv, ""); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			replicas = parsed
		}
	}

	cfg := &nats.StreamConfig{
		Name:              kbLockEventStreamName,
		Description:       "Mods KB lock lifecycle events",
		Subjects:          []string{kbLockEventSubjectPrefix + ".>"},
		Retention:         nats.LimitsPolicy,
		MaxMsgsPerSubject: 128,
		MaxAge:            72 * time.Hour,
		Replicas:          replicas,
		Storage:           nats.FileStorage,
	}

	if _, err := js.AddStream(cfg); err != nil && !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
		return fmt.Errorf("failed to create lock event stream: %w", err)
	}
	return nil
}

// AcquireLock attempts to acquire a distributed lock using JetStream KV CAS operations.
func (m *JetstreamKBLockManager) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	normalized := normalizeLockKey(key)
	lockKey := m.buildLockKey(normalized)
	owner := lockOwner()
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	record := JetstreamLockRecord{
		KBID:           normalized,
		Owner:          owner,
		LeaseSeconds:   int64(ttl.Round(time.Second) / time.Second),
		AcquiredAt:     now,
		LeaseExpiresAt: expiresAt,
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lock record: %w", err)
	}

	revision, err := m.bucket.Create(lockKey, payload)
	if err != nil {
		if !errors.Is(err, nats.ErrKeyExists) {
			return nil, fmt.Errorf("failed to create lock entry: %w", err)
		}

		entry, getErr := m.bucket.Get(lockKey)
		if getErr != nil {
			return nil, fmt.Errorf("failed to read existing lock: %w", getErr)
		}

		existing, parseErr := parseLockRecord(entry)
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse existing lock: %w", parseErr)
		}

		if !existing.expired(now) {
			return nil, fmt.Errorf("%w: %s held by %s", ErrLockHeld, normalized, existing.Owner)
		}

		// Publish expiration notification before attempting CAS takeover.
		_ = m.publishLockEvent(ctx, "expired", normalized, existing, entry.Revision())

		revision, err = m.bucket.Update(lockKey, payload, entry.Revision())
		if err != nil {
			if errors.Is(err, nats.ErrKeyExists) {
				return nil, fmt.Errorf("%w: %s", ErrLockHeld, normalized)
			}
			return nil, fmt.Errorf("failed to acquire expired lock: %w", err)
		}
	}

	lock := &Lock{
		Key:            lockKey,
		TTL:            ttl,
		Revision:       revision,
		Backend:        "jetstream",
		Owner:          owner,
		AcquiredAt:     now,
		LeaseExpiresAt: expiresAt,
	}

	if err := m.publishLockEvent(ctx, "acquired", normalized, record, revision); err != nil {
		fmt.Printf("Warning: failed to publish KB lock acquired event for %s: %v\n", normalized, err)
	}

	return lock, nil
}

// ReleaseLock releases a previously acquired lock.
func (m *JetstreamKBLockManager) ReleaseLock(ctx context.Context, lock *Lock) error {
	if lock == nil {
		return fmt.Errorf("cannot release nil lock")
	}
	if lock.Backend != "jetstream" {
		return fmt.Errorf("cannot release non-jetstream lock with jetstream manager")
	}

	entry, err := m.bucket.Get(lock.Key)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("failed to load lock state: %w", err)
	}

	record, err := parseLockRecord(entry)
	if err != nil {
		return fmt.Errorf("failed to parse stored lock: %w", err)
	}

	if lock.Owner != "" && record.Owner != lock.Owner {
		return fmt.Errorf("lock ownership mismatch: stored=%s requested=%s", record.Owner, lock.Owner)
	}

	if lock.Revision != 0 && entry.Revision() != lock.Revision {
		return fmt.Errorf("lock revision mismatch: stored=%d requested=%d", entry.Revision(), lock.Revision)
	}

	if err := m.bucket.Delete(lock.Key, nats.LastRevision(entry.Revision())); err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("failed to delete lock revision: %w", err)
	}

	if err := m.publishLockEvent(ctx, "released", record.KBID, record, lock.Revision); err != nil {
		fmt.Printf("Warning: failed to publish KB lock released event for %s: %v\n", record.KBID, err)
		return nil
	}

	return nil
}

// IsLocked checks if a key is currently locked.
func (m *JetstreamKBLockManager) IsLocked(ctx context.Context, key string) (bool, error) {
	normalized := normalizeLockKey(key)
	lockKey := m.buildLockKey(normalized)

	entry, err := m.bucket.Get(lockKey)
	if err != nil {
		if errors.Is(err, nats.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to read lock state: %w", err)
	}

	record, err := parseLockRecord(entry)
	if err != nil {
		return false, fmt.Errorf("failed to parse lock record: %w", err)
	}

	if record.expired(time.Now().UTC()) {
		_ = m.bucket.Delete(lockKey, nats.LastRevision(entry.Revision()))
		return false, nil
	}

	return true, nil
}

// TryWithLockRetry attempts to acquire a lock with retries, handling JetStream-specific errors.
func (m *JetstreamKBLockManager) TryWithLockRetry(ctx context.Context, key string, config *LockConfig, fn func() error) error {
	if config == nil {
		config = DefaultLockConfig()
	}

	attempts := 0
	lastErr := error(nil)
	start := time.Now()
	deadline := time.Time{}
	if config.MaxWaitTime > 0 {
		deadline = start.Add(config.MaxWaitTime)
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		lock, err := m.AcquireLock(ctx, key, config.DefaultTTL)
		if err == nil {
			runErr := fn()
			releaseErr := m.ReleaseLock(ctx, lock)
			if releaseErr != nil {
				fmt.Printf("Warning: failed to release lock %s: %v\n", lock.Key, releaseErr)
			}
			if runErr != nil {
				return runErr
			}
			return nil
		}

		lastErr = err
		attempts++

		if deadlineReached(deadline) {
			break
		}
		if config.MaxRetries > 0 && attempts >= config.MaxRetries {
			break
		}

		wait := backoffWithJitter(config.RetryInterval, attempts, rng)
		if deadlineReachedWithDelay(deadline, wait) {
			break
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("lock acquisition failed")
	}
	return fmt.Errorf("failed to acquire KB lock for %s after %d attempts: %w", key, attempts, lastErr)
}

func deadlineReached(deadline time.Time) bool {
	return !deadline.IsZero() && time.Now().After(deadline)
}

func deadlineReachedWithDelay(deadline time.Time, delay time.Duration) bool {
	return !deadline.IsZero() && time.Now().Add(delay).After(deadline)
}

func backoffWithJitter(base time.Duration, attempt int, rng *rand.Rand) time.Duration {
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	factor := time.Duration(1) << uint(min(attempt, 6))
	wait := base * factor
	jitter := time.Duration(rng.Int63n(int64(wait/3 + time.Millisecond)))
	return wait + jitter
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// buildLockKey builds the JetStream KV key for a lock.
func (m *JetstreamKBLockManager) buildLockKey(key string) string {
	cleaned := normalizeLockKey(key)
	return path.Join(kbLockKeyPrefix, cleaned)
}

func normalizeLockKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, kbLockKeyPrefix+"/")

	segments := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '/' || r == '\\'
	})

	var cleaned []string
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		cleaned = append(cleaned, seg)
	}
	if len(cleaned) == 0 {
		return "default"
	}
	return strings.Join(cleaned, "/")
}

func lockOwner() string {
	if alloc := utils.Getenv("NOMAD_ALLOC_ID", ""); alloc != "" {
		return alloc
	}
	if host := utils.Getenv("HOSTNAME", ""); host != "" {
		return host
	}
	return "unknown"
}

func parseLockRecord(entry nats.KeyValueEntry) (JetstreamLockRecord, error) {
	var record JetstreamLockRecord
	if err := json.Unmarshal(entry.Value(), &record); err != nil {
		return record, err
	}
	if record.KBID == "" {
		record.KBID = normalizeLockKey(strings.TrimPrefix(entry.Key(), kbLockKeyPrefix+"/"))
	}
	if record.AcquiredAt.IsZero() {
		record.AcquiredAt = entry.Created().UTC()
	}
	if record.LeaseExpiresAt.IsZero() && record.LeaseSeconds > 0 {
		record.LeaseExpiresAt = record.AcquiredAt.Add(time.Duration(record.LeaseSeconds) * time.Second)
	}
	return record, nil
}

func (m *JetstreamKBLockManager) publishLockEvent(ctx context.Context, event, kbID string, record JetstreamLockRecord, revision uint64) error {
	subject := fmt.Sprintf("%s.%s.%s", kbLockEventSubjectPrefix, event, sanitizeSubjectToken(kbID))
	payload := map[string]interface{}{
		"event":            event,
		"kb_id":            kbID,
		"owner":            record.Owner,
		"owner_instance":   record.OwnerInstance,
		"lease_expires_at": record.LeaseExpiresAt.UTC().Format(time.RFC3339Nano),
		"lease_seconds":    record.LeaseSeconds,
		"revision":         revision,
		"timestamp":        time.Now().UTC().Format(time.RFC3339Nano),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal lock event payload: %w", err)
	}

	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	msg.Header.Set("Nats-Msg-Id", fmt.Sprintf("%s-%s-%d", event, kbID, revision))

	if _, err := m.js.PublishMsg(msg); err != nil {
		return err
	}

	return nil
}

func sanitizeSubjectToken(token string) string {
	token = strings.ReplaceAll(token, " ", "_")
	token = strings.ReplaceAll(token, "*", "-")
	token = strings.ReplaceAll(token, ">", "-")
	if token == "" {
		return "default"
	}
	return token
}

// Close closes the JetStream connection.
func (m *JetstreamKBLockManager) Close() error {
	if m.conn != nil {
		m.conn.Close()
	}
	return nil
}
