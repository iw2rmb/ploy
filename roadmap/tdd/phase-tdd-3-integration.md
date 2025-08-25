# Phase 3: Integration Testing Framework

## Overview

Phase 3 establishes comprehensive integration testing for Ploy's API endpoints and service interactions. We'll create a robust framework for testing component integration while maintaining fast feedback cycles through strategic use of real and mock services.

## Objectives

1. Create comprehensive API testing framework with request builders
2. Implement service integration tests for critical workflows
3. Establish contract testing between components
4. Develop test data lifecycle management
5. Achieve 100% API endpoint coverage

## Implementation Plan

### API Testing Framework
- HTTP test client and builders
- API endpoint integration tests
- Request/response validation framework

### Service Integration
- Inter-service contract testing
- Workflow integration tests
- Performance integration testing

## Deliverables

### 1. API Testing Framework (`internal/testutil/api/`)

#### 1.1 HTTP Test Client (`client.go`)
```go
package api

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// TestClient provides a comprehensive HTTP client for integration testing
type TestClient struct {
    baseURL    string
    httpClient *http.Client
    t          *testing.T
    
    // Test context
    defaultHeaders map[string]string
    timeout        time.Duration
}

// NewTestClient creates a new API test client
func NewTestClient(t *testing.T, baseURL string) *TestClient {
    return &TestClient{
        baseURL: strings.TrimSuffix(baseURL, "/"),
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
        t:              t,
        defaultHeaders: make(map[string]string),
        timeout:        30 * time.Second,
    }
}

// WithTimeout sets request timeout
func (c *TestClient) WithTimeout(timeout time.Duration) *TestClient {
    c.timeout = timeout
    c.httpClient.Timeout = timeout
    return c
}

// WithDefaultHeader sets a default header for all requests
func (c *TestClient) WithDefaultHeader(key, value string) *TestClient {
    c.defaultHeaders[key] = value
    return c
}

// RequestBuilder provides fluent interface for building HTTP requests
type RequestBuilder struct {
    client  *TestClient
    method  string
    path    string
    body    interface{}
    headers map[string]string
    query   map[string]string
    
    // Expectations
    expectedStatus int
    expectJSON     bool
    expectError    bool
}

// GET creates a GET request builder
func (c *TestClient) GET(path string) *RequestBuilder {
    return &RequestBuilder{
        client:  c,
        method:  "GET",
        path:    path,
        headers: make(map[string]string),
        query:   make(map[string]string),
    }
}

// POST creates a POST request builder
func (c *TestClient) POST(path string) *RequestBuilder {
    return &RequestBuilder{
        client:  c,
        method:  "POST",
        path:    path,
        headers: make(map[string]string),
        query:   make(map[string]string),
    }
}

// PUT creates a PUT request builder
func (c *TestClient) PUT(path string) *RequestBuilder {
    return &RequestBuilder{
        client:  c,
        method:  "PUT",
        path:    path,
        headers: make(map[string]string),
        query:   make(map[string]string),
    }
}

// DELETE creates a DELETE request builder
func (c *TestClient) DELETE(path string) *RequestBuilder {
    return &RequestBuilder{
        client:  c,
        method:  "DELETE",
        path:    path,
        headers: make(map[string]string),
        query:   make(map[string]string),
    }
}

// WithJSON sets JSON body and appropriate headers
func (rb *RequestBuilder) WithJSON(body interface{}) *RequestBuilder {
    rb.body = body
    rb.headers["Content-Type"] = "application/json"
    return rb
}

// WithHeader adds a request header
func (rb *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
    rb.headers[key] = value
    return rb
}

// WithQuery adds a query parameter
func (rb *RequestBuilder) WithQuery(key, value string) *RequestBuilder {
    rb.query[key] = value
    return rb
}

// ExpectStatus sets expected HTTP status code
func (rb *RequestBuilder) ExpectStatus(status int) *RequestBuilder {
    rb.expectedStatus = status
    return rb
}

// ExpectJSON indicates response should be valid JSON
func (rb *RequestBuilder) ExpectJSON() *RequestBuilder {
    rb.expectJSON = true
    return rb
}

// ExpectError indicates request should fail
func (rb *RequestBuilder) ExpectError() *RequestBuilder {
    rb.expectError = true
    return rb
}

// Execute performs the HTTP request with automatic assertions
func (rb *RequestBuilder) Execute() *APIResponse {
    req, err := rb.buildRequest()
    require.NoError(rb.client.t, err, "Failed to build request")
    
    ctx, cancel := context.WithTimeout(context.Background(), rb.client.timeout)
    defer cancel()
    
    req = req.WithContext(ctx)
    
    resp, err := rb.client.httpClient.Do(req)
    if rb.expectError {
        assert.Error(rb.client.t, err, "Expected request to fail")
        return nil
    }
    
    require.NoError(rb.client.t, err, "HTTP request failed")
    defer resp.Body.Close()
    
    // Read response body
    bodyBytes, err := io.ReadAll(resp.Body)
    require.NoError(rb.client.t, err, "Failed to read response body")
    
    apiResp := &APIResponse{
        StatusCode: resp.StatusCode,
        Headers:    resp.Header,
        Body:       bodyBytes,
        t:          rb.client.t,
    }
    
    // Automatic assertions
    if rb.expectedStatus > 0 {
        assert.Equal(rb.client.t, rb.expectedStatus, resp.StatusCode, 
            "Unexpected status code. Response body: %s", string(bodyBytes))
    }
    
    if rb.expectJSON {
        var jsonData interface{}
        err := json.Unmarshal(bodyBytes, &jsonData)
        assert.NoError(rb.client.t, err, "Response should be valid JSON")
    }
    
    return apiResp
}

// buildRequest constructs the HTTP request
func (rb *RequestBuilder) buildRequest() (*http.Request, error) {
    fullURL := rb.client.baseURL + rb.path
    
    // Add query parameters
    if len(rb.query) > 0 {
        params := url.Values{}
        for key, value := range rb.query {
            params.Add(key, value)
        }
        fullURL += "?" + params.Encode()
    }
    
    var bodyReader io.Reader
    if rb.body != nil {
        bodyBytes, err := json.Marshal(rb.body)
        if err != nil {
            return nil, fmt.Errorf("failed to marshal request body: %w", err)
        }
        bodyReader = bytes.NewReader(bodyBytes)
    }
    
    req, err := http.NewRequest(rb.method, fullURL, bodyReader)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }
    
    // Add default headers
    for key, value := range rb.client.defaultHeaders {
        req.Header.Set(key, value)
    }
    
    // Add request-specific headers
    for key, value := range rb.headers {
        req.Header.Set(key, value)
    }
    
    return req, nil
}

// APIResponse wraps HTTP response with test utilities
type APIResponse struct {
    StatusCode int
    Headers    http.Header
    Body       []byte
    t          *testing.T
}

// JSON unmarshals response body as JSON
func (r *APIResponse) JSON(target interface{}) *APIResponse {
    err := json.Unmarshal(r.Body, target)
    require.NoError(r.t, err, "Failed to unmarshal JSON response")
    return r
}

// AssertStatus verifies status code
func (r *APIResponse) AssertStatus(expected int) *APIResponse {
    assert.Equal(r.t, expected, r.StatusCode, 
        "Unexpected status code. Response: %s", string(r.Body))
    return r
}

// AssertHeader verifies response header
func (r *APIResponse) AssertHeader(key, expected string) *APIResponse {
    actual := r.Headers.Get(key)
    assert.Equal(r.t, expected, actual, "Unexpected header value for %s", key)
    return r
}

// AssertJSONPath verifies JSON field value using simple path notation
func (r *APIResponse) AssertJSONPath(path string, expected interface{}) *APIResponse {
    var data map[string]interface{}
    err := json.Unmarshal(r.Body, &data)
    require.NoError(r.t, err, "Failed to unmarshal JSON for path assertion")
    
    value := getJSONPath(data, path)
    assert.Equal(r.t, expected, value, "Unexpected value at JSON path %s", path)
    return r
}

// Helper function to extract value from JSON path (simple implementation)
func getJSONPath(data map[string]interface{}, path string) interface{} {
    parts := strings.Split(path, ".")
    current := interface{}(data)
    
    for _, part := range parts {
        if m, ok := current.(map[string]interface{}); ok {
            current = m[part]
        } else {
            return nil
        }
    }
    
    return current
}
```

