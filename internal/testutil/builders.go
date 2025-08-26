package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/iw2rmb/ploy/controller/envstore"
)

// AppTestBuilder provides fluent interface for creating test apps
type AppTestBuilder struct {
	app TestApp
}

// NewAppTestBuilder creates a new app builder with defaults
func NewAppTestBuilder() *AppTestBuilder {
	return &AppTestBuilder{
		app: TestApp{
			Name:      "default-app",
			Language:  "go",
			Lane:      "A",
			Version:   "1.0.0",
			Status:    "running",
			Instances: 1,
			EnvVars:   make(map[string]string),
			GitURL:    "https://github.com/test/default-app.git",
			Branch:    "main",
			BuildTime: 2 * time.Minute,
		},
	}
}

func (b *AppTestBuilder) Named(name string) *AppTestBuilder {
	b.app.Name = name
	return b
}

func (b *AppTestBuilder) WithLanguage(lang string) *AppTestBuilder {
	b.app.Language = lang
	return b
}

func (b *AppTestBuilder) InLane(lane string) *AppTestBuilder {
	b.app.Lane = lane
	return b
}

func (b *AppTestBuilder) Version(version string) *AppTestBuilder {
	b.app.Version = version
	return b
}

func (b *AppTestBuilder) WithStatus(status string) *AppTestBuilder {
	b.app.Status = status
	return b
}

func (b *AppTestBuilder) WithInstances(instances int) *AppTestBuilder {
	b.app.Instances = instances
	return b
}

func (b *AppTestBuilder) WithEnvVar(key, value string) *AppTestBuilder {
	if b.app.EnvVars == nil {
		b.app.EnvVars = make(map[string]string)
	}
	b.app.EnvVars[key] = value
	return b
}

func (b *AppTestBuilder) WithEnvVars(envVars map[string]string) *AppTestBuilder {
	if b.app.EnvVars == nil {
		b.app.EnvVars = make(map[string]string)
	}
	for k, v := range envVars {
		b.app.EnvVars[k] = v
	}
	return b
}

func (b *AppTestBuilder) WithGitRepo(url, branch string) *AppTestBuilder {
	b.app.GitURL = url
	b.app.Branch = branch
	return b
}

func (b *AppTestBuilder) WithBuildTime(duration time.Duration) *AppTestBuilder {
	b.app.BuildTime = duration
	return b
}

func (b *AppTestBuilder) Build() TestApp {
	return b.app
}

// BuildConfigTestBuilder provides fluent interface for creating build configs
type BuildConfigTestBuilder struct {
	config TestBuildConfig
}

// NewBuildConfigTestBuilder creates a new build config builder with defaults
func NewBuildConfigTestBuilder() *BuildConfigTestBuilder {
	return &BuildConfigTestBuilder{
		config: TestBuildConfig{
			Lane:    "A",
			Builder: "unikraft",
			Timeout: 300,
			Resources: TestResources{
				CPU:    "500m",
				Memory: "512Mi",
			},
			EnvVars: make(map[string]string),
		},
	}
}

func (b *BuildConfigTestBuilder) ForLane(lane string) *BuildConfigTestBuilder {
	b.config.Lane = lane
	return b
}

func (b *BuildConfigTestBuilder) WithBuilder(builder string) *BuildConfigTestBuilder {
	b.config.Builder = builder
	return b
}

func (b *BuildConfigTestBuilder) WithTimeout(timeout int) *BuildConfigTestBuilder {
	b.config.Timeout = timeout
	return b
}

func (b *BuildConfigTestBuilder) WithResources(cpu, memory string) *BuildConfigTestBuilder {
	b.config.Resources.CPU = cpu
	b.config.Resources.Memory = memory
	return b
}

func (b *BuildConfigTestBuilder) WithEnvVar(key, value string) *BuildConfigTestBuilder {
	if b.config.EnvVars == nil {
		b.config.EnvVars = make(map[string]string)
	}
	b.config.EnvVars[key] = value
	return b
}

func (b *BuildConfigTestBuilder) Build() TestBuildConfig {
	return b.config
}

