package testutil

import (
	"time"

	"github.com/iw2rmb/ploy/api/envstore"
)

// TestDataRepository provides comprehensive test data
type TestDataRepository struct {
	Apps         []TestApp
	BuildConfigs []TestBuildConfig
	StorageItems []TestStorageItem
	EnvVarSets   []envstore.AppEnvVars
}

// TestApp represents a test application configuration
type TestApp struct {
	Name      string            `json:"name"`
	Language  string            `json:"language"`
	Lane      string            `json:"lane"`
	Version   string            `json:"version"`
	GitURL    string            `json:"git_url"`
	Branch    string            `json:"branch"`
	BuildTime time.Duration     `json:"build_time"`
	Status    string            `json:"status"`
	Instances int               `json:"instances"`
	EnvVars   map[string]string `json:"env_vars"`
}

// TestBuildConfig represents a test build configuration
type TestBuildConfig struct {
	Lane      string            `json:"lane"`
	Builder   string            `json:"builder"`
	Timeout   int               `json:"timeout"`
	Resources TestResources     `json:"resources"`
	EnvVars   map[string]string `json:"env_vars"`
}

// TestResources represents test resource configuration
type TestResources struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// TestStorageItem represents a test storage item
type TestStorageItem struct {
	Key         string    `json:"key"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	Checksum    string    `json:"checksum"`
	CreatedAt   time.Time `json:"created_at"`
}

// NewTestDataRepository creates repository with default test data
func NewTestDataRepository() *TestDataRepository {
	return &TestDataRepository{
		Apps:         generateTestApps(),
		BuildConfigs: generateTestBuildConfigs(),
		StorageItems: generateTestStorageItems(),
		EnvVarSets:   generateTestEnvVarSets(),
	}
}

// generateTestApps creates diverse app configurations for testing
func generateTestApps() []TestApp {
	return []TestApp{
		{
			Name:      "go-api",
			Language:  "go",
			Lane:      "A",
			Version:   "1.0.0",
			GitURL:    "https://github.com/test/go-api.git",
			Branch:    "main",
			BuildTime: 2 * time.Minute,
			Status:    "running",
			Instances: 3,
			EnvVars: map[string]string{
				"PORT":      "8080",
				"GO_ENV":    "production",
				"LOG_LEVEL": "info",
			},
		},
		{
			Name:      "node-frontend",
			Language:  "javascript",
			Lane:      "B",
			Version:   "2.1.0",
			GitURL:    "https://github.com/test/node-frontend.git",
			Branch:    "main",
			BuildTime: 5 * time.Minute,
			Status:    "building",
			Instances: 2,
			EnvVars: map[string]string{
				"NODE_ENV": "production",
				"PORT":     "3000",
				"API_URL":  "https://api.example.com",
			},
		},
		{
			Name:      "java-service",
			Language:  "java",
			Lane:      "C",
			Version:   "1.2.0",
			GitURL:    "https://github.com/test/java-service.git",
			Branch:    "develop",
			BuildTime: 8 * time.Minute,
			Status:    "failed",
			Instances: 0,
			EnvVars: map[string]string{
				"JAVA_OPTS":      "-Xmx512m",
				"SPRING_PROFILE": "prod",
				"DB_URL":         "jdbc:postgresql://db:5432/app",
			},
		},
		{
			Name:      "rust-wasm",
			Language:  "rust",
			Lane:      "G",
			Version:   "0.1.0",
			GitURL:    "https://github.com/test/rust-wasm.git",
			Branch:    "main",
			BuildTime: 3 * time.Minute,
			Status:    "running",
			Instances: 1,
			EnvVars: map[string]string{
				"RUST_ENV":      "production",
				"WASM_OPTIMIZE": "true",
			},
		},
		{
			Name:      "python-ml",
			Language:  "python",
			Lane:      "C",
			Version:   "3.2.1",
			GitURL:    "https://github.com/test/python-ml.git",
			Branch:    "main",
			BuildTime: 6 * time.Minute,
			Status:    "running",
			Instances: 2,
			EnvVars: map[string]string{
				"PYTHON_ENV":    "production",
				"MODEL_VERSION": "v2.1",
				"GPU_ENABLED":   "true",
			},
		},
		{
			Name:      "scala-analytics",
			Language:  "scala",
			Lane:      "E",
			Version:   "1.5.0",
			GitURL:    "https://github.com/test/scala-analytics.git",
			Branch:    "main",
			BuildTime: 12 * time.Minute,
			Status:    "stopped",
			Instances: 0,
			EnvVars: map[string]string{
				"SCALA_OPTS":    "-server",
				"SPARK_VERSION": "3.2.0",
				"MEMORY_LIMIT":  "8Gi",
			},
		},
	}
}

// generateTestBuildConfigs creates test build configurations
func generateTestBuildConfigs() []TestBuildConfig {
	return []TestBuildConfig{
		{
			Lane:    "A",
			Builder: "unikraft",
			Timeout: 300,
			Resources: TestResources{
				CPU:    "500m",
				Memory: "512Mi",
			},
			EnvVars: map[string]string{
				"CGO_ENABLED": "0",
				"GOOS":        "linux",
			},
		},
		{
			Lane:    "B",
			Builder: "unikraft-node",
			Timeout: 600,
			Resources: TestResources{
				CPU:    "1000m",
				Memory: "1Gi",
			},
			EnvVars: map[string]string{
				"NODE_ENV": "production",
			},
		},
		{
			Lane:    "C",
			Builder: "osv",
			Timeout: 900,
			Resources: TestResources{
				CPU:    "2000m",
				Memory: "2Gi",
			},
			EnvVars: map[string]string{
				"JVM_OPTS": "-server",
			},
		},
		{
			Lane:    "E",
			Builder: "jib-container",
			Timeout: 1200,
			Resources: TestResources{
				CPU:    "1500m",
				Memory: "4Gi",
			},
			EnvVars: map[string]string{
				"CONTAINER_RUNTIME": "firecracker",
			},
		},
		{
			Lane:    "G",
			Builder: "wasm-pack",
			Timeout: 240,
			Resources: TestResources{
				CPU:    "250m",
				Memory: "256Mi",
			},
			EnvVars: map[string]string{
				"WASM_TARGET": "wasm32-wasi",
				"OPTIMIZE":    "true",
			},
		},
	}
}

// generateTestStorageItems creates test storage items
func generateTestStorageItems() []TestStorageItem {
	baseTime := time.Now().Add(-24 * time.Hour)
	
	return []TestStorageItem{
		{
			Key:         "apps/go-api/v1.0.0/source.tar.gz",
			Size:        1024 * 1024,     // 1MB
			ContentType: "application/gzip",
			Checksum:    "sha256:abc123def456",
			CreatedAt:   baseTime,
		},
		{
			Key:         "apps/node-frontend/v2.1.0/source.tar.gz",
			Size:        2 * 1024 * 1024, // 2MB
			ContentType: "application/gzip",
			Checksum:    "sha256:def456ghi789",
			CreatedAt:   baseTime.Add(1 * time.Hour),
		},
		{
			Key:         "builds/java-service/v1.2.0/artifact.jar",
			Size:        50 * 1024 * 1024, // 50MB
			ContentType: "application/java-archive",
			Checksum:    "sha256:ghi789jkl012",
			CreatedAt:   baseTime.Add(2 * time.Hour),
		},
		{
			Key:         "builds/rust-wasm/v0.1.0/module.wasm",
			Size:        512 * 1024, // 512KB
			ContentType: "application/wasm",
			Checksum:    "sha256:jkl012mno345",
			CreatedAt:   baseTime.Add(3 * time.Hour),
		},
		{
			Key:         "logs/python-ml/2025-08-26.log",
			Size:        10 * 1024, // 10KB
			ContentType: "text/plain",
			Checksum:    "sha256:mno345pqr678",
			CreatedAt:   baseTime.Add(4 * time.Hour),
		},
	}
}

// generateTestEnvVarSets creates test environment variable sets
func generateTestEnvVarSets() []envstore.AppEnvVars {
	return []envstore.AppEnvVars{
		{
			"NODE_ENV":     "production",
			"PORT":         "3000",
			"API_URL":      "https://api.example.com",
			"LOG_LEVEL":    "info",
			"CACHE_TTL":    "300",
		},
		{
			"JAVA_OPTS":      "-Xmx1g -Xms512m",
			"SPRING_PROFILE": "prod",
			"DB_URL":         "postgresql://db:5432/app",
			"DB_USER":        "app_user",
			"REDIS_URL":      "redis://cache:6379",
		},
		{
			"PYTHON_ENV":     "production",
			"MODEL_PATH":     "/models/latest",
			"BATCH_SIZE":     "32",
			"GPU_MEMORY":     "4096",
			"WORKERS":        "4",
		},
		{
			"GO_ENV":         "production",
			"PORT":           "8080",
			"GRPC_PORT":      "9090",
			"METRICS_PORT":   "8081",
			"LOG_FORMAT":     "json",
		},
		{
			"RUST_ENV":       "production",
			"WASM_OPTIMIZE":  "true",
			"MEMORY_LIMIT":   "64MB",
			"THREAD_COUNT":   "1",
		},
	}
}

// Common test constants
const (
	// Test app names
	TestAppGoAPI        = "go-api"
	TestAppNodeFrontend = "node-frontend"
	TestAppJavaService  = "java-service"
	TestAppRustWasm     = "rust-wasm"
	TestAppPythonML     = "python-ml"
	TestAppScalaAnalytics = "scala-analytics"
	
	// Test lanes
	TestLaneA = "A" // Unikraft Go/Rust
	TestLaneB = "B" // Unikraft Node/Python
	TestLaneC = "C" // OSv JVM
	TestLaneE = "E" // Containers
	TestLaneG = "G" // WASM
	
	// Test versions
	TestVersionLatest = "latest"
	TestVersionV1     = "v1.0.0"
	TestVersionV2     = "v2.0.0"
	
	// Test statuses
	TestStatusRunning  = "running"
	TestStatusBuilding = "building"
	TestStatusFailed   = "failed"
	TestStatusStopped  = "stopped"
)

// GetTestApp returns a test app by name
func (r *TestDataRepository) GetTestApp(name string) *TestApp {
	for _, app := range r.Apps {
		if app.Name == name {
			return &app
		}
	}
	return nil
}

// GetTestBuildConfig returns a test build config by lane
func (r *TestDataRepository) GetTestBuildConfig(lane string) *TestBuildConfig {
	for _, config := range r.BuildConfigs {
		if config.Lane == lane {
			return &config
		}
	}
	return nil
}

// GetTestStorageItem returns a test storage item by key
func (r *TestDataRepository) GetTestStorageItem(key string) *TestStorageItem {
	for _, item := range r.StorageItems {
		if item.Key == key {
			return &item
		}
	}
	return nil
}

// GetTestEnvVars returns test environment variables by index
func (r *TestDataRepository) GetTestEnvVars(index int) envstore.AppEnvVars {
	if index >= 0 && index < len(r.EnvVarSets) {
		return r.EnvVarSets[index]
	}
	return envstore.AppEnvVars{}
}