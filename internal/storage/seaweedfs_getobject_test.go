package storage

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeaweedFSClient_GetObject(t *testing.T) {
	tests := []struct {
		name        string
		bucket      string
		key         string
		setupMock   func() *mockRoundTripper
		wantErr     bool
		errContains string
		wantBody    string
	}{
		{name: "successful retrieval", bucket: "test-bucket", key: "test-key", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusOK, body: "file content"}}}
		}, wantBody: "file content"},
		{name: "object not found", bucket: "test-bucket", key: "missing-key", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusNotFound, body: "Not Found"}}}
		}, wantErr: true, errContains: "failed to get object: 404"},
		{name: "network error", bucket: "test-bucket", key: "test-key", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{err: errors.New("connection refused")}}}
		}, wantErr: true, errContains: "connection refused"},
		{name: "server error", bucket: "test-bucket", key: "test-key", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusInternalServerError, body: "Internal Server Error"}}}
		}, wantErr: true, errContains: "failed to get object: 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			client := &SeaweedFSClient{filerURL: "http://localhost:8888", httpClient: &http.Client{Transport: mock}}
			reader, err := client.GetObject(tt.bucket, tt.key)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, reader)
				return
			}
			assert.NoError(t, err)
			defer func() { _ = reader.Close() }()
			content, _ := io.ReadAll(reader)
			assert.Equal(t, tt.wantBody, string(content))
		})
	}
}