#### 1.2 API Test Scenarios (`scenarios.go`)
```go
package api

import (
    "testing"
    "time"
    
    "github.com/iw2rmb/ploy/internal/testutil"
)

// APITestSuite provides comprehensive API testing scenarios
type APITestSuite struct {
    client   *TestClient
    fixtures *testutil.TestDataRepository
    t        *testing.T
}

// NewAPITestSuite creates a new API test suite
func NewAPITestSuite(t *testing.T, baseURL string) *APITestSuite {
    return &APITestSuite{
        client:   NewTestClient(t, baseURL),
        fixtures: testutil.NewTestDataRepository(),
        t:        t,
    }
}

// TestHealthEndpoints verifies all health check endpoints
func (suite *APITestSuite) TestHealthEndpoints() {
    suite.t.Run("health endpoints", func(t *testing.T) {
        // Basic health check
        suite.client.GET("/health").
            ExpectStatus(200).
            ExpectJSON().
            Execute().
            AssertJSONPath("status", "healthy")
        
        // Readiness check
        suite.client.GET("/ready").
            ExpectStatus(200).
            ExpectJSON().
            Execute()
        
        // Liveness check
        suite.client.GET("/live").
            ExpectStatus(200).
            Execute()
        
        // Metrics endpoint
        suite.client.GET("/metrics").
            ExpectStatus(200).
            Execute()
    })
}

// TestAppLifecycle tests complete application lifecycle
func (suite *APITestSuite) TestAppLifecycle() {
    suite.t.Run("app lifecycle", func(t *testing.T) {
        appName := "test-lifecycle-app"
        
        // 1. Create app via build trigger
        buildRequest := map[string]interface{}{
            "git_url": "https://github.com/test/go-app.git",
            "branch":  "main",
        }
        
        suite.client.POST("/v1/apps/"+appName+"/builds").
            WithJSON(buildRequest).
            ExpectStatus(202).
            ExpectJSON().
            Execute().
            AssertJSONPath("status", "build_triggered")
        
        // 2. Check app appears in list
        suite.client.GET("/v1/apps").
            ExpectStatus(200).
            ExpectJSON().
            Execute()
            // Could add more specific assertions about app in list
        
        // 3. Get app status
        suite.client.GET("/v1/apps/"+appName+"/status").
            ExpectStatus(200).
            ExpectJSON().
            Execute()
        
        // 4. Set environment variables
        envVars := map[string]string{
            "NODE_ENV": "test",
            "DEBUG":    "true",
        }
        
        suite.client.POST("/v1/apps/"+appName+"/env").
            WithJSON(envVars).
            ExpectStatus(200).
            Execute()
        
        // 5. Get environment variables
        suite.client.GET("/v1/apps/"+appName+"/env").
            ExpectStatus(200).
            ExpectJSON().
            Execute().
            AssertJSONPath("env.NODE_ENV", "test").
            AssertJSONPath("env.DEBUG", "true")
        
        // 6. Cleanup - destroy app
        suite.client.DELETE("/v1/apps/"+appName).
            ExpectStatus(200).
            Execute()
        
        // 7. Verify app is gone
        suite.client.GET("/v1/apps/"+appName+"/status").
            ExpectStatus(404).
            Execute()
    })
}

// TestEnvironmentVariables tests comprehensive env var management
func (suite *APITestSuite) TestEnvironmentVariables() {
    suite.t.Run("environment variables", func(t *testing.T) {
        appName := "test-env-app"
        
        // Initially empty
        suite.client.GET("/v1/apps/"+appName+"/env").
            ExpectStatus(200).
            ExpectJSON().
            Execute().
            AssertJSONPath("env", map[string]interface{}{})
        
        // Set multiple variables
        envVars := map[string]string{
            "VAR1": "value1",
            "VAR2": "value2",
            "VAR3": "value3",
        }
        
        suite.client.POST("/v1/apps/"+appName+"/env").
            WithJSON(envVars).
            ExpectStatus(200).
            Execute()
        
        // Verify all set
        resp := suite.client.GET("/v1/apps/"+appName+"/env").
            ExpectStatus(200).
            ExpectJSON().
            Execute()
        
        var envResponse map[string]interface{}
        resp.JSON(&envResponse)
        
        envMap := envResponse["env"].(map[string]interface{})
        assert.Equal(suite.t, "value1", envMap["VAR1"])
        assert.Equal(suite.t, "value2", envMap["VAR2"])
        assert.Equal(suite.t, "value3", envMap["VAR3"])
        
        // Update single variable
        suite.client.PUT("/v1/apps/"+appName+"/env/VAR1").
            WithJSON(map[string]string{"value": "updated_value1"}).
            ExpectStatus(200).
            Execute()
        
        // Verify update
        suite.client.GET("/v1/apps/"+appName+"/env").
            ExpectStatus(200).
            Execute().
            AssertJSONPath("env.VAR1", "updated_value1").
            AssertJSONPath("env.VAR2", "value2") // unchanged
        
        // Delete single variable
        suite.client.DELETE("/v1/apps/"+appName+"/env/VAR2").
            ExpectStatus(200).
            Execute()
        
        // Verify deletion
        suite.client.GET("/v1/apps/"+appName+"/env").
            ExpectStatus(200).
            Execute().
            AssertJSONPath("env.VAR1", "updated_value1")
            // VAR2 should not exist
            // VAR3 should still exist
    })
}

// TestDomainManagement tests domain management endpoints
func (suite *APITestSuite) TestDomainManagement() {
    suite.t.Run("domain management", func(t *testing.T) {
        appName := "test-domain-app"
        domain := "test.example.com"
        
        // Add domain
        suite.client.POST("/v1/apps/"+appName+"/domains").
            WithJSON(map[string]string{"domain": domain}).
            ExpectStatus(201).
            Execute()
        
        // List domains
        suite.client.GET("/v1/apps/"+appName+"/domains").
            ExpectStatus(200).
            ExpectJSON().
            Execute()
        
        // Remove domain
        suite.client.DELETE("/v1/apps/"+appName+"/domains/"+domain).
            ExpectStatus(200).
            Execute()
        
        // Verify removal
        suite.client.GET("/v1/apps/"+appName+"/domains").
            ExpectStatus(200).
            Execute()
    })
}

// TestErrorHandling tests various error scenarios
func (suite *APITestSuite) TestErrorHandling() {
    suite.t.Run("error handling", func(t *testing.T) {
        // Invalid app name
        suite.client.GET("/v1/apps/invalid-@#$-name/status").
            ExpectStatus(400).
            ExpectJSON().
            Execute().
            AssertJSONPath("error", "Invalid app name")
        
        // Non-existent app
        suite.client.GET("/v1/apps/non-existent-app/status").
            ExpectStatus(404).
            Execute()
        
        // Invalid JSON body
        suite.client.POST("/v1/apps/test-app/env").
            WithHeader("Content-Type", "application/json").
            WithJSON(`{invalid json}`).
            ExpectStatus(400).
            Execute()
        
        // Missing required fields
        suite.client.POST("/v1/apps/test-app/builds").
            WithJSON(map[string]interface{}{}).
            ExpectStatus(400).
            Execute()
    })
}

// TestConcurrency tests concurrent API operations
func (suite *APITestSuite) TestConcurrency() {
    suite.t.Run("concurrency", func(t *testing.T) {
        appName := "test-concurrent-app"
        numGoroutines := 10
        
        // Channel to collect results
        results := make(chan error, numGoroutines)
        
        // Concurrent environment variable updates
        for i := 0; i < numGoroutines; i++ {
            go func(id int) {
                envVar := fmt.Sprintf("CONCURRENT_VAR_%d", id)
                value := fmt.Sprintf("value_%d", id)
                
                _, err := suite.client.PUT("/v1/apps/"+appName+"/env/"+envVar).
                    WithJSON(map[string]string{"value": value}).
                    ExpectStatus(200).
                    Execute(), nil
                
                results <- err
            }(i)
        }
        
        // Wait for all goroutines
        for i := 0; i < numGoroutines; i++ {
            err := <-results
            assert.NoError(suite.t, err, "Concurrent operation failed")
        }
        
        // Verify all variables were set
        resp := suite.client.GET("/v1/apps/"+appName+"/env").
            ExpectStatus(200).
            Execute()
        
        var envResponse map[string]interface{}
        resp.JSON(&envResponse)
        
        envMap := envResponse["env"].(map[string]interface{})
        for i := 0; i < numGoroutines; i++ {
            envVar := fmt.Sprintf("CONCURRENT_VAR_%d", i)
            expectedValue := fmt.Sprintf("value_%d", i)
            assert.Equal(suite.t, expectedValue, envMap[envVar])
        }
    })
}
```

