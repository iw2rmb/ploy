package storage

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSeaweedFSClient_PutObject(t *testing.T) {
	tests := []struct {
		name        string
		bucket      string
		key         string
		body        string
		contentType string
		setupMock   func() *mockRoundTripper
		wantErr     bool
		errContains string
		wantSize    int64
		wantLoc     string
	}{
		{
			name:        "successful upload",
			bucket:      "test-bucket",
			key:         "test-key",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func() *mockRoundTripper {
				return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusCreated, body: `{"name":"test-key","size":12}`}}}
			},
			wantLoc: "test-bucket/test-key", wantSize: 12,
		},
		{
			name:        "upload with directory creation",
			bucket:      "test-bucket",
			key:         "path/to/file.txt",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func() *mockRoundTripper {
				return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusCreated, body: `{}`}, {statusCode: http.StatusCreated, body: `{"name":"file.txt","size":12}`}}}
			},
			wantLoc: "test-bucket/path/to/file.txt", wantSize: 12,
		},
		{
			name:        "server error",
			bucket:      "test-bucket",
			key:         "test-key",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func() *mockRoundTripper {
				return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusInternalServerError, body: "Internal Server Error"}}}
			},
			wantErr:     true,
			errContains: "upload failed: 500",
		},
		{
			name:        "network timeout",
			bucket:      "test-bucket",
			key:         "test-key",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func() *mockRoundTripper {
				return &mockRoundTripper{responses: []mockResponse{{err: errors.New("context deadline exceeded")}}}
			},
			wantErr:     true,
			errContains: "context deadline exceeded",
		},
		{
			name:        "invalid JSON response",
			bucket:      "test-bucket",
			key:         "test-key",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func() *mockRoundTripper {
				return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusCreated, body: "invalid json"}}}
			},
			wantErr:     true,
			errContains: "failed to parse response",
		},
		{
			name:        "directory creation failure",
			bucket:      "test-bucket",
			key:         "path/to/file.txt",
			body:        "test content",
			contentType: "text/plain",
			setupMock: func() *mockRoundTripper {
				return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusForbidden, body: "Permission denied"}}}
			},
			wantErr:     true,
			errContains: "failed to create directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			client := &SeaweedFSClient{masterURL: "http://localhost:9333", filerURL: "http://localhost:8888", collection: "test-collection", replication: "001", timeout: 30 * time.Second, httpClient: &http.Client{Transport: mock}}
			body := strings.NewReader(tt.body)
			result, err := client.PutObject(tt.bucket, tt.key, body, tt.contentType)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.wantLoc, result.Location)
				assert.Equal(t, tt.wantSize, result.Size)
			}
		})
	}
}
