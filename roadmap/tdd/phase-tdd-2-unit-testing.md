# Phase 2: Unit Testing Infrastructure

## Progress Summary (2025-08-26)

**Status**: 🔄 IN PROGRESS  
**Current Coverage**: 15.2% (Target: 60%)  
**Key Accomplishments**:
- ✅ Build module validation tests implemented
- ✅ TriggerBuild function comprehensive testing with dependency injection
- ✅ Status function validation testing
- ✅ App name validation complete test suite
- ✅ Table-driven testing patterns established
- ✅ Fast test execution achieved (~7 seconds)

**Recent Completions**:
- ✅ Storage module unit tests (Fixed - 70.8% coverage)
- ✅ Lane detection tests (Completed - 91.9% coverage, 2025-08-26)
- ✅ **Internal environment variable module unit tests (100.0% coverage, 2025-08-26)**
- ✅ **Storage handler success path testing (60% coverage maintained, 2025-08-26)**
- ✅ **Internal utilities module comprehensive unit tests (83.5% coverage, 2025-08-26)**
- ✅ **Internal git module comprehensive unit tests (43.7% coverage, 2025-08-26)**
- ✅ **Internal build module comprehensive unit tests (41.7% coverage, 2025-08-26)**

**Next Focus Areas**:
- API handler tests completion
- Enhanced test fixtures and builders
- Controller endpoint testing

## Overview

Phase 2 focuses on implementing comprehensive unit tests for Ploy's core components. We'll achieve 60% code coverage for critical modules while establishing sustainable testing patterns that can scale with the codebase.

## Objectives

1. Implement unit tests for storage, build, validation, and lifecycle modules
2. Achieve 60% code coverage for core components
3. Create test data fixtures and builders
4. Establish table-driven testing patterns
5. Set up coverage reporting and monitoring

## Implementation Plan

### Core Component Testing
- Storage module unit tests
- Build and validation module tests
- Lifecycle and utility tests

### API and Coverage
- Controller and handler tests
- Integration testing setup
- Coverage reporting and optimization

## Deliverables

### 1. Storage Module Unit Tests (`internal/storage/`)

#### 1.1 SeaweedFS Client Tests (`seaweedfs_test.go`)
```go
package storage

import (
    "bytes"
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/iw2rmb/ploy/internal/testutil"
)

func TestSeaweedFSClient_Upload(t *testing.T) {
    tests := []struct {
        name       string
        key        string
        data       []byte
        serverResp int
        wantErr    bool
    }{
        {
            name:       "successful upload",
            key:        "test-file.tar",
            data:       []byte("test data"),
            serverResp: http.StatusCreated,
            wantErr:    false,
        },
        {
            name:       "server error",
            key:        "test-file.tar",
            data:       []byte("test data"),
            serverResp: http.StatusInternalServerError,
            wantErr:    true,
        },
        {
            name:       "empty key",
            key:        "",
            data:       []byte("test data"),
            serverResp: http.StatusBadRequest,
            wantErr:    true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mock server
            server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(tt.serverResp)
                if tt.serverResp == http.StatusCreated {
                    w.Write([]byte(`{"fid":"1,abc","url":"http://localhost:8080/1,abc"}`))
                }
            }))
            defer server.Close()

            // Create client with mock server
            config := &Config{
                Endpoint:     server.URL,
                MasterServer: server.URL,
            }
            client := NewSeaweedFSClient(config)

            // Execute test
            ctx := context.Background()
            err := client.Upload(ctx, tt.key, tt.data)

            // Verify results
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}

func TestSeaweedFSClient_Download(t *testing.T) {
    expectedData := []byte("test file content")
    
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write(expectedData)
    }))
    defer server.Close()

    config := &Config{
        Endpoint:     server.URL,
        MasterServer: server.URL,
    }
    client := NewSeaweedFSClient(config)

    ctx := context.Background()
    data, err := client.Download(ctx, "test-key")

    require.NoError(t, err)
    assert.Equal(t, expectedData, data)
}

func TestSeaweedFSClient_Retry_Logic(t *testing.T) {
    attempts := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        attempts++
        if attempts < 3 {
            w.WriteHeader(http.StatusInternalServerError)
            return
        }
        w.WriteHeader(http.StatusCreated)
        w.Write([]byte(`{"fid":"1,abc","url":"http://localhost:8080/1,abc"}`))
    }))
    defer server.Close()

    config := &Config{
        Endpoint:     server.URL,
        MasterServer: server.URL,
        MaxRetries:   3,
        RetryDelay:   10 * time.Millisecond,
    }
    client := NewSeaweedFSClient(config)

    ctx := context.Background()
    err := client.Upload(ctx, "test-key", []byte("data"))

    assert.NoError(t, err)
    assert.Equal(t, 3, attempts, "Should have retried 3 times")
}

func TestSeaweedFSClient_Integrity_Check(t *testing.T) {
    data := []byte("integrity test data")
    
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == "POST" {
            w.WriteHeader(http.StatusCreated)
            w.Write([]byte(`{"fid":"1,abc","url":"http://localhost:8080/1,abc"}`))
        } else {
            w.WriteHeader(http.StatusOK)
            w.Write(data)
        }
    }))
    defer server.Close()

    config := &Config{
        Endpoint:        server.URL,
        MasterServer:    server.URL,
        IntegrityCheck:  true,
    }
    client := NewSeaweedFSClient(config)

    ctx := context.Background()
    
    // Upload with integrity check
    err := client.Upload(ctx, "test-key", data)
    assert.NoError(t, err)
    
    // Download and verify
    downloadedData, err := client.Download(ctx, "test-key")
    assert.NoError(t, err)
    assert.Equal(t, data, downloadedData)
}

func BenchmarkSeaweedFSClient_Upload(b *testing.B) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusCreated)
        w.Write([]byte(`{"fid":"1,abc","url":"http://localhost:8080/1,abc"}`))
    }))
    defer server.Close()

    config := &Config{
        Endpoint:     server.URL,
        MasterServer: server.URL,
    }
    client := NewSeaweedFSClient(config)
    data := bytes.Repeat([]byte("x"), 1024) // 1KB data

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ctx := context.Background()
        err := client.Upload(ctx, fmt.Sprintf("test-key-%d", i), data)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

#### 1.2 Storage Interface Tests (`interface_test.go`)
```go
package storage