### 2. Service Integration Tests

#### 2.1 Build Pipeline Integration (`build_integration_test.go`)
```go
//go:build integration
// +build integration

package build_test

import (
    "context"
    "os"
    "testing"
    "time"
    
    consulapi "github.com/hashicorp/consul/api"
    nomadapi "github.com/hashicorp/nomad/api"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/suite"
    
    "github.com/iw2rmb/ploy/internal/build"
    "github.com/iw2rmb/ploy/internal/storage"
    "github.com/iw2rmb/ploy/internal/testutil"
)

// BuildIntegrationSuite tests build pipeline with real services
type BuildIntegrationSuite struct {
    suite.Suite
    
    nomadClient   *nomadapi.Client
    consulClient  *consulapi.Client
    storageClient *storage.StorageClient
    buildHandler  *build.Handler
    
    tempDir string
}

func (suite *BuildIntegrationSuite) SetupSuite() {
    // Initialize Nomad client
    nomadConfig := nomadapi.DefaultConfig()
    nomadConfig.Address = testutil.GetEnvOrDefault("NOMAD_ADDR", "http://localhost:4646")
    
    var err error
    suite.nomadClient, err = nomadapi.NewClient(nomadConfig)
    require.NoError(suite.T(), err)
    
    // Wait for Nomad to be ready
    suite.waitForNomad()
    
    // Initialize Consul client
    consulConfig := consulapi.DefaultConfig()
    consulConfig.Address = testutil.GetEnvOrDefault("CONSUL_HTTP_ADDR", "localhost:8500")
    
    suite.consulClient, err = consulapi.NewClient(consulConfig)
    require.NoError(suite.T(), err)
    
    // Wait for Consul to be ready
    suite.waitForConsul()
    
    // Initialize storage client
    storageConfig := &storage.Config{
        Type:         "seaweedfs",
        Endpoint:     testutil.GetEnvOrDefault("SEAWEEDFS_FILER", "http://localhost:8888"),
        MasterServer: testutil.GetEnvOrDefault("SEAWEEDFS_MASTER", "http://localhost:9333"),
    }
    
    suite.storageClient = storage.NewSeaweedFSClient(storageConfig)
    
    // Wait for SeaweedFS to be ready
    suite.waitForSeaweedFS()
    
    // Initialize build handler
    suite.buildHandler = build.NewHandler(
        suite.storageClient,
        suite.nomadClient,
        build.NewLaneDetector(),
    )
    
    // Create temporary directory for test apps
    suite.tempDir, err = os.MkdirTemp("", "ploy-build-test-*")
    require.NoError(suite.T(), err)
}

func (suite *BuildIntegrationSuite) TearDownSuite() {
    // Cleanup test resources
    if suite.tempDir != "" {
        os.RemoveAll(suite.tempDir)
    }
    
    // Cleanup test jobs from Nomad
    suite.cleanupTestJobs()
}

func (suite *BuildIntegrationSuite) waitForNomad() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    for {
        select {
        case <-ctx.Done():
            suite.T().Fatal("Nomad not ready within timeout")
        default:
            _, _, err := suite.nomadClient.Status().Leader()
            if err == nil {
                return
            }
            time.Sleep(1 * time.Second)
        }
    }
}

func (suite *BuildIntegrationSuite) waitForConsul() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    for {
        select {
        case <-ctx.Done():
            suite.T().Fatal("Consul not ready within timeout")
        default:
            _, err := suite.consulClient.Status().Leader()
            if err == nil {
                return
            }
            time.Sleep(1 * time.Second)
        }
    }
}

func (suite *BuildIntegrationSuite) waitForSeaweedFS() {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    for {
        select {
        case <-ctx.Done():
            suite.T().Fatal("SeaweedFS not ready within timeout")
        default:
            err := suite.storageClient.HealthCheck(ctx)
            if err == nil {
                return
            }
            time.Sleep(1 * time.Second)
        }
    }
}

func (suite *BuildIntegrationSuite) cleanupTestJobs() {
    jobs, _, err := suite.nomadClient.Jobs().List(nil)
    if err != nil {
        return
    }
    
    for _, job := range jobs {
        if strings.HasPrefix(job.Name, "test-") {
            suite.nomadClient.Jobs().Deregister(job.ID, true, nil)
        }
    }
}

func (suite *BuildIntegrationSuite) TestBuildGolangApp() {
    // Create test Go app
    appDir := suite.createTestGoApp("test-go-app")
    
    // Create build configuration
    config := &build.Config{
        AppName: "test-go-app",
        AppPath: appDir,
        Lane:    "A",
        GitURL:  "file://" + appDir,
        Branch:  "main",
    }
    
    // Trigger build
    ctx := context.Background()
    result, err := suite.buildHandler.ExecuteBuild(ctx, config)
    
    require.NoError(suite.T(), err)
    assert.Equal(suite.T(), build.StatusSuccess, result.Status)
    assert.NotEmpty(suite.T(), result.ArtifactID)
    
    // Verify artifact was uploaded to storage
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    exists, err := suite.storageClient.Exists(ctx, result.ArtifactID)
    require.NoError(suite.T(), err)
    assert.True(suite.T(), exists, "Build artifact should be uploaded")
    
    // Verify Nomad job was created
    job, _, err := suite.nomadClient.Jobs().Info("test-go-app", nil)
    require.NoError(suite.T(), err)
    assert.Equal(suite.T(), "running", job.Status)
    
    // Verify job is actually running
    suite.Eventually(func() bool {
        allocs, _, err := suite.nomadClient.Jobs().Allocations("test-go-app", false, nil)
        if err != nil {
            return false
        }
        
        for _, alloc := range allocs {
            if alloc.ClientStatus == "running" {
                return true
            }
        }
        return false
    }, 60*time.Second, 2*time.Second, "Job should be running")
}

func (suite *BuildIntegrationSuite) TestBuildNodejsApp() {
    appDir := suite.createTestNodeApp("test-node-app")
    
    config := &build.Config{
        AppName: "test-node-app",
        AppPath: appDir,
        Lane:    "B",
        GitURL:  "file://" + appDir,
        Branch:  "main",
    }
    
    ctx := context.Background()
    result, err := suite.buildHandler.ExecuteBuild(ctx, config)
    
    require.NoError(suite.T(), err)
    assert.Equal(suite.T(), build.StatusSuccess, result.Status)
    
    // Verify Lane B specific artifacts
    assert.Contains(suite.T(), result.BuildLogs, "npm install")
    assert.Contains(suite.T(), result.BuildLogs, "Unikraft")
}

func (suite *BuildIntegrationSuite) TestBuildFailure() {
    appDir := suite.createInvalidApp("test-invalid-app")
    
    config := &build.Config{
        AppName: "test-invalid-app",
        AppPath: appDir,
        Lane:    "A",
        GitURL:  "file://" + appDir,
        Branch:  "main",
    }
    
    ctx := context.Background()
    result, err := suite.buildHandler.ExecuteBuild(ctx, config)
    
    // Build should fail gracefully
    require.NoError(suite.T(), err, "Handler should not return error for build failure")
    assert.Equal(suite.T(), build.StatusFailed, result.Status)
    assert.NotEmpty(suite.T(), result.ErrorMessage)
    assert.NotEmpty(suite.T(), result.BuildLogs)
}

func (suite *BuildIntegrationSuite) TestConcurrentBuilds() {
    numBuilds := 3
    results := make(chan *build.Result, numBuilds)
    
    // Start multiple builds concurrently
    for i := 0; i < numBuilds; i++ {
        go func(id int) {
            appName := fmt.Sprintf("test-concurrent-app-%d", id)
            appDir := suite.createTestGoApp(appName)
            
            config := &build.Config{
                AppName: appName,
                AppPath: appDir,
                Lane:    "A",
                GitURL:  "file://" + appDir,
                Branch:  "main",
            }
            
            ctx := context.Background()
            result, err := suite.buildHandler.ExecuteBuild(ctx, config)
            
            require.NoError(suite.T(), err)
            results <- result
        }(i)
    }
    
    // Collect all results
    for i := 0; i < numBuilds; i++ {
        result := <-results
        assert.Equal(suite.T(), build.StatusSuccess, result.Status)
    }
}

// Helper methods to create test applications

func (suite *BuildIntegrationSuite) createTestGoApp(name string) string {
    appDir := filepath.Join(suite.tempDir, name)
    err := os.MkdirAll(appDir, 0755)
    require.NoError(suite.T(), err)
    
    // Create go.mod
    goMod := fmt.Sprintf("module %s\n\ngo 1.21\n", name)
    err = os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0644)
    require.NoError(suite.T(), err)
    
    // Create main.go
    mainGo := `package main

