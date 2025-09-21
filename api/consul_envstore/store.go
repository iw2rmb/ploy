package consul_envstore

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
)

// CacheEntry represents a cached value with expiration
type CacheEntry struct {
	Value     interface{}
	ExpiresAt time.Time
}

// SimpleCache provides a thread-safe in-memory cache with TTL
type SimpleCache struct {
	entries map[string]*CacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	hits    int64
	misses  int64
}

// NewSimpleCache creates a new cache with the specified TTL
func NewSimpleCache(ttl time.Duration) *SimpleCache {
	cache := &SimpleCache{
		entries: make(map[string]*CacheEntry),
		ttl:     ttl,
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a value from the cache
func (c *SimpleCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return nil, false
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		delete(c.entries, key)
		c.misses++
		return nil, false
	}

	c.hits++
	return entry.Value, true
}

// Set stores a value in the cache with TTL
func (c *SimpleCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &CacheEntry{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes a key from the cache
func (c *SimpleCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

// Clear removes all entries from the cache
func (c *SimpleCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
}

// cleanup periodically removes expired entries
func (c *SimpleCache) cleanup() {
	ticker := time.NewTicker(c.ttl / 2) // Cleanup at half the TTL interval
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.ExpiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

// CacheStats represents cache statistics
type CacheStats struct {
	Size    int
	Hits    int64
	Misses  int64
	HitRate float64
}

// GetCacheStats returns cache statistics
func (c *SimpleCache) GetCacheStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Size:    len(c.entries),
		Hits:    c.hits,
		Misses:  c.misses,
		HitRate: hitRate,
	}
}

// consulKV abstracts the subset of the Consul KV API used by the env store so tests
// can inject a fake implementation without requiring a Consul agent.
type consulKV interface {
	Get(key string, opts *api.QueryOptions) (*api.KVPair, *api.QueryMeta, error)
	Put(pair *api.KVPair, opts *api.WriteOptions) (*api.WriteMeta, error)
	Delete(key string, opts *api.WriteOptions) (*api.WriteMeta, error)
}

// SecondaryKV represents an optional shadow key-value store (e.g., JetStream) that
// receives dual writes when enabled.
type SecondaryKV interface {
	Put(key string, value []byte) error
	Delete(key string) error
}

// MetricsRecorder captures dual-write metrics for observability. Implementations
// may emit Prometheus metrics or act as no-ops in tests.
type MetricsRecorder interface {
	RecordEnvStoreOperation(target, operation, status string, duration time.Duration)
}

// Option configures the ConsulEnvStore during construction.
type Option func(*ConsulEnvStore)

const (
	metricsTargetConsul    = "consul"
	metricsTargetJetStream = "jetstream"

	operationSet    = "set"
	operationDelete = "delete"
)

type noopMetrics struct{}

func (noopMetrics) RecordEnvStoreOperation(string, string, string, time.Duration) {}

// WithKV injects a custom Consul KV client implementation (primarily used in tests).
func WithKV(kv consulKV) Option {
	return func(store *ConsulEnvStore) {
		store.kv = kv
	}
}

// WithSecondary configures an optional secondary key-value store for dual writes.
func WithSecondary(secondary SecondaryKV) Option {
	return func(store *ConsulEnvStore) {
		store.secondary = secondary
		if secondary != nil {
			store.dualWrite = true
		}
	}
}

// WithMetrics wires a metrics recorder used to emit success/failure telemetry.
func WithMetrics(metrics MetricsRecorder) Option {
	return func(store *ConsulEnvStore) {
		store.metrics = metrics
	}
}

type ConsulEnvStore struct {
	client    *api.Client
	keyPrefix string
	mu        sync.RWMutex
	cache     *SimpleCache
	batchSize int
	kv        consulKV
	secondary SecondaryKV
	metrics   MetricsRecorder
	dualWrite bool
}

// Ensure ConsulEnvStore implements the EnvStoreInterface
var _ envstore.EnvStoreInterface = (*ConsulEnvStore)(nil)

func New(consulAddr, keyPrefix string, opts ...Option) (*ConsulEnvStore, error) {
	config := api.DefaultConfig()
	if consulAddr != "" {
		config.Address = consulAddr
	}

	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	if keyPrefix == "" {
		keyPrefix = "ploy/apps"
	}

	// Create cache with 5-minute TTL for environment variables
	cache := NewSimpleCache(5 * time.Minute)

	kv := client.KV()
	store := &ConsulEnvStore{
		client:    client,
		keyPrefix: keyPrefix,
		cache:     cache,
		batchSize: 10,
		kv:        kv,
	}

	for _, opt := range opts {
		opt(store)
	}

	if store.kv == nil {
		store.kv = kv
	}

	if store.metrics == nil {
		store.metrics = noopMetrics{}
	}

	if store.secondary != nil {
		store.dualWrite = true
	}

	return store, nil
}

func (s *ConsulEnvStore) appEnvKey(app string) string {
	return fmt.Sprintf("%s/%s/env", s.keyPrefix, app)
}

func (s *ConsulEnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("app:%s:env", app)
	if cached, found := s.cache.Get(cacheKey); found {
		if envVars, ok := cached.(envstore.AppEnvVars); ok {
			return envVars, nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.appEnvKey(app)

	// Use direct client
	pair, _, err := s.kv.Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get from Consul: %w", err)
	}

	if pair == nil {
		return envstore.AppEnvVars{}, nil
	}

	var envVars envstore.AppEnvVars
	if err := json.Unmarshal(pair.Value, &envVars); err != nil {
		return nil, fmt.Errorf("failed to unmarshal environment variables: %w", err)
	}

	// Cache the result
	s.cache.Set(cacheKey, envVars)
	log.Printf("[ConsulEnvStore] Retrieved and cached %d environment variables for app %s", len(envVars), app)
	return envVars, nil
}

func (s *ConsulEnvStore) Set(app, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Invalidate cache when setting
	cacheKey := fmt.Sprintf("app:%s:env", app)
	s.cache.Delete(cacheKey)

	// Get current env vars
	envVars, err := s.getUnsafe(app)
	if err != nil {
		return err
	}

	if envVars == nil {
		envVars = make(envstore.AppEnvVars)
	}

	envVars[key] = value
	return s.saveUnsafe(app, envVars, operationSet)
}

func (s *ConsulEnvStore) SetAll(app string, envVars envstore.AppEnvVars) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Invalidate cache when setting
	cacheKey := fmt.Sprintf("app:%s:env", app)
	s.cache.Delete(cacheKey)

	return s.saveUnsafe(app, envVars, operationSet)
}

func (s *ConsulEnvStore) Get(app, key string) (string, bool, error) {
	envVars, err := s.GetAll(app)
	if err != nil {
		return "", false, err
	}

	value, exists := envVars[key]
	return value, exists, nil
}

func (s *ConsulEnvStore) Delete(app, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Invalidate cache when deleting
	cacheKey := fmt.Sprintf("app:%s:env", app)
	s.cache.Delete(cacheKey)

	envVars, err := s.getUnsafe(app)
	if err != nil {
		return err
	}

	if envVars == nil {
		return nil // Nothing to delete
	}

	delete(envVars, key)
	return s.saveUnsafe(app, envVars, operationDelete)
}

func (s *ConsulEnvStore) getUnsafe(app string) (envstore.AppEnvVars, error) {
	key := s.appEnvKey(app)
	pair, _, err := s.kv.Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get from Consul: %w", err)
	}

	if pair == nil {
		return nil, nil // No data exists
	}

	var envVars envstore.AppEnvVars
	if err := json.Unmarshal(pair.Value, &envVars); err != nil {
		return nil, fmt.Errorf("failed to unmarshal environment variables: %w", err)
	}

	return envVars, nil
}

func (s *ConsulEnvStore) saveUnsafe(app string, envVars envstore.AppEnvVars, operation string) error {
	data, err := json.Marshal(envVars)
	if err != nil {
		return fmt.Errorf("failed to marshal environment variables: %w", err)
	}

	key := s.appEnvKey(app)
	start := time.Now()
	pair := &api.KVPair{Key: key, Value: data}

	if _, err = s.kv.Put(pair, nil); err != nil {
		s.recordOperation(metricsTargetConsul, operation, "failure", time.Since(start))
		return fmt.Errorf("failed to save to Consul: %w", err)
	}

	duration := time.Since(start)
	s.recordOperation(metricsTargetConsul, operation, "success", duration)

	log.Printf("[ConsulEnvStore] Saved %d environment variables for app %s to key %s (operation=%s)", len(envVars), app, key, operation)

	s.writeSecondary(key, operation, data, len(envVars))
	return nil
}

func (s *ConsulEnvStore) writeSecondary(key, operation string, data []byte, envCount int) {
	if !s.dualWrite || s.secondary == nil {
		return
	}

	start := time.Now()
	var err error

	if operation == operationDelete && envCount == 0 {
		err = s.secondary.Delete(key)
	} else {
		err = s.secondary.Put(key, data)
	}

	duration := time.Since(start)
	if err != nil {
		s.recordOperation(metricsTargetJetStream, operation, "failure", duration)
		log.Printf("[ConsulEnvStore] Secondary write failed for key %s (operation=%s): %v", key, operation, err)
		return
	}

	s.recordOperation(metricsTargetJetStream, operation, "success", duration)
}

func (s *ConsulEnvStore) recordOperation(target, operation, status string, duration time.Duration) {
	if s.metrics == nil {
		return
	}
	s.metrics.RecordEnvStoreOperation(target, operation, status, duration)
}

func (s *ConsulEnvStore) ToStringArray(app string) ([]string, error) {
	envVars, err := s.GetAll(app)
	if err != nil {
		return nil, err
	}

	var result []string
	for key, value := range envVars {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}

	return result, nil
}

// Health check for the Consul connection
func (s *ConsulEnvStore) HealthCheck() error {
	_, _, err := s.kv.Get("ploy/health", nil)
	if err != nil {
		return fmt.Errorf("consul health check failed: %w", err)
	}
	return nil
}

// GetCacheStats returns cache performance statistics
func (s *ConsulEnvStore) GetCacheStats() CacheStats {
	if s.cache != nil {
		return s.cache.GetCacheStats()
	}
	return CacheStats{}
}

// GetPoolStats returns cache statistics (pool functionality removed)
func (s *ConsulEnvStore) GetPoolStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["cache_stats"] = s.GetCacheStats()
	return stats
}

// ClearCache clears all cached environment variables
func (s *ConsulEnvStore) ClearCache() {
	if s.cache != nil {
		s.cache.Clear()
		log.Printf("[ConsulEnvStore] Cache cleared")
	}
}

// WarmupCache pre-loads frequently accessed apps into cache
func (s *ConsulEnvStore) WarmupCache(apps []string) error {
	log.Printf("[ConsulEnvStore] Warming up cache for %d apps", len(apps))
	for _, app := range apps {
		_, err := s.GetAll(app)
		if err != nil {
			log.Printf("Warning: Failed to warmup cache for app %s: %v", app, err)
		}
	}
	return nil
}