// HTTPTestBuilder for API testing
type HTTPTestBuilder struct {
	method  string
	path    string
	body    interface{}
	headers map[string]string
	query   map[string]string
}

// NewHTTPTestBuilder creates a new HTTP test builder
func NewHTTPTestBuilder() *HTTPTestBuilder {
	return &HTTPTestBuilder{
		method:  "GET",
		headers: make(map[string]string),
		query:   make(map[string]string),
	}
}

func (b *HTTPTestBuilder) GET(path string) *HTTPTestBuilder {
	b.method = "GET"
	b.path = path
	return b
}

func (b *HTTPTestBuilder) POST(path string) *HTTPTestBuilder {
	b.method = "POST"
	b.path = path
	return b
}

func (b *HTTPTestBuilder) PUT(path string) *HTTPTestBuilder {
	b.method = "PUT"
	b.path = path
	return b
}

func (b *HTTPTestBuilder) DELETE(path string) *HTTPTestBuilder {
	b.method = "DELETE"
	b.path = path
	return b
}

func (b *HTTPTestBuilder) PATCH(path string) *HTTPTestBuilder {
	b.method = "PATCH"
	b.path = path
	return b
}

func (b *HTTPTestBuilder) WithJSON(body interface{}) *HTTPTestBuilder {
	b.body = body
	b.headers["Content-Type"] = "application/json"
	return b
}

func (b *HTTPTestBuilder) WithBody(body interface{}) *HTTPTestBuilder {
	b.body = body
	return b
}

func (b *HTTPTestBuilder) WithHeader(key, value string) *HTTPTestBuilder {
	b.headers[key] = value
	return b
}

func (b *HTTPTestBuilder) WithHeaders(headers map[string]string) *HTTPTestBuilder {
	for k, v := range headers {
		b.headers[k] = v
	}
	return b
}

func (b *HTTPTestBuilder) WithQuery(key, value string) *HTTPTestBuilder {
	b.query[key] = value
	return b
}

func (b *HTTPTestBuilder) WithQueryParams(params map[string]string) *HTTPTestBuilder {
	for k, v := range params {
		b.query[k] = v
	}
	return b
}

func (b *HTTPTestBuilder) WithAuth(token string) *HTTPTestBuilder {
	b.headers["Authorization"] = "Bearer " + token
	return b
}

func (b *HTTPTestBuilder) WithContentType(contentType string) *HTTPTestBuilder {
	b.headers["Content-Type"] = contentType
	return b
}