import (
    "fmt"
    "net/http"
    "os"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Hello from %s!", os.Getenv("APP_NAME"))
    })
    
    http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprint(w, "OK")
    })
    
    fmt.Printf("Server starting on port %s\n", port)
    http.ListenAndServe(":"+port, nil)
}
`
    
    err = os.WriteFile(filepath.Join(appDir, "main.go"), []byte(mainGo), 0644)
    require.NoError(suite.T(), err)
    
    return appDir
}

func (suite *BuildIntegrationSuite) createTestNodeApp(name string) string {
    appDir := filepath.Join(suite.tempDir, name)
    err := os.MkdirAll(appDir, 0755)
    require.NoError(suite.T(), err)
    
    // Create package.json
    packageJSON := fmt.Sprintf(`{
  "name": "%s",
  "version": "1.0.0",
  "description": "Test Node.js app",
  "main": "server.js",
  "scripts": {
    "start": "node server.js"
  },
  "dependencies": {
    "express": "^4.18.0"
  }
}`, name)
    
    err = os.WriteFile(filepath.Join(appDir, "package.json"), []byte(packageJSON), 0644)
    require.NoError(suite.T(), err)
    
    // Create server.js
    serverJS := `const express = require('express');
const app = express();
const port = process.env.PORT || 3000;

