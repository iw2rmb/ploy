package mocks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stretchr/testify/mock"
)

// ServiceHealth represents the health status of a service
type ServiceHealth string

const (
	ServiceHealthPassing  ServiceHealth = "passing"
	ServiceHealthWarning  ServiceHealth = "warning"
	ServiceHealthCritical ServiceHealth = "critical"
)

// MockService represents a Consul service
type MockService struct {
	ID      string
	Name    string
	Tags    []string
	Address string
	Port    int
	Health  ServiceHealth
	Meta    map[string]string
}

// MockKVPair represents a Consul key-value pair
type MockKVPair struct {
	Key         string
	Value       []byte
	Flags       uint64
	Session     string
	LockIndex   uint64
	CreateIndex uint64
	ModifyIndex uint64
}

// MockConsulClient is a mock implementation of the Consul client
type MockConsulClient struct {
	mock.Mock
	mu       sync.RWMutex
	services map[string]*MockService
	kvStore  map[string]*MockKVPair
}

// NewMockConsulClient creates a new mock Consul client
func NewMockConsulClient() *MockConsulClient {
	return &MockConsulClient{
		services: make(map[string]*MockService),
		kvStore:  make(map[string]*MockKVPair),
	}
}

// RegisterService registers a service with Consul
func (m *MockConsulClient) RegisterService(ctx context.Context, service *MockService) error {
	args := m.Called(ctx, service)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.services[service.ID] = service
	return nil
}

// DeregisterService deregisters a service from Consul
func (m *MockConsulClient) DeregisterService(ctx context.Context, serviceID string) error {
	args := m.Called(ctx, serviceID)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.services, serviceID)
	return nil
}

// GetService retrieves a service by ID
func (m *MockConsulClient) GetService(ctx context.Context, serviceID string) (*MockService, error) {
	args := m.Called(ctx, serviceID)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	service, exists := m.services[serviceID]
	if !exists {
		return nil, fmt.Errorf("service not found: %s", serviceID)
	}

	return service, nil
}

// ListServices lists all registered services
func (m *MockConsulClient) ListServices(ctx context.Context, tags []string) ([]*MockService, error) {
	args := m.Called(ctx, tags)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var services []*MockService
	for _, service := range m.services {
		// Simple tag filtering
		if len(tags) == 0 || m.hasAllTags(service, tags) {
			services = append(services, service)
		}
	}

	return services, nil
}

// DiscoverService discovers healthy services by name
func (m *MockConsulClient) DiscoverService(ctx context.Context, serviceName string) ([]*MockService, error) {
	args := m.Called(ctx, serviceName)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var services []*MockService
	for _, service := range m.services {
		if service.Name == serviceName && service.Health == ServiceHealthPassing {
			services = append(services, service)
		}
	}

	return services, nil
}

// UpdateServiceHealth updates the health status of a service
func (m *MockConsulClient) UpdateServiceHealth(ctx context.Context, serviceID string, health ServiceHealth) error {
	args := m.Called(ctx, serviceID, health)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	service, exists := m.services[serviceID]
	if !exists {
		return fmt.Errorf("service not found: %s", serviceID)
	}

	service.Health = health
	return nil
}

// Put stores a key-value pair
func (m *MockConsulClient) Put(ctx context.Context, key string, value []byte) error {
	args := m.Called(ctx, key, value)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.kvStore[key]
	var createIndex, modifyIndex uint64
	if exists {
		createIndex = existing.CreateIndex
		modifyIndex = existing.ModifyIndex + 1
	} else {
		createIndex = uint64(time.Now().Unix())
		modifyIndex = createIndex
	}

	m.kvStore[key] = &MockKVPair{
		Key:         key,
		Value:       value,
		CreateIndex: createIndex,
		ModifyIndex: modifyIndex,
	}

	return nil
}

// Get retrieves a key-value pair
func (m *MockConsulClient) Get(ctx context.Context, key string) (*MockKVPair, error) {
	args := m.Called(ctx, key)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	kv, exists := m.kvStore[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	return kv, nil
}

// Delete removes a key-value pair
func (m *MockConsulClient) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	
	if args.Error(0) != nil {
		return args.Error(0)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.kvStore, key)
	return nil
}

// List lists all keys with the given prefix
func (m *MockConsulClient) List(ctx context.Context, prefix string) ([]*MockKVPair, error) {
	args := m.Called(ctx, prefix)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var pairs []*MockKVPair
	for key, pair := range m.kvStore {
		if len(prefix) == 0 || len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			pairs = append(pairs, pair)
		}
	}

	return pairs, nil
}

