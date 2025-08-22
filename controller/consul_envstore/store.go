package consul_envstore

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/controller/envstore"
)

type ConsulEnvStore struct {
	client     *api.Client
	keyPrefix  string
	mu         sync.RWMutex
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
	
	return &ConsulEnvStore{
		client:    client,
		keyPrefix: keyPrefix,
	}, nil
}

func (s *ConsulEnvStore) appEnvKey(app string) string {
	return fmt.Sprintf("%s/%s/env", s.keyPrefix, app)
}

func (s *ConsulEnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	key := s.appEnvKey(app)
	kv := s.client.KV()
	
	pair, _, err := kv.Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get from Consul: %w", err)
	}
	
	if pair == nil {
		// No environment variables exist for this app
		return envstore.AppEnvVars{}, nil
	}
	
	var envVars envstore.AppEnvVars
	if err := json.Unmarshal(pair.Value, &envVars); err != nil {
		return nil, fmt.Errorf("failed to unmarshal environment variables: %w", err)
	}
	
	log.Printf("[ConsulEnvStore] Retrieved %d environment variables for app %s", len(envVars), app)
	return envVars, nil
}

func (s *ConsulEnvStore) Set(app, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
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