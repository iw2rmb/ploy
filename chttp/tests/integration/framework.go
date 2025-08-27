package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/iw2rmb/ploy/chttp/internal/server"
)

// IntegrationFramework provides utilities for integration testing
type IntegrationFramework struct {
	tempDirs []string
	servers  []*TestServer
}

// TestServer represents a test server instance
type TestServer struct {
	Server     *server.Server
	ConfigPath string
	Port       int
	URL        string
	Process    *os.Process
}

// TestFixture represents test data for integration tests
type TestFixture struct {
	Name        string
	Data        []byte
	ContentType string
	Description string
}

// TestFile represents a file to be included in test archives
type TestFile struct {
	Path    string
	Content string
}

// HTTPTestClient wraps http.Client with test-specific functionality
type HTTPTestClient struct {
	Client   *http.Client
	BaseURL  string
	ClientID string
	AuthKey  string
}

// NewIntegrationFramework creates a new integration testing framework
func NewIntegrationFramework() *IntegrationFramework {
	return &IntegrationFramework{
		tempDirs: make([]string, 0),
		servers:  make([]*TestServer, 0),
	}
}

// Cleanup cleans up all resources created by the framework
func (f *IntegrationFramework) Cleanup() {
	// Stop all test servers
	for _, server := range f.servers {
		f.StopServer(server)
	}

	// Remove all temporary directories
	for _, dir := range f.tempDirs {
		os.RemoveAll(dir)
	}
}

// CreateTestServer creates a new test server with the specified config
func (f *IntegrationFramework) CreateTestServer(configType string) (*TestServer, error) {
	configPath, cleanup, err := f.CreateTestConfig(configType)
	if err != nil {
		return nil, fmt.Errorf("failed to create test config: %w", err)
	}

	srv, err := server.NewServer(configPath)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	testServer := &TestServer{
		Server:     srv,
		ConfigPath: configPath,
		Port:       8080, // Default test port
		URL:        "http://localhost:8080",
	}

	f.servers = append(f.servers, testServer)
	return testServer, nil
}

// StartServer starts a test server
func (f *IntegrationFramework) StartServer(server *TestServer) error {
	// Start server in background
	go func() {
		server.Server.Start()
	}()
	
	return nil
}

// StopServer stops a test server
func (f *IntegrationFramework) StopServer(server *TestServer) error {
	if server.Server != nil {
		return server.Server.Shutdown()
	}
	return nil
}

// IsServerReady checks if a server is ready to accept requests
func (f *IntegrationFramework) IsServerReady(server *TestServer) bool {
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(server.URL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode == http.StatusOK
}

// WaitForServerReady waits for a server to be ready within timeout seconds
func (f *IntegrationFramework) WaitForServerReady(server *TestServer, timeoutSeconds int) (bool, error) {
	for i := 0; i < timeoutSeconds; i++ {
		if f.IsServerReady(server) {
			return true, nil
		}
		time.Sleep(time.Second)
	}
	return false, fmt.Errorf("server not ready within %d seconds", timeoutSeconds)
}

// CreateHTTPClient creates an HTTP client for testing
func (f *IntegrationFramework) CreateHTTPClient(clientID, authKey string) (*HTTPTestClient, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &HTTPTestClient{
		Client:   client,
		BaseURL:  "http://localhost:8080",
		ClientID: clientID,
		AuthKey:  authKey,
	}, nil
}

// CreateTestConfig creates a test configuration file
func (f *IntegrationFramework) CreateTestConfig(configType string) (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "chttp-test-")
	if err != nil {
		return "", nil, err
	}

	f.tempDirs = append(f.tempDirs, tempDir)
	
	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	configContent, err := f.generateConfigContent(configType)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	configPath := filepath.Join(tempDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		cleanup()
		return "", nil, err
	}

	return configPath, cleanup, nil
}

// generateConfigContent generates configuration content based on type
func (f *IntegrationFramework) generateConfigContent(configType string) (string, error) {
	baseConfig := `
service:
  name: "test-chttp"
  port: 8080
  listen_addr: "0.0.0.0:8080"

executable:
  path: "echo"
  args: ["test output"]
  timeout: "30s"
  working_dir: "/tmp"

security:
  auth_method: "none"
  run_as_user: ""
  max_memory: "512MB"
  max_cpu: "1.0"
  sandbox_enabled: false
  temp_dir: "/tmp"

input:
  formats: ["tar.gz", "tar", "zip"]
  allowed_extensions: [".py", ".js", ".go", ".txt"]
  max_archive_size: "100MB"
  max_files: 1000
  excluded_paths: ["__pycache__", ".git"]

output:
  format: "json"
  parser: "pylint"
  include_stats: false

logging:
  level: "info"
  format: "json"
  output: "stdout"
`

	switch configType {
	case "basic":
		return baseConfig, nil
		
	case "streaming":
		return baseConfig + `
input:
  streaming_enabled: true
  buffer_size: 32768
  buffer_pool_size: 10
  max_concurrent_streams: 5
`, nil

	case "auth":
		return `
service:
  name: "test-chttp-auth"
  port: 8080

executable:
  path: "echo"
  args: ["test output"]
  timeout: "30s"

security:
  auth_method: "public_key"
  public_key_path: "/tmp/test-public.key"
  run_as_user: ""
  max_memory: "512MB"
  max_cpu: "1.0"

input:
  formats: ["tar.gz"]
  allowed_extensions: [".py"]
  max_archive_size: "100MB"

output:
  format: "json"
  parser: "pylint"

logging:
  level: "info"
  format: "json"
`, nil

	case "rate-limiting":
		return baseConfig + `
security:
  rate_limit_per_sec: 2
  rate_limit_burst: 3
  max_open_files: 100
`, nil

	case "invalid":
		return "invalid yaml content {[", fmt.Errorf("intentionally invalid config")
		
	default:
		return "", fmt.Errorf("unknown config type: %s", configType)
	}
}

