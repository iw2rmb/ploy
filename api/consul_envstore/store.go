package consul_envstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/api/envstore"
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

type ConsulEnvStore struct {
	client     *api.Client
	keyPrefix  string
	mu         sync.RWMutex
	cache      *SimpleCache
	batchSize  int
}

// Ensure ConsulEnvStore implements the EnvStoreInterface
var _ envstore.EnvStoreInterface = (*ConsulEnvStore)(nil)

func New(consulAddr, keyPrefix string) (*ConsulEnvStore, error) {
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
	
	return &ConsulEnvStore{
		client:     client,
		keyPrefix:  keyPrefix,
		cache:      cache,
		batchSize:  10,
	}, nil
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
	kv := s.client.KV()
	pair, _, err := kv.Get(key, nil)
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
	return s.saveUnsafe(app, envVars)
}

func (s *ConsulEnvStore) SetAll(app string, envVars envstore.AppEnvVars) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Invalidate cache when setting
	cacheKey := fmt.Sprintf("app:%s:env", app)
	s.cache.Delete(cacheKey)
	
	return s.saveUnsafe(app, envVars)
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
	return s.saveUnsafe(app, envVars)
}

func (s *ConsulEnvStore) getUnsafe(app string) (envstore.AppEnvVars, error) {
	key := s.appEnvKey(app)
	kv := s.client.KV()
	
	pair, _, err := kv.Get(key, nil)
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

func (s *ConsulEnvStore) saveUnsafe(app string, envVars envstore.AppEnvVars) error {
	data, err := json.Marshal(envVars)
	if err != nil {
		return fmt.Errorf("failed to marshal environment variables: %w", err)
	}
	
	key := s.appEnvKey(app)
	kv := s.client.KV()
	
	pair := &api.KVPair{
		Key:   key,
		Value: data,
	}
	
	_, err = kv.Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to save to Consul: %w", err)
	}
	
	log.Printf("[ConsulEnvStore] Saved %d environment variables for app %s to key %s", len(envVars), app, key)
	return nil
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
	kv := s.client.KV()
	_, _, err := kv.Get("ploy/health", nil)
	if err != nil {
		return fmt.Errorf("Consul health check failed: %w", err)
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