package testutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/iw2rmb/ploy/controller/envstore"
	"github.com/iw2rmb/ploy/internal/storage"
)

// MockEnvStore provides a mock implementation of envstore.EnvStoreInterface
type MockEnvStore struct {
	mock.Mock
	data map[string]envstore.AppEnvVars // In-memory storage for testing
}

// NewMockEnvStore creates a new mock environment store
func NewMockEnvStore() *MockEnvStore {
	return &MockEnvStore{
		data: make(map[string]envstore.AppEnvVars),
	}
}

// GetAll retrieves all environment variables for an app
func (m *MockEnvStore) GetAll(app string) (envstore.AppEnvVars, error) {
	args := m.Called(app)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(envstore.AppEnvVars), args.Error(1)
}

// Set sets a single environment variable
func (m *MockEnvStore) Set(app, key, value string) error {
	args := m.Called(app, key, value)
	return args.Error(0)
}

// SetAll sets all environment variables for an app
func (m *MockEnvStore) SetAll(app string, envVars envstore.AppEnvVars) error {
	args := m.Called(app, envVars)
	return args.Error(0)
}

// Get retrieves a single environment variable
func (m *MockEnvStore) Get(app, key string) (string, bool, error) {
	args := m.Called(app, key)
	return args.String(0), args.Bool(1), args.Error(2)
}

// Delete deletes a single environment variable
func (m *MockEnvStore) Delete(app, key string) error {
	args := m.Called(app, key)
	return args.Error(0)
}

