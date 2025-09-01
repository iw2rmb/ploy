package mocks_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/iw2rmb/ploy/internal/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockStorageClient_BasicOperations(t *testing.T) {
	ctx := context.Background()

	t.Run("Upload and Download", func(t *testing.T) {
		client := mocks.NewStorageClient()
		data := []byte("test data")

		// Test Upload with io.Reader
		err := client.Upload(ctx, "test-key", bytes.NewReader(data))
		require.NoError(t, err)

		// Test Download
		reader, err := client.Download(ctx, "test-key")
		require.NoError(t, err)
		defer reader.Close()

		downloaded, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, data, downloaded)
	})

	t.Run("Exists", func(t *testing.T) {
		client := mocks.NewStorageClient()

		// Check non-existent key
		exists, err := client.Exists(ctx, "non-existent")
		require.NoError(t, err)
		assert.False(t, exists)

		// Upload and check existence
		err = client.Upload(ctx, "exists-key", bytes.NewReader([]byte("data")))
		require.NoError(t, err)

		exists, err = client.Exists(ctx, "exists-key")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("Delete", func(t *testing.T) {
		client := mocks.NewStorageClient()

		// Upload data
		err := client.Upload(ctx, "delete-key", bytes.NewReader([]byte("data")))
		require.NoError(t, err)

		// Verify it exists
		exists, err := client.Exists(ctx, "delete-key")
		require.NoError(t, err)
		assert.True(t, exists)

		// Delete it
		err = client.Delete(ctx, "delete-key")
		require.NoError(t, err)

		// Verify it's gone
		exists, err = client.Exists(ctx, "delete-key")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("List", func(t *testing.T) {
		client := mocks.NewStorageClient()

		// Upload multiple files
		files := map[string][]byte{
			"prefix/file1.txt": []byte("content1"),
			"prefix/file2.txt": []byte("content2"),
			"other/file3.txt":  []byte("content3"),
		}

		for key, data := range files {
			err := client.Upload(ctx, key, bytes.NewReader(data))
			require.NoError(t, err)
		}

		// List with prefix
		keys, err := client.List(ctx, "prefix/")
		require.NoError(t, err)
		assert.Len(t, keys, 2)
		assert.Contains(t, keys, "prefix/file1.txt")
		assert.Contains(t, keys, "prefix/file2.txt")
	})
}

func TestMockStorageClient_HelperMethods(t *testing.T) {
	ctx := context.Background()

	t.Run("WithFile", func(t *testing.T) {
		client := mocks.NewStorageClient()
		data := []byte("preset data")

		// Use WithFile to preset data
		client.WithFile("preset-key", data)

		// Verify data is available
		reader, err := client.Download(ctx, "preset-key")
		require.NoError(t, err)
		defer reader.Close()

		downloaded, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Equal(t, data, downloaded)

		// Verify exists returns true
		exists, err := client.Exists(ctx, "preset-key")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("WithError", func(t *testing.T) {
		client := mocks.NewStorageClient()
		expectedErr := assert.AnError

		// Setup error for specific operations
		client.WithError("error-key", expectedErr)

		// Download should return error
		_, err := client.Download(ctx, "error-key")
		assert.Equal(t, expectedErr, err)

		// Upload should return error
		err = client.Upload(ctx, "error-key", bytes.NewReader([]byte("data")))
		assert.Equal(t, expectedErr, err)
	})

	t.Run("Reset", func(t *testing.T) {
		client := mocks.NewStorageClient()

		// Add some data
		client.WithFile("key1", []byte("data1"))
		client.WithFile("key2", []byte("data2"))

		// Verify data exists
		exists, _ := client.Exists(ctx, "key1")
		assert.True(t, exists)

		// Reset
		client.Reset()

		// Verify data is cleared
		exists, _ = client.Exists(ctx, "key1")
		assert.False(t, exists)
		exists, _ = client.Exists(ctx, "key2")
		assert.False(t, exists)
	})
}

func TestMockStorageClient_MetricsAndHealth(t *testing.T) {
	t.Run("GetHealthStatus", func(t *testing.T) {
		client := mocks.NewStorageClient()

		// Default health status
		status := client.GetHealthStatus()
		assert.NotNil(t, status)

		// Can set custom health status
		customStatus := map[string]interface{}{"status": "healthy", "uptime": 1000}
		client.WithHealthStatus(customStatus)

		status = client.GetHealthStatus()
		assert.Equal(t, customStatus, status)
	})

	t.Run("GetMetrics", func(t *testing.T) {
		client := mocks.NewStorageClient()

		// Default metrics
		metrics := client.GetMetrics()
		assert.NotNil(t, metrics)

		// Can set custom metrics
		customMetrics := map[string]interface{}{"requests": 100, "errors": 2}
		client.WithMetrics(customMetrics)

		metrics = client.GetMetrics()
		assert.Equal(t, customMetrics, metrics)
	})
}

func TestMockStorageClient_ArtifactMethods(t *testing.T) {
	t.Run("GetArtifactsBucket", func(t *testing.T) {
		client := mocks.NewStorageClient()

		// Default bucket
		bucket := client.GetArtifactsBucket()
		assert.Equal(t, "artifacts", bucket)

		// Can set custom bucket
		client.WithArtifactsBucket("custom-bucket")
		bucket = client.GetArtifactsBucket()
		assert.Equal(t, "custom-bucket", bucket)
	})

	t.Run("UploadArtifactBundleWithVerification", func(t *testing.T) {
		client := mocks.NewStorageClient()

		// Successful upload
		result, err := client.UploadArtifactBundleWithVerification("prefix/", "/path/to/artifact")
		require.NoError(t, err)
		assert.NotNil(t, result)

		// Can configure to return error
		client.WithUploadArtifactError(assert.AnError)
		_, err = client.UploadArtifactBundleWithVerification("prefix/", "/path/to/artifact")
		assert.Equal(t, assert.AnError, err)
	})
}
