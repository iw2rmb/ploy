package arf

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/iw2rmb/ploy/internal/storage"
)

// mockSeaweedFS implements storage.StorageProvider for unit tests
type mockSeaweedFS struct{ data map[string][]byte }

func newMockSeaweed() *mockSeaweedFS { return &mockSeaweedFS{data: make(map[string][]byte)} }

func (m *mockSeaweedFS) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*storage.PutObjectResult, error) {
	b, _ := io.ReadAll(body)
	m.data[key] = b
	return &storage.PutObjectResult{Location: key, Size: int64(len(b))}, nil
}

func (m *mockSeaweedFS) GetObject(bucket, key string) (io.ReadCloser, error) {
	if b, ok := m.data[key]; ok {
		return io.NopCloser(bytes.NewReader(b)), nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockSeaweedFS) ListObjects(bucket, prefix string) ([]storage.ObjectInfo, error) {
	var out []storage.ObjectInfo
	for k, v := range m.data {
		if strings.HasPrefix(k, prefix) {
			out = append(out, storage.ObjectInfo{Key: k, Size: int64(len(v)), ContentType: "application/x-yaml"})
		}
	}
	return out, nil
}

func (m *mockSeaweedFS) UploadArtifactBundle(string, string) error { return nil }
func (m *mockSeaweedFS) UploadArtifactBundleWithVerification(string, string) (*storage.BundleIntegrityResult, error) {
	return &storage.BundleIntegrityResult{Verified: true}, nil
}
func (m *mockSeaweedFS) VerifyUpload(string) error  { return nil }
func (m *mockSeaweedFS) GetProviderType() string    { return "seaweedfs" }
func (m *mockSeaweedFS) GetArtifactsBucket() string { return "ploy-recipes" }