app.get('/', (req, res) => {
  res.send('Hello from Node.js app!');
});

app.get('/healthz', (req, res) => {
  res.status(200).send('OK');
});

app.listen(port, () => {
  console.log('Server running on port ${port}');
});
`
    
    err = os.WriteFile(filepath.Join(appDir, "server.js"), []byte(serverJS), 0644)
    require.NoError(suite.T(), err)
    
    return appDir
}

func (suite *BuildIntegrationSuite) createInvalidApp(name string) string {
    appDir := filepath.Join(suite.tempDir, name)
    err := os.MkdirAll(appDir, 0755)
    require.NoError(suite.T(), err)
    
    // Create invalid go.mod
    goMod := "invalid go.mod content"
    err = os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0644)
    require.NoError(suite.T(), err)
    
    // Create invalid main.go
    mainGo := "this is not valid go code {"
    err = os.WriteFile(filepath.Join(appDir, "main.go"), []byte(mainGo), 0644)
    require.NoError(suite.T(), err)
    
    return appDir
}

func TestBuildIntegrationSuite(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration tests in short mode")
    }
    
    suite.Run(t, new(BuildIntegrationSuite))
}
```

### 3. Contract Testing Framework

#### 3.1 Service Contract Tests (`contract_test.go`)
```go
package contract

import (
    "context"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/iw2rmb/ploy/internal/testutil"
)

// ServiceContract defines the contract between services
type ServiceContract struct {
    ServiceName string
    Version     string
    Endpoints   []EndpointContract
    Events      []EventContract
}

// EndpointContract defines API endpoint contract
type EndpointContract struct {
    Method      string
    Path        string
    RequestSchema  interface{}
    ResponseSchema interface{}
    StatusCodes    []int
}

// EventContract defines event publishing/consuming contract
type EventContract struct {
    Name       string
    Version    string
    Schema     interface{}
    Publisher  string
    Consumers  []string
}

// TestStorageServiceContract tests storage service contracts
func TestStorageServiceContract(t *testing.T) {
    contract := ServiceContract{
        ServiceName: "storage",
        Version:     "1.0",
        Endpoints: []EndpointContract{
            {
                Method: "POST",
                Path:   "/upload",
                RequestSchema: map[string]interface{}{
                    "key":  "string",
                    "data": "binary",
                },
                ResponseSchema: map[string]interface{}{
                    "fid": "string",
                    "url": "string",
                },
                StatusCodes: []int{201, 400, 500},
            },
            {
                Method: "GET",
                Path:   "/download/{key}",
                ResponseSchema: "binary",
                StatusCodes:    []int{200, 404, 500},
            },
        },
    }
    
    // Test contract compliance
    testServiceContract(t, contract)
}

// TestBuildServiceContract tests build service contracts  
func TestBuildServiceContract(t *testing.T) {
    contract := ServiceContract{
        ServiceName: "build",
        Version:     "1.0",
        Endpoints: []EndpointContract{
            {
                Method: "POST",
                Path:   "/v1/apps/{app}/builds",
                RequestSchema: map[string]interface{}{
                    "git_url": "string",
                    "branch":  "string",
                    "lane":    "string", // optional
                },
                ResponseSchema: map[string]interface{}{
                    "status":   "string",
                    "build_id": "string",
                },
                StatusCodes: []int{202, 400, 500},
            },
            {
                Method: "GET",
                Path:   "/v1/apps/{app}/status",
                ResponseSchema: map[string]interface{}{
                    "status":    "string",
                    "instances": "number",
                    "version":   "string",
                },
                StatusCodes: []int{200, 404},
            },
        },
        Events: []EventContract{
            {
                Name:      "build.started",
                Version:   "1.0",
                Publisher: "build-service",
                Schema: map[string]interface{}{
                    "app_name":  "string",
                    "build_id":  "string",
                    "timestamp": "datetime",
                },
            },
            {
                Name:      "build.completed",
                Version:   "1.0", 
                Publisher: "build-service",
                Schema: map[string]interface{}{
                    "app_name":    "string",
                    "build_id":    "string",
                    "status":      "string",
                    "artifact_id": "string",
                    "timestamp":   "datetime",
                },
            },
        },
    }
    
    testServiceContract(t, contract)
}

// testServiceContract validates a service contract
func testServiceContract(t *testing.T, contract ServiceContract) {
    t.Run("contract_"+contract.ServiceName, func(t *testing.T) {
        // Test all endpoints
        for _, endpoint := range contract.Endpoints {
            t.Run(fmt.Sprintf("%s_%s", endpoint.Method, endpoint.Path), func(t *testing.T) {
                testEndpointContract(t, endpoint)
            })
        }
        
        // Test all events
        for _, event := range contract.Events {
            t.Run("event_"+event.Name, func(t *testing.T) {
                testEventContract(t, event)
            })
        }
    })
}

// testEndpointContract validates an endpoint contract
func testEndpointContract(t *testing.T, contract EndpointContract) {
    client := testutil.NewAPITestClient(t, "http://localhost:8081")
    
    // Test with valid request
    req := client.Request(contract.Method, contract.Path)
    
    if contract.RequestSchema != nil {
        // Generate valid request from schema
        validRequest := generateFromSchema(contract.RequestSchema)
        req = req.WithJSON(validRequest)
    }
    
    resp := req.Execute()
    
    // Verify status code is in allowed list
    assert.Contains(t, contract.StatusCodes, resp.StatusCode,
        "Status code should be in contract")
    
    // Verify response schema (if success status)
    if resp.StatusCode < 300 && contract.ResponseSchema != nil {
        validateResponseSchema(t, resp.Body, contract.ResponseSchema)
    }
}

// testEventContract validates an event contract
func testEventContract(t *testing.T, contract EventContract) {
    // This would integrate with event system testing
    // For now, validate schema structure
    assert.NotEmpty(t, contract.Name, "Event name should not be empty")
    assert.NotEmpty(t, contract.Publisher, "Publisher should be specified")
    assert.NotNil(t, contract.Schema, "Event schema should be defined")
}

// Schema validation helpers (simplified implementation)
func generateFromSchema(schema interface{}) interface{} {
    // This would be a more sophisticated schema-to-data generator
    if schemaMap, ok := schema.(map[string]interface{}); ok {
        result := make(map[string]interface{})
        for key, fieldType := range schemaMap {
            switch fieldType {
            case "string":
                result[key] = "test_" + key
            case "number":
                result[key] = 42
            case "boolean":
                result[key] = true
            case "binary":
                result[key] = []byte("test data")
            }
        }
        return result
    }
    return nil
}

func validateResponseSchema(t *testing.T, body []byte, schema interface{}) {
    // This would validate the response against the schema
    // Simplified implementation for now
    var responseData interface{}
    err := json.Unmarshal(body, &responseData)
    require.NoError(t, err, "Response should be valid JSON")
    
    if schemaMap, ok := schema.(map[string]interface{}); ok {
        responseMap, ok := responseData.(map[string]interface{})
        require.True(t, ok, "Response should be a JSON object")
        
        for key, expectedType := range schemaMap {
            assert.Contains(t, responseMap, key, "Response should contain field: "+key)
            
            // Validate field type (simplified)
            value := responseMap[key]
            switch expectedType {
            case "string":
                assert.IsType(t, "", value, "Field "+key+" should be string")
            case "number":
                assert.IsType(t, float64(0), value, "Field "+key+" should be number")
            }
        }
    }
}
```

