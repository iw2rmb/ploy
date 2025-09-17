package storage

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeaweedFSClient_VerifyUpload(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		setupMock   func() *mockRoundTripper
		wantErr     bool
		errContains string
	}{
		{name: "successful verification", key: "bucket/key", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusOK, headers: map[string]string{"Content-Length": "1024"}}}}
		}},
		{name: "object not found", key: "bucket/missing", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusNotFound}}}
		}, wantErr: true, errContains: "object not found"},
		{name: "network error", key: "bucket/key", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{err: errors.New("connection refused")}}}
		}, wantErr: true, errContains: "connection refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			client := &SeaweedFSClient{filerURL: "http://localhost:8888", collection: "test-collection", httpClient: &http.Client{Transport: mock}}
			err := client.VerifyUpload(tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetProviderType(t *testing.T) {
	client := &SeaweedFSClient{}
	assert.Equal(t, "seaweedfs", client.GetProviderType())
}

func TestGetArtifactsBucket(t *testing.T) {
	client := &SeaweedFSClient{collection: "my-artifacts"}
	assert.Equal(t, "my-artifacts", client.GetArtifactsBucket())
}

func TestSeaweedFSClient_TestVolumeAssignment(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func() *mockRoundTripper
		wantErr     bool
		errContains string
	}{
		{name: "successful volume assignment", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusOK, body: `{"fid":"3,01234567","url":"localhost:8080","publicUrl":"localhost:8080","count":1}`}}}
		}},
		{name: "master server error", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusInternalServerError, body: "Server Error"}}}
		}, wantErr: true, errContains: "failed to assign volume"},
		{name: "network error", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{err: errors.New("connection refused")}}}
		}, wantErr: true, errContains: "connection refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			client := &SeaweedFSClient{masterURL: "http://localhost:9333", collection: "test-collection", httpClient: &http.Client{Transport: mock}}
			result, err := client.TestVolumeAssignment()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

// Retry/simple e2e
func TestSeaweedFSClient_Retry(t *testing.T) {
	mock := &mockRoundTripper{responses: []mockResponse{{err: errors.New("network error")}, {statusCode: http.StatusOK, body: "success"}}}
	client := &SeaweedFSClient{filerURL: "http://localhost:8888", httpClient: &http.Client{Transport: mock}}
	_, err := client.GetObject("bucket", "key")
	assert.Error(t, err)
	reader, err := client.GetObject("bucket", "key")
	assert.NoError(t, err)
	assert.NotNil(t, reader)
	_ = reader.Close()
}

func TestSeaweedFSClient_UploadArtifactBundleWithVerification(t *testing.T) {
	tempDir := t.TempDir()
	artifactPath := tempDir + "/artifact.tar"
	err := os.WriteFile(artifactPath, []byte("test content"), 0644)
	require.NoError(t, err)
	mock := &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusCreated, body: `{}`}, {statusCode: http.StatusCreated, body: `{"name":"artifact.tar","size":12}`}, {statusCode: http.StatusOK, body: "test content"}}}
	client := &SeaweedFSClient{filerURL: "http://localhost:8888", collection: "test-collection", replication: "001", httpClient: &http.Client{Transport: mock}}
	result, e := client.UploadArtifactBundleWithVerification("test-prefix", artifactPath)
	if e != nil {
		assert.Contains(t, e.Error(), "integrity verification failed")
	}
	assert.NotNil(t, result)
}

// Benchmarks
func BenchmarkSeaweedFSClient_PutObject(b *testing.B) {
	mock := &mockRoundTripper{responses: make([]mockResponse, b.N)}
	for i := 0; i < b.N; i++ {
		mock.responses[i] = mockResponse{statusCode: http.StatusCreated, body: `{"name":"test","size":100}`}
	}
	client := &SeaweedFSClient{filerURL: "http://localhost:8888", collection: "test-collection", replication: "001", httpClient: &http.Client{Transport: mock}}
	data := bytes.Repeat([]byte("x"), 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := bytes.NewReader(data)
		if _, err := client.PutObject("bucket", fmt.Sprintf("key-%d", i), reader, "application/octet-stream"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSeaweedFSClient_GetObject(b *testing.B) {
	mock := &mockRoundTripper{responses: make([]mockResponse, b.N)}
	for i := 0; i < b.N; i++ {
		mock.responses[i] = mockResponse{statusCode: http.StatusOK, body: "test content"}
	}
	client := &SeaweedFSClient{filerURL: "http://localhost:8888", httpClient: &http.Client{Transport: mock}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, err := client.GetObject("bucket", fmt.Sprintf("key-%d", i))
		if err != nil {
			b.Fatal(err)
		}
		_ = reader.Close()
	}
}