func (b *HTTPTestBuilder) BuildRequest() (*http.Request, error) {
	var bodyReader io.Reader

	if b.body != nil {
		switch body := b.body.(type) {
		case string:
			bodyReader = bytes.NewReader([]byte(body))
		case []byte:
			bodyReader = bytes.NewReader(body)
		default:
			bodyBytes, err := json.Marshal(b.body)
			if err != nil {
				return nil, err
			}
			bodyReader = bytes.NewReader(bodyBytes)
			// Set JSON content type if not already set
			if _, exists := b.headers["Content-Type"]; !exists {
				b.headers["Content-Type"] = "application/json"
			}
		}
	}

	req := httptest.NewRequest(b.method, b.path, bodyReader)

	for key, value := range b.headers {
		req.Header.Set(key, value)
	}

	if len(b.query) > 0 {
		q := req.URL.Query()
		for key, value := range b.query {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	return req, nil
}

// StorageItemTestBuilder provides fluent interface for creating storage items
type StorageItemTestBuilder struct {
	item TestStorageItem
}

// NewStorageItemTestBuilder creates a new storage item builder
func NewStorageItemTestBuilder() *StorageItemTestBuilder {
	return &StorageItemTestBuilder{
		item: TestStorageItem{
			Key:         "test-key",
			Size:        1024,
			ContentType: "application/octet-stream",
			Checksum:    "sha256:default",
			CreatedAt:   time.Now(),
		},
	}
}

func (b *StorageItemTestBuilder) WithKey(key string) *StorageItemTestBuilder {
	b.item.Key = key
	return b
}

func (b *StorageItemTestBuilder) WithSize(size int64) *StorageItemTestBuilder {
	b.item.Size = size
	return b
}

func (b *StorageItemTestBuilder) WithContentType(contentType string) *StorageItemTestBuilder {
	b.item.ContentType = contentType
	return b
}

func (b *StorageItemTestBuilder) WithChecksum(checksum string) *StorageItemTestBuilder {
	b.item.Checksum = checksum
	return b
}

func (b *StorageItemTestBuilder) WithCreatedAt(createdAt time.Time) *StorageItemTestBuilder {
	b.item.CreatedAt = createdAt
	return b
}

func (b *StorageItemTestBuilder) Build() TestStorageItem {
	return b.item
}

// EnvVarsTestBuilder provides fluent interface for creating environment variables
type EnvVarsTestBuilder struct {
	envVars envstore.AppEnvVars
}

// NewEnvVarsTestBuilder creates a new environment variables builder
func NewEnvVarsTestBuilder() *EnvVarsTestBuilder {
	return &EnvVarsTestBuilder{
		envVars: make(envstore.AppEnvVars),
	}
}

func (b *EnvVarsTestBuilder) WithVar(key, value string) *EnvVarsTestBuilder {
	b.envVars[key] = value
	return b
}

func (b *EnvVarsTestBuilder) WithVars(vars map[string]string) *EnvVarsTestBuilder {
	for k, v := range vars {
		b.envVars[k] = v
	}
	return b
}

func (b *EnvVarsTestBuilder) WithNodeEnv(env string) *EnvVarsTestBuilder {
	b.envVars["NODE_ENV"] = env
	return b
}

func (b *EnvVarsTestBuilder) WithGoEnv(env string) *EnvVarsTestBuilder {
	b.envVars["GO_ENV"] = env
	return b
}

func (b *EnvVarsTestBuilder) WithJavaOpts(opts string) *EnvVarsTestBuilder {
	b.envVars["JAVA_OPTS"] = opts
	return b
}

func (b *EnvVarsTestBuilder) WithPort(port string) *EnvVarsTestBuilder {
	b.envVars["PORT"] = port
	return b
}

func (b *EnvVarsTestBuilder) WithLogLevel(level string) *EnvVarsTestBuilder {
	b.envVars["LOG_LEVEL"] = level
	return b
}

func (b *EnvVarsTestBuilder) Build() envstore.AppEnvVars {
	return b.envVars
}

// Common builder helper functions

// BuildTestAppsForAllLanes creates test apps for each lane
func BuildTestAppsForAllLanes() []TestApp {
	lanes := []struct {
		lane     string
		language string
		builder  string
	}{
		{"A", "go", "unikraft"},
		{"B", "javascript", "unikraft-node"},
		{"C", "java", "osv"},
		{"E", "scala", "jib-container"},
		{"G", "rust", "wasm-pack"},
	}

	var apps []TestApp
	for i, l := range lanes {
		app := NewAppTestBuilder().
			Named("test-app-" + l.lane).
			WithLanguage(l.language).
			InLane(l.lane).
			Version("1.0." + string(rune('0'+i))).
			WithStatus("running").
			WithInstances(i + 1).
			Build()
		apps = append(apps, app)
	}

	return apps
}

// BuildTestEnvVarsForApp creates environment variables specific to an app type
func BuildTestEnvVarsForApp(appType string) envstore.AppEnvVars {
	builder := NewEnvVarsTestBuilder()

	switch appType {
	case "go":
		return builder.
			WithGoEnv("production").
			WithPort("8080").
			WithLogLevel("info").
			Build()
	case "node":
		return builder.
			WithNodeEnv("production").
			WithPort("3000").
			WithLogLevel("info").
			Build()
	case "java":
		return builder.
			WithJavaOpts("-Xmx1g").
			WithPort("8080").
			WithVar("SPRING_PROFILE", "prod").
			Build()
	case "python":
		return builder.
			WithVar("PYTHON_ENV", "production").
			WithPort("8000").
			WithVar("WORKERS", "4").
			Build()
	default:
		return builder.
			WithPort("8080").
			WithLogLevel("info").
			Build()
	}
}