// GetTestFixture returns a test fixture by type
func (f *IntegrationFramework) GetTestFixture(fixtureType string) (*TestFixture, error) {
	switch fixtureType {
	case "python-valid":
		archive, err := f.CreateTestArchive([]TestFile{
			{Path: "main.py", Content: "#!/usr/bin/env python3\nprint('Hello, World!')"},
			{Path: "utils.py", Content: "def helper():\n    return 'helper'"},
		})
		if err != nil {
			return nil, err
		}
		return &TestFixture{
			Name:        "python-valid",
			Data:        archive,
			ContentType: "application/gzip",
			Description: "Valid Python project archive",
		}, nil

	case "python-errors":
		archive, err := f.CreateTestArchive([]TestFile{
			{Path: "bad.py", Content: "import os\nprint('unused import')\ndef unused_func():\n    pass"},
			{Path: "syntax.py", Content: "def broken(\n    print('syntax error')"},
		})
		if err != nil {
			return nil, err
		}
		return &TestFixture{
			Name:        "python-errors",
			Data:        archive,
			ContentType: "application/gzip",
			Description: "Python project with lint errors",
		}, nil

	case "invalid-archive":
		return &TestFixture{
			Name:        "invalid-archive",
			Data:        []byte("this is not a valid gzip archive"),
			ContentType: "application/gzip",
			Description: "Invalid archive data",
		}, nil

	case "large-archive":
		archive, err := f.CreateLargeArchive(5 * 1024 * 1024) // 5MB
		if err != nil {
			return nil, err
		}
		return &TestFixture{
			Name:        "large-archive", 
			Data:        archive,
			ContentType: "application/gzip",
			Description: "Large archive for performance testing",
		}, nil

	default:
		return nil, fmt.Errorf("unknown fixture type: %s", fixtureType)
	}
}

// CreateTestArchive creates a gzipped tar archive from test files
func (f *IntegrationFramework) CreateTestArchive(files []TestFile) ([]byte, error) {
	var buf bytes.Buffer
	
	// Create gzip writer
	gzWriter := gzip.NewWriter(&buf)
	defer gzWriter.Close()
	
	// Create tar writer
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()
	
	// Add files to archive
	for _, file := range files {
		header := &tar.Header{
			Name: file.Path,
			Mode: 0644,
			Size: int64(len(file.Content)),
		}
		
		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, err
		}
		
		if _, err := tarWriter.Write([]byte(file.Content)); err != nil {
			return nil, err
		}
	}
	
	tarWriter.Close()
	gzWriter.Close()
	
	return buf.Bytes(), nil
}

// CreateMalformedArchive creates a malformed archive for testing error handling
func (f *IntegrationFramework) CreateMalformedArchive() []byte {
	return []byte("malformed archive data that looks like gzip but isn't")
}

// CreateLargeArchive creates a large archive of specified size in bytes
func (f *IntegrationFramework) CreateLargeArchive(sizeBytes int) ([]byte, error) {
	// Create a large file content
	largeContent := make([]byte, sizeBytes-1024) // Leave room for tar headers
	rand.Read(largeContent)
	
	files := []TestFile{
		{Path: "large_file.txt", Content: string(largeContent)},
		{Path: "small.py", Content: "print('small file')"},
	}
	
	return f.CreateTestArchive(files)
}

// Post makes an authenticated POST request
func (c *HTTPTestClient) Post(path string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", contentType)
	
	if c.ClientID != "" {
		req.Header.Set("X-Client-ID", c.ClientID)
	}
	
	if c.AuthKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthKey)
	}
	
	return c.Client.Do(req)
}

// Get makes an authenticated GET request
func (c *HTTPTestClient) Get(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	
	if c.ClientID != "" {
		req.Header.Set("X-Client-ID", c.ClientID)
	}
	
	if c.AuthKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthKey)
	}
	
	return c.Client.Do(req)
}