### 4. Performance Integration Tests

#### 4.1 Load Testing Framework (`load_test.go`)
```go
//go:build integration
// +build integration

package performance

import (
    "context"
    "sync"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/iw2rmb/ploy/internal/testutil/api"
)

// LoadTestConfig defines load testing parameters
type LoadTestConfig struct {
    Duration     time.Duration
    Concurrency  int
    RequestRate  int // requests per second
    RampUpTime   time.Duration
}

// LoadTestResult contains load test metrics
type LoadTestResult struct {
    TotalRequests    int
    SuccessfulReqs   int
    FailedRequests   int
    AverageLatency   time.Duration
    P95Latency       time.Duration
    P99Latency       time.Duration
    ErrorRate        float64
}

// TestAPILoadPerformance tests API performance under load
func TestAPILoadPerformance(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping load tests in short mode")
    }
    
    config := LoadTestConfig{
        Duration:    30 * time.Second,
        Concurrency: 10,
        RequestRate: 50, // 50 RPS
        RampUpTime:  5 * time.Second,
    }
    
    result := runLoadTest(t, config)
    
    // Assert performance requirements
    assert.Less(t, result.ErrorRate, 0.01, "Error rate should be less than 1%")
    assert.Less(t, result.AverageLatency, 200*time.Millisecond, 
        "Average latency should be less than 200ms")
    assert.Less(t, result.P95Latency, 500*time.Millisecond,
        "P95 latency should be less than 500ms")
    assert.Less(t, result.P99Latency, 1*time.Second,
        "P99 latency should be less than 1s")
}

// runLoadTest executes a load test with the given configuration
func runLoadTest(t *testing.T, config LoadTestConfig) *LoadTestResult {
    client := api.NewTestClient(t, "http://localhost:8081")
    
    // Channels for coordination
    start := make(chan struct{})
    results := make(chan requestResult, config.Concurrency*1000)
    done := make(chan struct{})
    
    var wg sync.WaitGroup
    
    // Start worker goroutines
    for i := 0; i < config.Concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            loadTestWorker(client, start, results, config)
        }()
    }
    
    // Collect results
    go func() {
        wg.Wait()
        close(results)
        done <- struct{}{}
    }()
    
    // Start load test
    close(start)
    
    // Wait for completion or timeout
    select {
    case <-done:
        // Test completed normally
    case <-time.After(config.Duration + 10*time.Second):
        t.Fatal("Load test timed out")
    }
    
    // Calculate metrics
    return calculateLoadTestMetrics(results)
}

type requestResult struct {
    success bool
    latency time.Duration
    error   error
}

// loadTestWorker performs requests for the duration of the test
func loadTestWorker(client *api.TestClient, start <-chan struct{}, 
                   results chan<- requestResult, config LoadTestConfig) {
    
    <-start // Wait for test to start
    
    endTime := time.Now().Add(config.Duration)
    requestInterval := time.Duration(int64(time.Second) / int64(config.RequestRate/config.Concurrency))
    
    ticker := time.NewTicker(requestInterval)
    defer ticker.Stop()
    
    for time.Now().Before(endTime) {
        select {
        case <-ticker.C:
            startTime := time.Now()
            
            // Make request to health endpoint (lightweight)
            _, err := client.GET("/health").Execute(), nil
            
            latency := time.Since(startTime)
            
            results <- requestResult{
                success: err == nil,
                latency: latency,
                error:   err,
            }
            
        case <-time.After(requestInterval * 2):
            // Timeout protection
            return
        }
    }
}

// calculateLoadTestMetrics computes performance metrics from results
func calculateLoadTestMetrics(results <-chan requestResult) *LoadTestResult {
    var (
        total      = 0
        successful = 0
        failed     = 0
        latencies  []time.Duration
    )
    
    // Collect all results
    for result := range results {
        total++
        latencies = append(latencies, result.latency)
        
        if result.success {
            successful++
        } else {
            failed++
        }
    }
    
    if total == 0 {
        return &LoadTestResult{}
    }
    
    // Sort latencies for percentile calculation
    sort.Slice(latencies, func(i, j int) bool {
        return latencies[i] < latencies[j]
    })
    
    // Calculate average latency
    var totalLatency time.Duration
    for _, latency := range latencies {
        totalLatency += latency
    }
    avgLatency := totalLatency / time.Duration(len(latencies))
    
    // Calculate percentiles
    p95Index := int(float64(len(latencies)) * 0.95)
    p99Index := int(float64(len(latencies)) * 0.99)
    
    var p95Latency, p99Latency time.Duration
    if len(latencies) > 0 {
        if p95Index >= len(latencies) {
            p95Index = len(latencies) - 1
        }
        if p99Index >= len(latencies) {
            p99Index = len(latencies) - 1
        }
        
        p95Latency = latencies[p95Index]
        p99Latency = latencies[p99Index]
    }
    
    errorRate := float64(failed) / float64(total)
    
    return &LoadTestResult{
        TotalRequests:  total,
        SuccessfulReqs: successful,
        FailedRequests: failed,
        AverageLatency: avgLatency,
        P95Latency:     p95Latency,
        P99Latency:     p99Latency,
        ErrorRate:      errorRate,
    }
}

// TestConcurrentBuildRequests tests build endpoint under concurrent load
func TestConcurrentBuildRequests(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping concurrent build tests in short mode")
    }
    
    client := api.NewTestClient(t, "http://localhost:8081")
    numConcurrent := 5
    
    var wg sync.WaitGroup
    results := make(chan error, numConcurrent)
    
    buildRequest := map[string]interface{}{
        "git_url": "https://github.com/test/go-app.git",
        "branch":  "main",
    }
    
    // Start concurrent build requests
    for i := 0; i < numConcurrent; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            
            appName := fmt.Sprintf("load-test-app-%d", id)
            
            _, err := client.POST("/v1/apps/"+appName+"/builds").
                WithJSON(buildRequest).
                ExpectStatus(202).
                Execute(), nil
            
            results <- err
        }(i)
    }
    
    wg.Wait()
    close(results)
    
    // Check all requests succeeded
    for err := range results {
        assert.NoError(t, err, "Concurrent build request should succeed")
    }
}
```