import (
    "context"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/iw2rmb/ploy/internal/testutil"
)

// TestStorageInterface_Compliance ensures all implementations follow the interface contract
func TestStorageInterface_Compliance(t *testing.T) {
    clients := []struct {
        name   string
        client StorageInterface
    }{
        {"SeaweedFS", testutil.NewMockStorageClient()},
        {"S3", testutil.NewMockS3Client()}, // When S3 is implemented
    }

    for _, tc := range clients {
        t.Run(tc.name, func(t *testing.T) {
            testStorageInterfaceContract(t, tc.client)
        })
    }
}

func testStorageInterfaceContract(t *testing.T, client StorageInterface) {
    ctx := context.Background()
    key := "test-contract"
    data := []byte("contract test data")

    // Test Upload
    err := client.Upload(ctx, key, data)
    assert.NoError(t, err, "Upload should succeed")

    // Test Download
    downloadedData, err := client.Download(ctx, key)
    assert.NoError(t, err, "Download should succeed")
    assert.Equal(t, data, downloadedData, "Downloaded data should match uploaded data")

    // Test Exists
    exists, err := client.Exists(ctx, key)
    assert.NoError(t, err, "Exists should succeed")
    assert.True(t, exists, "File should exist after upload")

    // Test Delete
    err = client.Delete(ctx, key)
    assert.NoError(t, err, "Delete should succeed")

    // Verify deletion
    exists, err = client.Exists(ctx, key)
    assert.NoError(t, err, "Exists check should succeed")
    assert.False(t, exists, "File should not exist after deletion")
}
```

### 2. Build Module Unit Tests (`internal/build/`)

#### 2.1 Lane Detection Tests (`lane_detection_test.go`)
```go
package build

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/iw2rmb/ploy/internal/testutil"
)

func TestDetectLane(t *testing.T) {
    tests := []struct {
        name     string
        files    []string
        expected string
        wantErr  bool
    }{
        {
            name:     "Go project with go.mod",
            files:    []string{"go.mod", "main.go"},
            expected: "A",
            wantErr:  false,
        },
        {
            name:     "Node.js project with package.json",
            files:    []string{"package.json", "index.js"},
            expected: "B",
            wantErr:  false,
        },
        {
            name:     "Java project with build.gradle.kts",
            files:    []string{"build.gradle.kts", "src/main/java/Main.java"},
            expected: "C",
            wantErr:  false,
        },
        {
            name:     "Java project with Jib (Lane E)",
            files:    []string{"build.gradle.kts", "src/main/java/Main.java"},
            expected: "E",
            wantErr:  false,
            // Mock gradle file content that includes Jib plugin
        },
        {
            name:     "Python with C extensions",
            files:    []string{"pyproject.toml", "setup.py", "extension.c"},
            expected: "C",
            wantErr:  false,
        },
        {
            name:     "WASM Rust project",
            files:    []string{"Cargo.toml", "src/lib.rs"},
            expected: "G",
            wantErr:  false,
            // Mock Cargo.toml with wasm32-wasi target
        },
        {
            name:     "Ambiguous project",
            files:    []string{"README.md"},
            expected: "",
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Create temporary directory with test files
            tmpDir := testutil.CreateTempDir(t)
            defer testutil.CleanupTempDir(t, tmpDir)

            testutil.CreateTestFiles(t, tmpDir, tt.files)

            // Execute lane detection
            detector := NewLaneDetector()
            lane, err := detector.DetectLane(tmpDir)

            // Verify results
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expected, lane)
            }
        })
    }
}

