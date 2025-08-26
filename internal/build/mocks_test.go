package build

import (
	"archive/tar"
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	
	"github.com/iw2rmb/ploy/internal/storage"
)

// Mock storage client for testing
type MockStorageClient struct {
	mock.Mock
}

func (m *MockStorageClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*storage.PutObjectResult, error) {
	args := m.Called(bucket, key, body, contentType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.PutObjectResult), args.Error(1)
}

func (m *MockStorageClient) GetObject(bucket, key string) (io.ReadCloser, error) {
	args := m.Called(bucket, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockStorageClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	args := m.Called(keyPrefix, artifactPath)
	return args.Error(0)
}

func (m *MockStorageClient) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*storage.BundleIntegrityResult, error) {
	args := m.Called(keyPrefix, artifactPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.BundleIntegrityResult), args.Error(1)
}

func (m *MockStorageClient) VerifyUpload(key string) error {
	args := m.Called(key)
	return args.Error(0)
}

func (m *MockStorageClient) ListObjects(bucket, prefix string) ([]storage.ObjectInfo, error) {
	args := m.Called(bucket, prefix)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.ObjectInfo), args.Error(1)
}

func (m *MockStorageClient) GetProviderType() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockStorageClient) GetArtifactsBucket() string {
	args := m.Called()
	return args.String(0)
}

// Mock environment store for testing
type MockEnvStore struct {
	mock.Mock
}

func (m *MockEnvStore) Get(appName, key string) (string, bool, error) {
	args := m.Called(appName, key)
	return args.String(0), args.Bool(1), args.Error(2)
}

func (m *MockEnvStore) GetAll(appName string) (map[string]string, error) {
	args := m.Called(appName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

func (m *MockEnvStore) Set(appName, key, value string) error {
	args := m.Called(appName, key, value)
	return args.Error(0)
}

func (m *MockEnvStore) SetAll(appName string, envVars map[string]string) error {
	args := m.Called(appName, envVars)
	return args.Error(0)
}

func (m *MockEnvStore) Delete(appName, key string) error {
	args := m.Called(appName, key)
	return args.Error(0)
}

func (m *MockEnvStore) ToStringArray(appName string) ([]string, error) {
	args := m.Called(appName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

// Helper function to create test tarballs
func createTestTarball(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}
		err := tw.WriteHeader(hdr)
		require.NoError(t, err)
		
		_, err = tw.Write([]byte(content))
		require.NoError(t, err)
	}
	
	err := tw.Close()
	require.NoError(t, err)
	
	return buf.Bytes()
}