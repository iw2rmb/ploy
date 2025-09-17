package storage

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSeaweedFSClient_ListObjects(t *testing.T) {
	tests := []struct {
		name        string
		bucket      string
		prefix      string
		setupMock   func() *mockRoundTripper
		wantErr     bool
		errContains string
		wantObjects []ObjectInfo
	}{
		{name: "empty list", bucket: "test-bucket", prefix: "prefix/", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusOK, body: `{"Entries":[]}`}}}
		}, wantObjects: []ObjectInfo{}},
		{
			name:   "list with multiple objects",
			bucket: "test-bucket",
			prefix: "prefix/",
			setupMock: func() *mockRoundTripper {
				return &mockRoundTripper{responses: []mockResponse{{
					statusCode: http.StatusOK,
					body:       `{"Entries":[{"FullPath":"/test-bucket/prefix/file1.txt","FileSize":1024,"Mode":33188,"Mtime":"2024-01-01T12:00:00Z"},{"FullPath":"/test-bucket/prefix/file2.txt","FileSize":2048,"Mode":33188,"Mtime":"2024-01-02T12:00:00Z"}]}`,
				}}}
			},
			wantObjects: []ObjectInfo{
				{Key: "file1.txt", Size: 1024, LastModified: "2024-01-01T12:00:00Z", ContentType: "application/octet-stream"},
				{Key: "file2.txt", Size: 2048, LastModified: "2024-01-02T12:00:00Z", ContentType: "application/octet-stream"},
			},
		},
		{name: "server error", bucket: "test-bucket", prefix: "prefix/", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusInternalServerError, body: "Internal Server Error"}}}
		}, wantErr: true, errContains: "failed to list objects: 500"},
		{name: "invalid JSON response", bucket: "test-bucket", prefix: "prefix/", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusOK, body: "not json"}}}
		}, wantErr: true, errContains: "failed to decode response"},
		{name: "network error", bucket: "test-bucket", prefix: "prefix/", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{err: errors.New("network unreachable")}}}
		}, wantErr: true, errContains: "network unreachable"},
		{name: "directory not found returns empty list", bucket: "test-bucket", prefix: "non-existent/", setupMock: func() *mockRoundTripper {
			return &mockRoundTripper{responses: []mockResponse{{statusCode: http.StatusNotFound, body: "Not Found"}}}
		}, wantObjects: []ObjectInfo{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			client := &SeaweedFSClient{filerURL: "http://localhost:8888", httpClient: &http.Client{Transport: mock}}
			objects, err := client.ListObjects(tt.bucket, tt.prefix)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, objects)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, len(tt.wantObjects), len(objects))
			for i, want := range tt.wantObjects {
				assert.Equal(t, want.Key, objects[i].Key)
				assert.Equal(t, want.Size, objects[i].Size)
				assert.Equal(t, want.LastModified, objects[i].LastModified)
				assert.Equal(t, want.ContentType, objects[i].ContentType)
			}
		})
	}
}