## Implementation Checklist

### Phase 1 Tasks
- [ ] **API Testing Framework**
  - [ ] HTTP test client with fluent interface
  - [ ] Request builders and response validators
  - [ ] JSON path assertions and schema validation
  - [ ] Comprehensive API test scenarios

- [ ] **Service Integration Tests**
  - [ ] Build pipeline integration with real services
  - [ ] Storage service integration tests
  - [ ] Environment variable service tests
  - [ ] Error handling and timeout scenarios

### Phase 2 Tasks  
- [ ] **Contract Testing**
  - [ ] Service contract definitions
  - [ ] Endpoint contract validation
  - [ ] Event contract testing framework
  - [ ] Inter-service communication validation

- [ ] **Performance Integration**
  - [ ] Load testing framework
  - [ ] Concurrent request handling tests
  - [ ] Latency and throughput benchmarks
  - [ ] Performance regression detection

## Success Criteria

### API Coverage
- [ ] **100% Endpoint Coverage**: All API endpoints tested
- [ ] **Request/Response Validation**: Schema compliance verified
- [ ] **Error Scenarios**: All error paths tested
- [ ] **Authentication/Authorization**: Security tested (when implemented)

### Integration Quality  
- [ ] **Service Integration**: All service boundaries tested
- [ ] **Contract Compliance**: Service contracts validated
- [ ] **Data Flow**: End-to-end data flow verified
- [ ] **Error Propagation**: Error handling across services tested