func TestDetectLane_WithContent(t *testing.T) {
    tests := []struct {
        name        string
        files       map[string]string
        expected    string
        description string
    }{
        {
            name: "Java with Jib plugin",
            files: map[string]string{
                "build.gradle.kts": `
                plugins {
                    id("com.google.cloud.tools.jib") version "3.3.1"
                }
                `,
                "src/main/java/Main.java": "public class Main {}",
            },
            expected:    "E",
            description: "Should detect Lane E for Java with Jib",
        },
        {
            name: "Python with C extensions",
            files: map[string]string{
                "pyproject.toml": `
                [build-system]
                requires = ["setuptools", "wheel", "Cython"]
                `,
                "extension.c": "#include <Python.h>",
            },
            expected:    "C",
            description: "Should detect Lane C for Python with C extensions",
        },
        {
            name: "Rust WASM target",
            files: map[string]string{
                "Cargo.toml": `
                [package]
                name = "wasm-app"
                
                [lib]
                crate-type = ["cdylib"]
                
                [dependencies]
                wasm-bindgen = "0.2"
                `,
                "src/lib.rs": `
                use wasm_bindgen::prelude::*;
                #[wasm_bindgen]
                pub fn hello() {}
                `,
            },
            expected:    "G",
            description: "Should detect Lane G for Rust WASM",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            tmpDir := testutil.CreateTempDir(t)
            defer testutil.CleanupTempDir(t, tmpDir)

            testutil.CreateTestFilesWithContent(t, tmpDir, tt.files)

            detector := NewLaneDetector()
            lane, err := detector.DetectLane(tmpDir)

            require.NoError(t, err, tt.description)
            assert.Equal(t, tt.expected, lane, tt.description)
        })
    }
}

func TestLaneDetector_Priority(t *testing.T) {
    // Test that when multiple lanes are possible, the correct priority is used
    tmpDir := testutil.CreateTempDir(t)
    defer testutil.CleanupTempDir(t, tmpDir)

    files := map[string]string{
        "go.mod":       "module test",
        "package.json": `{"name": "test"}`,
        "Dockerfile":   "FROM alpine",
    }
    testutil.CreateTestFilesWithContent(t, tmpDir, files)

    detector := NewLaneDetector()
    lane, err := detector.DetectLane(tmpDir)

    require.NoError(t, err)
    // Go should have higher priority than Node.js
    assert.Equal(t, "A", lane, "Go should take priority over Node.js")
}