// ToStringArray converts environment variables to string array
func (m *MockEnvStore) ToStringArray(app string) ([]string, error) {
	args := m.Called(app)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Helper methods for easier mock setup

// WithApp sets up mock data for a specific app
func (m *MockEnvStore) WithApp(app string, envVars envstore.AppEnvVars) *MockEnvStore {
	m.data[app] = envVars
	m.On("GetAll", app).Return(envVars, nil)
	
	// Set up individual Get calls
	for key, value := range envVars {
		m.On("Get", app, key).Return(value, true, nil)
	}
	
	return m
}

// WithError sets up mock to return error for specific app
func (m *MockEnvStore) WithError(app string, err error) *MockEnvStore {
	m.On("GetAll", app).Return(nil, err)
	return m
}

// WithSetError sets up mock to return error when setting variables
func (m *MockEnvStore) WithSetError(app string, err error) *MockEnvStore {
	m.On("SetAll", app, mock.Anything).Return(err)
	return m
}

// MockStorageClient provides a mock implementation of storage client
type MockStorageClient struct {
	mock.Mock
	data map[string][]byte // In-memory storage for testing
}

// NewMockStorageClient creates a new mock storage client
func NewMockStorageClient() *MockStorageClient {
	return &MockStorageClient{
		data: make(map[string][]byte),
	}
}

// Upload uploads data to storage
func (m *MockStorageClient) Upload(ctx context.Context, key string, data []byte) error {
	args := m.Called(ctx, key, data)
	if args.Error(0) == nil {
		m.data[key] = data
	}
	return args.Error(0)
}

// Download downloads data from storage
func (m *MockStorageClient) Download(ctx context.Context, key string) ([]byte, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// Delete deletes data from storage
func (m *MockStorageClient) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	if args.Error(0) == nil {
		delete(m.data, key)
	}
	return args.Error(0)
}

// Exists checks if data exists in storage
func (m *MockStorageClient) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

// List lists storage items with prefix
func (m *MockStorageClient) List(ctx context.Context, prefix string) ([]storage.ObjectInfo, error) {
	args := m.Called(ctx, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.ObjectInfo), args.Error(1)
}

// Helper methods for easier mock setup

// WithFile sets up mock storage with a file
func (m *MockStorageClient) WithFile(key string, data []byte) *MockStorageClient {
	m.data[key] = data
	m.On("Download", mock.Anything, key).Return(data, nil)
	m.On("Exists", mock.Anything, key).Return(true, nil)
	m.On("Upload", mock.Anything, key, data).Return(nil)
	return m
}

// WithError sets up mock to return error for specific key
func (m *MockStorageClient) WithError(key string, err error) *MockStorageClient {
	m.On("Download", mock.Anything, key).Return(nil, err)
	m.On("Exists", mock.Anything, key).Return(false, err)
	return m
}

// WithUploadError sets up mock to return error on upload
func (m *MockStorageClient) WithUploadError(key string, err error) *MockStorageClient {
	m.On("Upload", mock.Anything, key, mock.Anything).Return(err)
	return m
}

// RealisticMockData provides factory functions for creating realistic mock data

// RealisticEnvVarsBuilder creates realistic environment variables for different app types
type RealisticEnvVarsBuilder struct{}

// NewRealisticEnvVarsBuilder creates a new builder
func NewRealisticEnvVarsBuilder() *RealisticEnvVarsBuilder {
	return &RealisticEnvVarsBuilder{}
}

// ForGoApp creates realistic env vars for a Go application
func (b *RealisticEnvVarsBuilder) ForGoApp(appName string) envstore.AppEnvVars {
	return envstore.AppEnvVars{
		"GO_ENV":         "production",
		"PORT":           "8080",
		"GRPC_PORT":      "9090",
		"METRICS_PORT":   "8081",
		"LOG_LEVEL":      "info",
		"LOG_FORMAT":     "json",
		"APP_NAME":       appName,
		"VERSION":        "1.0.0",
		"BUILD_TIME":     time.Now().Format(time.RFC3339),
		"CGO_ENABLED":    "0",
	}
}

// ForNodeApp creates realistic env vars for a Node.js application
func (b *RealisticEnvVarsBuilder) ForNodeApp(appName string) envstore.AppEnvVars {
	return envstore.AppEnvVars{
		"NODE_ENV":       "production",
		"PORT":           "3000",
		"API_BASE_URL":   "https://api.example.com",
		"LOG_LEVEL":      "info",
		"APP_NAME":       appName,
		"VERSION":        "2.1.0",
		"CACHE_TTL":      "300",
		"SESSION_SECRET": "test-session-secret",
		"DB_POOL_SIZE":   "10",
	}
}

// ForJavaApp creates realistic env vars for a Java application
func (b *RealisticEnvVarsBuilder) ForJavaApp(appName string) envstore.AppEnvVars {
	return envstore.AppEnvVars{
		"JAVA_OPTS":        "-Xmx1g -Xms512m -server",
		"SPRING_PROFILES":  "prod",
		"SERVER_PORT":      "8080",
		"MANAGEMENT_PORT":  "8081",
		"APP_NAME":         appName,
		"VERSION":          "1.2.0",
		"LOG_LEVEL":        "INFO",
		"DB_URL":           "jdbc:postgresql://db:5432/" + appName,
		"DB_USERNAME":      appName + "_user",
		"REDIS_URL":        "redis://cache:6379",
		"JVM_HEAP_SIZE":    "1g",
	}
}

// ForPythonApp creates realistic env vars for a Python application
func (b *RealisticEnvVarsBuilder) ForPythonApp(appName string) envstore.AppEnvVars {
	return envstore.AppEnvVars{
		"PYTHON_ENV":     "production",
		"PORT":           "8000",
		"WORKERS":        "4",
		"WORKER_CLASS":   "uvicorn.workers.UvicornWorker",
		"APP_NAME":       appName,
		"VERSION":        "3.1.0",
		"LOG_LEVEL":      "info",
		"DB_URL":         "postgresql://db:5432/" + appName,
		"REDIS_URL":      "redis://cache:6379",
		"MODEL_PATH":     "/models/latest",
		"BATCH_SIZE":     "32",
	}
}

// RealisticStorageBuilder creates realistic storage items
type RealisticStorageBuilder struct{}

// NewRealisticStorageBuilder creates a new builder
func NewRealisticStorageBuilder() *RealisticStorageBuilder {
	return &RealisticStorageBuilder{}
}

// ForApp creates realistic storage items for an app
func (b *RealisticStorageBuilder) ForApp(appName, version string) []TestStorageItem {
	baseTime := time.Now().Add(-24 * time.Hour)
	
	return []TestStorageItem{
		{
			Key:         fmt.Sprintf("apps/%s/%s/source.tar.gz", appName, version),
			Size:        2 * 1024 * 1024, // 2MB
			ContentType: "application/gzip",
			Checksum:    "sha256:" + generateChecksum(appName, "source"),
			CreatedAt:   baseTime,
		},
		{
			Key:         fmt.Sprintf("builds/%s/%s/artifact.tar", appName, version),
			Size:        10 * 1024 * 1024, // 10MB
			ContentType: "application/x-tar",
			Checksum:    "sha256:" + generateChecksum(appName, "build"),
			CreatedAt:   baseTime.Add(10 * time.Minute),
		},
		{
			Key:         fmt.Sprintf("logs/%s/%s.log", appName, time.Now().Format("2006-01-02")),
			Size:        50 * 1024, // 50KB
			ContentType: "text/plain",
			Checksum:    "sha256:" + generateChecksum(appName, "logs"),
			CreatedAt:   baseTime.Add(1 * time.Hour),
		},
	}
}

// generateChecksum generates a fake but consistent checksum for testing
func generateChecksum(appName, itemType string) string {
	// Simple hash-like string for testing
	base := appName + itemType
	hash := ""
	for i, r := range base {
		hash += fmt.Sprintf("%02x", int(r)+i)
	}
	// Pad or truncate to typical SHA256 length
	for len(hash) < 64 {
		hash += "0"
	}
	return hash[:64]
}

// MockAssertions provides common assertion patterns for mocks

// AssertEnvStoreCalled asserts that the env store was called correctly
func AssertEnvStoreCalled(t mock.TestingT, envStore *MockEnvStore, app string, operations ...string) {
	for _, op := range operations {
		switch op {
		case "GetAll":
			envStore.AssertCalled(t, "GetAll", app)
		case "SetAll":
			envStore.AssertCalled(t, "SetAll", app, mock.Anything)
		case "Delete":
			envStore.AssertCalled(t, "Delete", app, mock.Anything)
		}
	}
}

// AssertStorageClientCalled asserts that the storage client was called correctly
func AssertStorageClientCalled(t mock.TestingT, storageClient *MockStorageClient, key string, operations ...string) {
	for _, op := range operations {
		switch op {
		case "Upload":
			storageClient.AssertCalled(t, "Upload", mock.Anything, key, mock.Anything)
		case "Download":
			storageClient.AssertCalled(t, "Download", mock.Anything, key)
		case "Delete":
			storageClient.AssertCalled(t, "Delete", mock.Anything, key)
		case "Exists":
			storageClient.AssertCalled(t, "Exists", mock.Anything, key)
		}
	}
}

// Common mock scenarios

// NewMockEnvStoreWithApps creates a mock env store with multiple apps
func NewMockEnvStoreWithApps() *MockEnvStore {
	envStore := NewMockEnvStore()
	builder := NewRealisticEnvVarsBuilder()
	
	envStore.WithApp("go-api", builder.ForGoApp("go-api"))
	envStore.WithApp("node-frontend", builder.ForNodeApp("node-frontend"))
	envStore.WithApp("java-service", builder.ForJavaApp("java-service"))
	envStore.WithApp("python-ml", builder.ForPythonApp("python-ml"))
	
	return envStore
}

// NewMockStorageClientWithFiles creates a mock storage client with test files
func NewMockStorageClientWithFiles() *MockStorageClient {
	storageClient := NewMockStorageClient()
	builder := NewRealisticStorageBuilder()
	
	apps := []struct {
		name    string
		version string
	}{
		{"go-api", "v1.0.0"},
		{"node-frontend", "v2.1.0"},
		{"java-service", "v1.2.0"},
	}
	
	for _, app := range apps {
		items := builder.ForApp(app.name, app.version)
		for _, item := range items {
			data := []byte(fmt.Sprintf("Mock data for %s", item.Key))
			storageClient.WithFile(item.Key, data)
		}
	}
	
	return storageClient
}

// Error scenarios for testing

var (
	ErrMockNotFound      = errors.New("mock: resource not found")
	ErrMockUnauthorized  = errors.New("mock: unauthorized access")
	ErrMockTimeout       = errors.New("mock: operation timeout")
	ErrMockInvalidInput  = errors.New("mock: invalid input")
	ErrMockInternalError = errors.New("mock: internal server error")
)

// NewMockEnvStoreWithErrors creates a mock env store that returns errors
func NewMockEnvStoreWithErrors() *MockEnvStore {
	envStore := NewMockEnvStore()
	
	// Set up error scenarios
	envStore.WithError("not-found-app", ErrMockNotFound)
	envStore.WithError("timeout-app", ErrMockTimeout)
	envStore.WithSetError("readonly-app", ErrMockUnauthorized)
	
	return envStore
}

// NewMockStorageClientWithErrors creates a mock storage client that returns errors
func NewMockStorageClientWithErrors() *MockStorageClient {
	storageClient := NewMockStorageClient()
	
	// Set up error scenarios
	storageClient.WithError("not-found-key", ErrMockNotFound)
	storageClient.WithUploadError("readonly-key", ErrMockUnauthorized)
	storageClient.WithError("timeout-key", ErrMockTimeout)
	
	return storageClient
}