### Performance Metrics
- [ ] **Response Time**: API responses < 200ms average
- [ ] **Throughput**: Handle 50+ RPS with 10 concurrent users
- [ ] **Error Rate**: < 1% error rate under normal load
- [ ] **Stability**: No memory leaks or resource exhaustion

### Test Execution
- [ ] **Fast Execution**: Integration tests < 5 minutes
- [ ] **Reliable**: < 1% flaky test rate
- [ ] **Isolated**: Tests don't interfere with each other
- [ ] **Reproducible**: Tests produce consistent results

## Risk Mitigation

### Technical Risks
1. **Slow Test Execution**
   - Mitigation: Optimize test data, parallel execution, fast teardown
   
2. **Service Dependencies**
   - Mitigation: Docker containers, health checks, timeouts

3. **Test Data Management** 
   - Mitigation: Isolated test databases, cleanup automation

### Infrastructure Risks
1. **Docker Resource Usage**
   - Mitigation: Resource limits, cleanup procedures
   
2. **Port Conflicts**
   - Mitigation: Dynamic port allocation, proper cleanup

## Dependencies

### Prerequisites from Phase 2
- Unit testing framework
- Mock implementations
- Test utilities package
- Coverage reporting setup

### External Dependencies
- Docker and Docker Compose
- Local service stack (Consul, Nomad, SeaweedFS)
- Test database instances
- Network configuration

## Next Steps

After completing Phase 3:
1. **Phase 4**: Behavioral & E2E Testing
2. **Performance Optimization**: Based on integration test findings  
3. **Service Refactoring**: Improve service boundaries based on contract tests
4. **Monitoring Integration**: Add observability to integration tests
5. **Documentation Updates**: Document integration testing patterns

## References

- [Integration Testing Best Practices](https://martinfowler.com/articles/microservice-testing/)
- [Contract Testing with Pact](https://docs.pact.io/)
- [API Testing Strategies](https://restfulapi.net/testing/)
- [Load Testing Guidelines](https://k6.io/docs/testing-guides/)
- [Docker Testing Patterns](https://docs.docker.com/language/golang/)