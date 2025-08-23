package consul_envstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/controller/envstore"
	"github.com/iw2rmb/ploy/controller/performance"
)

type ConsulEnvStore struct {
	client     *api.Client
	keyPrefix  string
	mu         sync.RWMutex
	cache      *performance.StatefulCache
	consulPool *performance.ConsulPool
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
	
	// Create connection pool for better performance
	consulPool, err := performance.NewConsulPool(consulAddr, 10) // Pool size of 10
	if err != nil {
		log.Printf("Warning: Failed to create Consul pool, using single client: %v", err)
		consulPool = nil
	}
	
	// Create cache with 5-minute TTL for environment variables
	cache := performance.NewStatefulCache(5 * time.Minute)
	
	return &ConsulEnvStore{
		client:     client,
		keyPrefix:  keyPrefix,
		cache:      cache,
		consulPool: consulPool,
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
	
	// Use pooled client if available
	if s.consulPool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		var envVars envstore.AppEnvVars
		err := s.consulPool.WithClient(ctx, func(client *api.Client) error {
			kv := client.KV()
			pair, _, err := kv.Get(key, nil)
			if err != nil {
				return fmt.Errorf("failed to get from Consul: %w", err)
			}
			
			if pair == nil {
				envVars = envstore.AppEnvVars{}
				return nil
			}
			
			if err := json.Unmarshal(pair.Value, &envVars); err != nil {
				return fmt.Errorf("failed to unmarshal environment variables: %w", err)
			}
			
			return nil
		})
		
		if err != nil {
			return nil, err
		}
		
		// Cache the result
		s.cache.Set(cacheKey, envVars)
		log.Printf("[ConsulEnvStore] Retrieved and cached %d environment variables for app %s", len(envVars), app)
		return envVars, nil
	}
	
	// Fallback to direct client
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
func (s *ConsulEnvStore) GetCacheStats() performance.CacheStats {
	if s.cache != nil {
		return s.cache.Stats()
	}
	return performance.CacheStats{}
}

// GetPoolStats returns connection pool statistics
func (s *ConsulEnvStore) GetPoolStats() map[string]interface{} {
	stats := make(map[string]interface{})
	if s.consulPool != nil {
		stats["pool_size"] = s.consulPool.Size()
	}
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