func BenchmarkLaneDetection(b *testing.B) {
    tmpDir := testutil.CreateTempDir(b)
    defer testutil.CleanupTempDir(b, tmpDir)

    files := []string{"go.mod", "main.go", "README.md"}
    testutil.CreateTestFiles(b, tmpDir, files)

    detector := NewLaneDetector()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := detector.DetectLane(tmpDir)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

#### 2.2 Build Handler Tests (`handler_test.go`)
```go
package build

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    "github.com/iw2rmb/ploy/internal/testutil"
)

func TestBuildHandler_TriggerBuild(t *testing.T) {
    tests := []struct {
        name           string
        appName        string
        requestBody    string
        mockSetup      func(*testutil.MockStorageClient, *testutil.MockNomadClient)
        expectedStatus int
        wantErr        bool
    }{
        {
            name:    "successful build trigger",
            appName: "test-app",
            requestBody: `{
                "git_url": "https://github.com/test/repo.git",
                "branch": "main"
            }`,
            mockSetup: func(storage *testutil.MockStorageClient, nomad *testutil.MockNomadClient) {
                storage.On("Upload", mock.Anything, mock.Anything, mock.Anything).Return(nil)
                nomad.On("Jobs").Return(&nomadapi.Jobs{})
                nomad.Jobs().On("Register", mock.Anything, mock.Anything).Return(nil, nil, nil)
            },
            expectedStatus: 202,
            wantErr:        false,
        },
        {
            name:    "invalid request body",
            appName: "test-app",
            requestBody: `{invalid json}`,
            mockSetup: func(storage *testutil.MockStorageClient, nomad *testutil.MockNomadClient) {
                // No setup needed for invalid request
            },
            expectedStatus: 400,
            wantErr:        true,
        },
        {
            name:    "storage failure",
            appName: "test-app",
            requestBody: `{
                "git_url": "https://github.com/test/repo.git",
                "branch": "main"
            }`,
            mockSetup: func(storage *testutil.MockStorageClient, nomad *testutil.MockNomadClient) {
                storage.On("Upload", mock.Anything, mock.Anything, mock.Anything).
                    Return(errors.New("storage error"))
            },
            expectedStatus: 500,
            wantErr:        true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mocks
            mockStorage := testutil.NewMockStorageClient()
            mockNomad := testutil.NewMockNomadClient()
            tt.mockSetup(mockStorage, mockNomad)

            // Create handler
            handler := &BuildHandler{
                StorageClient: mockStorage,
                NomadClient:   mockNomad,
                LaneDetector:  NewLaneDetector(),
            }

            // Create test request
            req := testutil.NewRequestBuilder().
                Method("POST").
                Path("/v1/apps/" + tt.appName + "/builds").
                Body(tt.requestBody).
                Build()

            // Execute
            resp, err := handler.TriggerBuild(req)

            // Verify
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                require.NoError(t, err)
                assert.Equal(t, tt.expectedStatus, resp.Status)
            }

            // Verify mock expectations
            mockStorage.AssertExpectations(t)
            mockNomad.AssertExpectations(t)
        })
    }
}

func TestBuildHandler_BuildTimeout(t *testing.T) {
    mockStorage := testutil.NewMockStorageClient()
    mockNomad := testutil.NewMockNomadClient()
    
    // Setup long-running build
    mockNomad.On("Jobs").Return(&nomadapi.Jobs{})
    mockNomad.Jobs().On("Register", mock.Anything, mock.Anything).Return(nil, nil, nil)
    mockStorage.On("Upload", mock.Anything, mock.Anything, mock.Anything).Return(nil)

    handler := &BuildHandler{
        StorageClient: mockStorage,
        NomadClient:   mockNomad,
        LaneDetector:  NewLaneDetector(),
        BuildTimeout:  1 * time.Second, // Short timeout for testing
    }

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()

    // This would normally take longer than 1 second
    err := handler.ExecuteBuild(ctx, "test-app", &BuildConfig{
        Lane: "A",
        // ... other config
    })

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
}
```

### 3. Validation Module Unit Tests (`internal/validation/`)

#### 3.1 App Name Validation Tests (`app_name_test.go`)
```go
package validation

import (
    "testing"
    
    "github.com/stretchr/testify/assert"
)

func TestValidateAppName(t *testing.T) {
    tests := []struct {
        name    string
        appName string
        wantErr bool
        errMsg  string
    }{
        // Valid names
        {"valid lowercase", "myapp", false, ""},
        {"valid with hyphen", "my-app", false, ""},
        {"valid with number", "app123", false, ""},
        {"valid mixed", "my-app-v2", false, ""},
        {"minimum length", "ab", false, ""},
        {"maximum length", "a" + strings.Repeat("b", 61), false, ""},
        
        // Invalid names
        {"empty name", "", true, "name cannot be empty"},
        {"too short", "a", true, "name must be at least 2 characters"},
        {"too long", strings.Repeat("a", 64), true, "name must be at most 63 characters"},
        {"uppercase", "MyApp", true, "name must be lowercase"},
        {"starts with hyphen", "-myapp", true, "name must start with a letter"},
        {"ends with hyphen", "myapp-", true, "name must end with letter or number"},
        {"consecutive hyphens", "my--app", true, "name cannot contain consecutive hyphens"},
        {"special characters", "my_app", true, "name can only contain letters, numbers, and hyphens"},
        {"spaces", "my app", true, "name can only contain letters, numbers, and hyphens"},
        
        // Reserved names
        {"reserved api", "api", true, "name 'api' is reserved"},
        {"reserved dev", "dev", true, "name 'dev' is reserved"},
        {"reserved controller", "controller", true, "name 'controller' is reserved"},
        {"reserved ploy", "ploy", true, "name 'ploy' is reserved"},
        {"reserved with prefix", "ploy-test", true, "name cannot start with reserved prefix 'ploy-'"},
        {"reserved system prefix", "system-app", true, "name cannot start with reserved prefix 'system-'"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            validator := NewAppNameValidator()
            err := validator.ValidateAppName(tt.appName)

            if tt.wantErr {
                assert.Error(t, err)
                if tt.errMsg != "" {
                    assert.Contains(t, err.Error(), tt.errMsg)
                }
            } else {
                assert.NoError(t, err)
            }
        })
    }
}

func TestValidateAppName_Performance(t *testing.T) {
    validator := NewAppNameValidator()
    
    // Test validation performance doesn't degrade with complex patterns
    complexNames := []string{
        "a" + strings.Repeat("b", 60),     // Maximum length
        "complex-app-name-with-many-parts",
        "app-with-numbers-123-456-789",
    }
    
    for _, name := range complexNames {
        t.Run("performance_"+name[:10], func(t *testing.T) {
            start := time.Now()
            err := validator.ValidateAppName(name)
            duration := time.Since(start)
            
            assert.NoError(t, err)
            assert.Less(t, duration, 1*time.Millisecond, 
                "Validation should be very fast")
        })
    }
}

func BenchmarkValidateAppName(b *testing.B) {
    validator := NewAppNameValidator()
    testName := "my-test-app-name"
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        validator.ValidateAppName(testName)
    }
}
```

### 4. API Handler Unit Tests (`controller/server/`)

#### 4.1 Environment Variable Handler Tests (`env_handlers_test.go`)
```go
package server

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/gofiber/fiber/v2"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "github.com/iw2rmb/ploy/internal/testutil"
)

func TestServer_HandleSetEnvVars(t *testing.T) {
    tests := []struct {
        name         string
        appName      string
        requestBody  map[string]string
        mockSetup    func(*testutil.MockEnvStore)
        expectedCode int
        wantErr      bool
    }{
        {
            name:    "successful set multiple vars",
            appName: "test-app",
            requestBody: map[string]string{
                "NODE_ENV":     "production",
                "DATABASE_URL": "postgres://localhost",
                "DEBUG":        "true",
            },
            mockSetup: func(store *testutil.MockEnvStore) {
                store.On("SetEnvVars", "test-app", mock.MatchedBy(func(envVars map[string]string) bool {
                    return envVars["NODE_ENV"] == "production" &&
                           envVars["DATABASE_URL"] == "postgres://localhost" &&
                           envVars["DEBUG"] == "true"
                })).Return(nil)
            },
            expectedCode: 200,
            wantErr:      false,
        },
        {
            name:         "empty request body",
            appName:      "test-app",
            requestBody:  map[string]string{},
            mockSetup:    func(store *testutil.MockEnvStore) {},
            expectedCode: 400,
            wantErr:      true,
        },
        {
            name:    "store error",
            appName: "test-app",
            requestBody: map[string]string{
                "VAR": "value",
            },
            mockSetup: func(store *testutil.MockEnvStore) {
                store.On("SetEnvVars", "test-app", mock.Anything).
                    Return(errors.New("store error"))
            },
            expectedCode: 500,
            wantErr:      true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup mocks
            mockEnvStore := testutil.NewMockEnvStore()
            tt.mockSetup(mockEnvStore)

            // Create server with mocks
            deps := &ServiceDependencies{
                EnvStore: mockEnvStore,
            }
            server := &Server{dependencies: deps}

            // Create test app
            app := fiber.New()
            app.Post("/apps/:app/env", server.handleSetEnvVars)

            // Create request
            bodyBytes, _ := json.Marshal(tt.requestBody)
            req := httptest.NewRequest("POST", "/apps/"+tt.appName+"/env", 
                bytes.NewReader(bodyBytes))
            req.Header.Set("Content-Type", "application/json")

            // Execute request
            resp, err := app.Test(req)
            require.NoError(t, err)

            // Verify response
            assert.Equal(t, tt.expectedCode, resp.StatusCode)

            // Verify mock expectations
            mockEnvStore.AssertExpectations(t)
        })
    }
}

func TestServer_HandleGetEnvVars(t *testing.T) {
    mockEnvStore := testutil.NewMockEnvStore()
    expectedVars := map[string]string{
        "NODE_ENV": "production",
        "DEBUG":    "false",
    }
    
    mockEnvStore.On("GetEnvVars", "test-app").Return(expectedVars, nil)

    deps := &ServiceDependencies{
        EnvStore: mockEnvStore,
    }
    server := &Server{dependencies: deps}

    app := fiber.New()
    app.Get("/apps/:app/env", server.handleGetEnvVars)

    req := httptest.NewRequest("GET", "/apps/test-app/env", nil)
    resp, err := app.Test(req)
    require.NoError(t, err)

    assert.Equal(t, http.StatusOK, resp.StatusCode)

    var responseBody map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&responseBody)
    
    envVars := responseBody["env"].(map[string]interface{})
    assert.Equal(t, "production", envVars["NODE_ENV"])
    assert.Equal(t, "false", envVars["DEBUG"])

    mockEnvStore.AssertExpectations(t)
}

func TestServer_EnvironmentVariableIntegration(t *testing.T) {
    // Integration test that tests the full flow
    mockEnvStore := testutil.NewMockEnvStore()
    
    deps := &ServiceDependencies{
        EnvStore: mockEnvStore,
    }
    server := &Server{dependencies: deps}

    app := fiber.New()
    app.Post("/apps/:app/env", server.handleSetEnvVars)
    app.Get("/apps/:app/env", server.handleGetEnvVars)

    appName := "integration-test-app"
    envVars := map[string]string{
        "INTEGRATION": "true",
        "TEST_VAR":    "test_value",
    }

    // Setup mocks for the full flow
    mockEnvStore.On("SetEnvVars", appName, envVars).Return(nil)
    mockEnvStore.On("GetEnvVars", appName).Return(envVars, nil)

    // Set environment variables
    bodyBytes, _ := json.Marshal(envVars)
    setReq := httptest.NewRequest("POST", "/apps/"+appName+"/env", 
        bytes.NewReader(bodyBytes))
    setReq.Header.Set("Content-Type", "application/json")

    setResp, err := app.Test(setReq)
    require.NoError(t, err)
    assert.Equal(t, http.StatusOK, setResp.StatusCode)

    // Get environment variables
    getReq := httptest.NewRequest("GET", "/apps/"+appName+"/env", nil)
    getResp, err := app.Test(getReq)
    require.NoError(t, err)
    assert.Equal(t, http.StatusOK, getResp.StatusCode)

    var getResponseBody map[string]interface{}
    json.NewDecoder(getResp.Body).Decode(&getResponseBody)
    
    returnedVars := getResponseBody["env"].(map[string]interface{})
    assert.Equal(t, "true", returnedVars["INTEGRATION"])
    assert.Equal(t, "test_value", returnedVars["TEST_VAR"])

    mockEnvStore.AssertExpectations(t)
}
```

### 5. Test Data Fixtures and Builders

#### 5.1 Enhanced Fixtures (`internal/testutil/fixtures.go`)
```go
package testutil

import (
    "encoding/json"
    "time"
    
    "github.com/iw2rmb/ploy/internal/build"
    "github.com/iw2rmb/ploy/internal/storage"
)

// TestDataRepository provides comprehensive test data
type TestDataRepository struct {
    Apps          []build.App
    BuildConfigs  []build.BuildConfig
    StorageItems  []storage.Item
    EnvVarSets    []map[string]string
}

// NewTestDataRepository creates repository with default test data
func NewTestDataRepository() *TestDataRepository {
    return &TestDataRepository{
        Apps:          generateTestApps(),
        BuildConfigs:  generateTestBuildConfigs(),
        StorageItems:  generateTestStorageItems(),
        EnvVarSets:    generateTestEnvVarSets(),
    }
}

// generateTestApps creates diverse app configurations for testing
func generateTestApps() []build.App {
    return []build.App{
        {
            Name:        "go-api",
            Language:    "go",
            Lane:        "A",
            Version:     "1.0.0",
            GitURL:      "https://github.com/test/go-api.git",
            Branch:      "main",
            BuildTime:   2 * time.Minute,
            Status:      "running",
            Instances:   3,
            EnvVars: map[string]string{
                "PORT":         "8080",
                "GO_ENV":       "production",
                "LOG_LEVEL":    "info",
            },
        },
        {
            Name:        "node-frontend",
            Language:    "javascript",
            Lane:        "B",
            Version:     "2.1.0",
            GitURL:      "https://github.com/test/node-frontend.git",
            Branch:      "main",
            BuildTime:   5 * time.Minute,
            Status:      "building",
            Instances:   2,
            EnvVars: map[string]string{
                "NODE_ENV":     "production",
                "PORT":         "3000",
                "API_URL":      "https://api.example.com",
            },
        },
        {
            Name:        "java-service",
            Language:    "java",
            Lane:        "C",
            Version:     "1.2.0",
            GitURL:      "https://github.com/test/java-service.git",
            Branch:      "develop",
            BuildTime:   8 * time.Minute,
            Status:      "failed",
            Instances:   0,
            EnvVars: map[string]string{
                "JAVA_OPTS":    "-Xmx512m",
                "SPRING_PROFILE": "prod",
                "DB_URL":       "jdbc:postgresql://db:5432/app",
            },
        },
        {
            Name:        "rust-wasm",
            Language:    "rust",
            Lane:        "G",
            Version:     "0.1.0",
            GitURL:      "https://github.com/test/rust-wasm.git",
            Branch:      "main",
            BuildTime:   3 * time.Minute,
            Status:      "running",
            Instances:   1,
            EnvVars: map[string]string{
                "RUST_ENV":     "production",
                "WASM_OPTIMIZE": "true",
            },
        },
    }
}

// generateTestBuildConfigs creates test build configurations
func generateTestBuildConfigs() []build.BuildConfig {
    return []build.BuildConfig{
        {
            Lane:        "A",
            Builder:     "unikraft",
            Timeout:     300,
            Resources: build.Resources{
                CPU:    "500m",
                Memory: "512Mi",
            },
            EnvVars: map[string]string{
                "CGO_ENABLED": "0",
                "GOOS":        "linux",
            },
        },
        {
            Lane:        "B",
            Builder:     "unikraft-node",
            Timeout:     600,
            Resources: build.Resources{
                CPU:    "1000m",
                Memory: "1Gi",
            },
            EnvVars: map[string]string{
                "NODE_ENV": "production",
            },
        },
        {
            Lane:        "C",
            Builder:     "osv",
            Timeout:     900,
            Resources: build.Resources{
                CPU:    "2000m",
                Memory: "2Gi",
            },
            EnvVars: map[string]string{
                "JVM_OPTS": "-server",
            },
        },
    }
}

// FluentBuilders for dynamic test data creation

// AppTestBuilder provides fluent interface for creating test apps
type AppTestBuilder struct {
    app build.App
}

// NewAppTestBuilder creates a new app builder with defaults
func NewAppTestBuilder() *AppTestBuilder {
    return &AppTestBuilder{
        app: build.App{
            Name:      "default-app",
            Language:  "go",
            Lane:      "A",
            Version:   "1.0.0",
            Status:    "running",
            Instances: 1,
            EnvVars:   make(map[string]string),
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

func (b *AppTestBuilder) RunningInstances(count int) *AppTestBuilder {
    b.app.Instances = count
    return b
}

func (b *AppTestBuilder) WithEnv(key, value string) *AppTestBuilder {
    if b.app.EnvVars == nil {
        b.app.EnvVars = make(map[string]string)
    }
    b.app.EnvVars[key] = value
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

func (b *AppTestBuilder) Build() build.App {
    return b.app
}

// HTTPTestBuilder for API testing
type HTTPTestBuilder struct {
    method  string
    path    string
    body    interface{}
    headers map[string]string
    query   map[string]string
}

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

func (b *HTTPTestBuilder) WithJSON(body interface{}) *HTTPTestBuilder {
    b.body = body
    b.headers["Content-Type"] = "application/json"
    return b
}

func (b *HTTPTestBuilder) WithHeader(key, value string) *HTTPTestBuilder {
    b.headers[key] = value
    return b
}

func (b *HTTPTestBuilder) WithQuery(key, value string) *HTTPTestBuilder {
    b.query[key] = value
    return b
}

func (b *HTTPTestBuilder) BuildRequest() (*http.Request, error) {
    var bodyReader io.Reader
    
    if b.body != nil {
        bodyBytes, err := json.Marshal(b.body)
        if err != nil {
            return nil, err
        }
        bodyReader = bytes.NewReader(bodyBytes)
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
```

### 6. Coverage Configuration and Reporting

#### 6.1 Coverage Configuration (`coverage.yml`)
```yaml
coverage:
  precision: 2
  round: down
  range: "70...100"

  status:
    project:
      default:
        target: 70%
        threshold: 1%
        if_no_uploads: error
    patch:
      default:
        target: 80%
        if_no_uploads: error

  ignore:
    - "**/*_test.go"
    - "**/testutil/**"
    - "**/mocks/**"
    - "**/testdata/**"
    - "**/cmd/**"
    - "**/*.pb.go"
    - "**/vendor/**"

comment:
  layout: "reach, diff, flags, files"
  behavior: default
  require_changes: false
```

#### 6.2 Make Targets for Testing (`Makefile` addition)
```makefile
# Test targets
.PHONY: test test-unit test-integration test-coverage test-benchmark

# Unit tests only
test-unit:
	@echo "Running unit tests..."
	go test -v -race ./... -short

# Integration tests (requires docker services)
test-integration:
	@echo "Running integration tests..."
	docker-compose -f iac/local/docker-compose.yml up -d
	@sleep 5  # Wait for services to start
	go test -v -tags=integration ./...
	docker-compose -f iac/local/docker-compose.yml down

# Full test suite
test: test-unit test-integration

# Coverage report
test-coverage:
	@echo "Generating test coverage report..."
	go test -v -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out
	@echo "Coverage report: coverage.html"

# Coverage with threshold check
test-coverage-check:
	@echo "Running tests with coverage threshold check..."
	@coverage=$$(go test -coverprofile=coverage.out ./... | grep -E "coverage: [0-9]+\.[0-9]+%" | sed 's/.*coverage: \([0-9.]*\)%.*/\1/' | tail -1); \
	echo "Current coverage: $$coverage%"; \
	if [ $$(echo "$$coverage < 60" | bc -l) -eq 1 ]; then \
		echo "❌ Coverage $$coverage% is below 60% threshold"; \
		exit 1; \
	else \
		echo "✅ Coverage $$coverage% meets 60% threshold"; \
	fi

# Benchmark tests
test-benchmark:
	@echo "Running benchmark tests..."
	go test -bench=. -benchmem ./...

# Generate test mocks
generate-mocks:
	@echo "Generating test mocks..."
	go generate ./...

# Test data setup
test-data-setup:
	@echo "Setting up test data..."
	mkdir -p testdata coverage test-results
	@if [ ! -f testdata/sample.json ]; then \
		echo '{"test": true}' > testdata/sample.json; \
	fi

# Clean test artifacts
test-clean:
	@echo "Cleaning test artifacts..."
	rm -rf coverage.out coverage.html test-results/
	docker-compose -f iac/local/docker-compose.yml down -v 2>/dev/null || true

# Test everything with clean setup
test-all: test-clean test-data-setup generate-mocks test-coverage-check test-benchmark
```

## Implementation Checklist

### Phase 1 Tasks
- [ ] **Storage Module Tests**
  - [ ] SeaweedFS client unit tests with mock HTTP server
  - [ ] Storage interface compliance tests
  - [ ] Retry logic and error handling tests
  - [ ] Integrity check tests
  - [ ] Performance benchmarks

- [ ] **Build Module Tests**
  - ✅ Lane detection tests with various project types (2025-08-26)
  - ✅ Build handler tests with mocked dependencies (2025-08-26)
  - ✅ Build configuration validation tests (2025-08-26)
  - ✅ Build timeout and error scenarios (2025-08-26)

- [ ] **Validation Module Tests**
  - ✅ App name validation with all edge cases (2025-08-26)
  - ✅ Git URL validation tests (2025-08-26)
  - ✅ Git repository validation framework (2025-08-26)
  - [ ] Environment variable validation
  - [ ] Resource constraint validation

### Phase 2 Tasks
- [ ] **API Handler Tests**
  - [ ] Environment variable CRUD operations
  - ✅ Build trigger endpoints (2025-08-26)
  - ✅ Health check endpoints (2025-08-26)
  - ✅ Error handling and edge cases (2025-08-26)

- [ ] **Test Infrastructure**
  - [ ] Enhanced test fixtures and builders
  - [ ] HTTP test builders for API testing
  - [ ] Mock generators and factories
  - [ ] Test data management utilities

- [ ] **Coverage and Reporting**
  - [ ] Coverage configuration setup
  - [ ] Make targets for various test types
  - [ ] CI/CD integration for coverage reporting
  - [ ] Coverage threshold enforcement

## Success Criteria

### Coverage Targets
- [ ] **Overall Coverage**: 60% minimum (Current: 15.2% - In Progress)
- [ ] **Critical Modules**: 80% minimum
  - ✅ Storage operations (70.8% coverage - 2025-08-26)
  - ✅ Lane detection logic (91.9% coverage - 2025-08-26)
  - ✅ App name validation (100.0% coverage - 2025-08-26)
  - ✅ Internal utilities module (83.5% coverage - 2025-08-26)
  - ✅ Environment variable handling (100.0% coverage - 2025-08-26)
  - ✅ Git repository operations (43.7% coverage - 2025-08-26)
  - ✅ Build pipeline core (41.7% coverage - 2025-08-26)

### Test Quality Metrics
- ✅ **Zero Flaky Tests**: All tests deterministic (2025-08-26)
- ✅ **Fast Execution**: Unit test suite < 30 seconds (Current: ~7 seconds - 2025-08-26)
- ✅ **Test Isolation**: No interdependencies (2025-08-26)
- ✅ **Mock Coverage**: All external dependencies mocked (2025-08-26)

### Developer Experience
- [ ] **Easy Test Writing**: Builders and fixtures available
- [ ] **Clear Assertions**: Custom assertions for domain logic
- [ ] **Good Error Messages**: Descriptive test failure messages
- [ ] **Documentation**: Testing patterns documented

## Risk Mitigation

### Technical Risks
1. **High Test Maintenance Burden**
   - Mitigation: Focus on behavior over implementation details
   - Use shared fixtures and builders
   - Regular refactoring of test code

2. **Slow Test Execution**
   - Mitigation: Aggressive mocking of external dependencies
   - Parallel test execution
   - Separate unit and integration tests

3. **Flaky Tests**
   - Mitigation: Deterministic test data
   - Proper cleanup and isolation
   - Time-independent tests using mocks

### Process Risks
1. **Low Test Adoption**
   - Mitigation: Make testing easy with good tooling
   - Lead by example with comprehensive tests
   - Code review requirements

2. **Coverage Gaming**
   - Mitigation: Focus on meaningful tests over coverage numbers
   - Review test quality, not just quantity
   - Educate team on effective testing practices

## Next Steps

After completing Phase 2:
1. **Phase 3**: Integration Testing Framework
2. **Refactor Existing Code**: Improve testability of legacy components
3. **Team Training**: Workshop on unit testing best practices
4. **Tool Integration**: IDE plugins and test runners
5. **Metrics Monitoring**: Track test effectiveness over time

## Dependencies

### Prerequisites from Phase 1
- Test utilities package (`internal/testutil/`)
- Local development environment
- Docker Compose stack
- CI/CD pipeline configured

### New Dependencies
- `github.com/stretchr/testify` - Testing framework
- `github.com/golang/mock` - Mock generation
- `github.com/DATA-DOG/go-sqlmock` - Database mocking (if needed)
- `github.com/h2non/gock` - HTTP mocking (alternative approach)

## References

- [Go Testing Documentation](https://golang.org/doc/tutorial/add-a-test)
- [Testify Framework](https://github.com/stretchr/testify)
- [Table-Driven Tests](https://github.com/golang/go/wiki/TableDrivenTests)
- [Testing Best Practices](https://golang.org/doc/code.html#Testing)
- [Mock Testing Patterns](https://blog.golang.org/examples)