// WatchKey watches for changes to a key
func (m *MockConsulClient) WatchKey(ctx context.Context, key string) (<-chan *MockKVPair, error) {
	args := m.Called(ctx, key)
	
	if args.Error(1) != nil {
		return nil, args.Error(1)
	}

	// Create a channel and return it
	// In a real implementation, this would watch for actual changes
	ch := make(chan *MockKVPair, 1)
	
	// Send current value if it exists
	m.mu.RLock()
	if kv, exists := m.kvStore[key]; exists {
		ch <- kv
	}
	m.mu.RUnlock()

	return ch, nil
}

// GetLeader returns the current cluster leader
func (m *MockConsulClient) GetLeader(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	
	if args.Error(1) != nil {
		return "", args.Error(1)
	}

	return "mock-consul-leader:8300", nil
}

// Health checks the health of the Consul cluster
func (m *MockConsulClient) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// Close closes the Consul client
func (m *MockConsulClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// SetupDefault sets up default mock behavior
func (m *MockConsulClient) SetupDefault() {
	m.On("RegisterService", mock.Anything, mock.Anything).Return(nil)
	m.On("DeregisterService", mock.Anything, mock.Anything).Return(nil)
	m.On("GetService", mock.Anything, mock.Anything).Return(&MockService{
		ID:      "test-service",
		Name:    "test",
		Health:  ServiceHealthPassing,
		Address: "127.0.0.1",
		Port:    8080,
	}, nil)
	m.On("ListServices", mock.Anything, mock.Anything).Return([]*MockService{}, nil)
	m.On("DiscoverService", mock.Anything, mock.Anything).Return([]*MockService{}, nil)
	m.On("UpdateServiceHealth", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	m.On("Put", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	m.On("Get", mock.Anything, mock.Anything).Return(&MockKVPair{
		Key:   "test-key",
		Value: []byte("test-value"),
	}, nil)
	m.On("Delete", mock.Anything, mock.Anything).Return(nil)
	m.On("List", mock.Anything, mock.Anything).Return([]*MockKVPair{}, nil)
	m.On("WatchKey", mock.Anything, mock.Anything).Return(make(chan *MockKVPair), nil)
	m.On("GetLeader", mock.Anything).Return("mock-leader:8300", nil)
	m.On("Health", mock.Anything).Return(nil)
	m.On("Close").Return(nil)
}

// SimulateFailure configures the mock to simulate Consul failures
func (m *MockConsulClient) SimulateFailure(operation string, err error) {
	switch operation {
	case "register":
		m.On("RegisterService", mock.Anything, mock.Anything).Return(err)
	case "deregister":
		m.On("DeregisterService", mock.Anything, mock.Anything).Return(err)
	case "get":
		m.On("Get", mock.Anything, mock.Anything).Return(nil, err)
	case "put":
		m.On("Put", mock.Anything, mock.Anything, mock.Anything).Return(err)
	case "health":
		m.On("Health", mock.Anything).Return(err)
	}
}

// SimulateServiceFailure marks a service as unhealthy
func (m *MockConsulClient) SimulateServiceFailure(serviceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if service, exists := m.services[serviceID]; exists {
		service.Health = ServiceHealthCritical
	}
}

// GetStoredServices returns all stored services (for testing purposes)
func (m *MockConsulClient) GetStoredServices() map[string]*MockService {
	m.mu.RLock()
	defer m.mu.RUnlock()

	services := make(map[string]*MockService)
	for k, v := range m.services {
		services[k] = v
	}
	return services
}

// GetStoredKVPairs returns all stored key-value pairs (for testing purposes)
func (m *MockConsulClient) GetStoredKVPairs() map[string]*MockKVPair {
	m.mu.RLock()
	defer m.mu.RUnlock()

	kvs := make(map[string]*MockKVPair)
	for k, v := range m.kvStore {
		kvs[k] = v
	}
	return kvs
}

// ClearAll clears all stored data
func (m *MockConsulClient) ClearAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.services = make(map[string]*MockService)
	m.kvStore = make(map[string]*MockKVPair)
}

// Helper function to check if service has all required tags
func (m *MockConsulClient) hasAllTags(service *MockService, requiredTags []string) bool {
	serviceTagMap := make(map[string]bool)
	for _, tag := range service.Tags {
		serviceTagMap[tag] = true
	}

	for _, requiredTag := range requiredTags {
		if !serviceTagMap[requiredTag] {
			return false
		}
	}

	return true
}