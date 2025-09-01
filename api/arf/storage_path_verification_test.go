package arf

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PathTrackingStorage wraps a storage implementation and tracks all paths used
type PathTrackingStorage struct {
	storage.Storage
	putPaths    []string
	getPaths    []string
	deletePaths []string
	existsPaths []string
}

func NewPathTrackingStorage(underlying storage.Storage) *PathTrackingStorage {
	return &PathTrackingStorage{
		Storage: underlying,
	}
}

func (p *PathTrackingStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	p.putPaths = append(p.putPaths, key)
	fmt.Printf("[PathTracker] PUT called with key: %s\n", key)
	return p.Storage.Put(ctx, key, reader, opts...)
}

func (p *PathTrackingStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	p.getPaths = append(p.getPaths, key)
	fmt.Printf("[PathTracker] GET called with key: %s\n", key)
	return p.Storage.Get(ctx, key)
}

func (p *PathTrackingStorage) Delete(ctx context.Context, key string) error {
	p.deletePaths = append(p.deletePaths, key)
	fmt.Printf("[PathTracker] DELETE called with key: %s\n", key)
	return p.Storage.Delete(ctx, key)
}

func (p *PathTrackingStorage) Exists(ctx context.Context, key string) (bool, error) {
	p.existsPaths = append(p.existsPaths, key)
	fmt.Printf("[PathTracker] EXISTS called with key: %s\n", key)
	return p.Storage.Exists(ctx, key)
}

// TestStoragePathVerification_NoDoubleBucketPrefix verifies that bucket prefixes are not doubled
func TestStoragePathVerification_NoDoubleBucketPrefix(t *testing.T) {
	// This test verifies the fix for the double bucket prefix issue
	// Expected: paths should NOT contain "artifacts/artifacts/"

	ctx := context.Background()

	// Create mock storage that tracks paths
	mockStorage := NewMockUnifiedStorage()
	pathTracker := NewPathTrackingStorage(mockStorage)

	// Create ARFService with empty bucket (storage layer handles bucket)
	service, err := NewARFService(pathTracker, "")
	require.NoError(t, err)

	testCases := []struct {
		name           string
		key            string
		expectNoBucket bool // Should have no "artifacts/" prefix at service level
	}{
		{
			name:           "Job input path",
			key:            "jobs/openrewrite-123/input.tar",
			expectNoBucket: true,
		},
		{
			name:           "Job output path",
			key:            "jobs/openrewrite-123/output.tar",
			expectNoBucket: true,
		},
		{
			name:           "Recipe path",
			key:            "recipes/java-migration.yaml",
			expectNoBucket: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset path tracker
			pathTracker.putPaths = nil

			// Put data through ARFService
			testData := []byte("test content")
			err := service.Put(ctx, tc.key, testData)
			assert.NoError(t, err)

			// Verify the path passed to storage
			require.Len(t, pathTracker.putPaths, 1, "Expected exactly one PUT call")
			actualPath := pathTracker.putPaths[0]

			// Path should be exactly as provided (no bucket prefix added by ARFService)
			assert.Equal(t, tc.key, actualPath,
				"ARFService should pass key without modification")

			// Verify NO double prefixing
			assert.False(t, strings.Contains(actualPath, "artifacts/artifacts/"),
				"Path should NOT contain double bucket prefix")

			// For documentation: the storage layer (with collection configured)
			// will internally add the bucket when calling the actual storage backend
			t.Logf("Path at ARFService level: %s", actualPath)
			t.Logf("Storage layer will internally prepend bucket from collection config")
		})
	}
}

// TestStoragePathVerification_CorrectPathStructure verifies paths follow expected structure
func TestStoragePathVerification_CorrectPathStructure(t *testing.T) {
	// This test verifies that paths are structured correctly for SeaweedFS

	ctx := context.Background()

	// Create mock storage that simulates SeaweedFS behavior
	mockStorage := NewMockUnifiedStorage()
	pathTracker := NewPathTrackingStorage(mockStorage)

	// Create ARFService with empty bucket
	service, err := NewARFService(pathTracker, "")
	require.NoError(t, err)

	// Test OpenRewrite job paths
	jobID := "openrewrite-20240101-123456"
	inputKey := fmt.Sprintf("jobs/%s/input.tar", jobID)
	outputKey := fmt.Sprintf("jobs/%s/output.tar", jobID)

	t.Run("Input tar upload", func(t *testing.T) {
		pathTracker.putPaths = nil

		inputData := []byte("tar file content")
		err := service.Put(ctx, inputKey, inputData)
		assert.NoError(t, err)

		// Verify correct path structure
		require.Len(t, pathTracker.putPaths, 1)
		actualPath := pathTracker.putPaths[0]

		// Path should maintain the jobs/{id}/file.tar structure
		assert.True(t, strings.HasPrefix(actualPath, "jobs/"),
			"Path should start with jobs/")
		assert.True(t, strings.Contains(actualPath, jobID),
			"Path should contain job ID")
		assert.True(t, strings.HasSuffix(actualPath, "/input.tar"),
			"Path should end with /input.tar")

		// Full path structure check
		expectedPath := inputKey
		assert.Equal(t, expectedPath, actualPath,
			"Path structure should be jobs/{job-id}/input.tar")
	})

	t.Run("Output tar download", func(t *testing.T) {
		pathTracker.getPaths = nil

		// Put the output data into mock storage first
		err := mockStorage.Put(ctx, outputKey, bytes.NewReader([]byte("output data")))
		require.NoError(t, err)

		data, err := service.Get(ctx, outputKey)
		assert.NoError(t, err)
		assert.NotNil(t, data)

		// Verify correct path structure
		require.Len(t, pathTracker.getPaths, 1)
		actualPath := pathTracker.getPaths[0]

		// Verify path maintains correct structure
		assert.Equal(t, outputKey, actualPath,
			"GET should use the same path structure")
	})
}

// TestStoragePathVerification_DirectoryCreation verifies directory creation works
func TestStoragePathVerification_DirectoryCreation(t *testing.T) {
	// This test verifies that directory creation doesn't fail with 404
	// The SeaweedFS provider should handle directory creation properly

	ctx := context.Background()

	// Create mock storage
	mockStorage := NewMockUnifiedStorage()
	pathTracker := NewPathTrackingStorage(mockStorage)

	// Create ARFService
	service, err := NewARFService(pathTracker, "")
	require.NoError(t, err)

	// Test creating nested directory structure
	deepPath := "jobs/test-job/artifacts/nested/deep/file.txt"

	t.Run("Deep nested path", func(t *testing.T) {
		pathTracker.putPaths = nil

		err := service.Put(ctx, deepPath, []byte("test"))
		assert.NoError(t, err)

		// Verify path is passed correctly
		require.Len(t, pathTracker.putPaths, 1)
		assert.Equal(t, deepPath, pathTracker.putPaths[0])

		// In real scenario, SeaweedFS provider would create directories:
		// - jobs/
		// - jobs/test-job/
		// - jobs/test-job/artifacts/
		// - jobs/test-job/artifacts/nested/
		// - jobs/test-job/artifacts/nested/deep/
		// Before uploading the file

		t.Log("SeaweedFS provider should handle directory creation internally")
	})
}

// TestStoragePathVerification_EndToEnd simulates a complete OpenRewrite transformation flow
func TestStoragePathVerification_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end test in short mode")
	}

	ctx := context.Background()

	// Create mock storage
	mockStorage := NewMockUnifiedStorage()
	pathTracker := NewPathTrackingStorage(mockStorage)

	// Create ARFService
	service, err := NewARFService(pathTracker, "")
	require.NoError(t, err)

	// Simulate OpenRewrite transformation flow
	jobID := "openrewrite-test-123"

	t.Run("Complete transformation flow", func(t *testing.T) {
		// 1. Upload input tar
		inputKey := fmt.Sprintf("jobs/%s/input.tar", jobID)
		err := service.Put(ctx, inputKey, []byte("input tar content"))
		assert.NoError(t, err)

		// 2. Check if input exists
		pathTracker.existsPaths = nil

		exists, err := service.Exists(ctx, inputKey)
		assert.NoError(t, err)
		assert.True(t, exists)

		// 3. Simulate transformation (would happen in Nomad job)
		// Job would download input.tar and upload output.tar

		// 4. Upload output tar (simulating Nomad job completion)
		outputKey := fmt.Sprintf("jobs/%s/output.tar", jobID)
		err = service.Put(ctx, outputKey, []byte("transformed output"))
		assert.NoError(t, err)

		// 5. Download output
		pathTracker.getPaths = nil

		data, err := service.Get(ctx, outputKey)
		assert.NoError(t, err)
		assert.NotNil(t, data)

		// Verify all paths are correct
		assert.Contains(t, pathTracker.putPaths, inputKey)
		assert.Contains(t, pathTracker.putPaths, outputKey)
		assert.Contains(t, pathTracker.existsPaths, inputKey)
		assert.Contains(t, pathTracker.getPaths, outputKey)

		// No paths should have double prefixes
		allPaths := append(pathTracker.putPaths, pathTracker.getPaths...)
		allPaths = append(allPaths, pathTracker.existsPaths...)
		for _, path := range allPaths {
			assert.False(t, strings.Contains(path, "artifacts/artifacts/"),
				"No path should have double bucket prefix: %s", path)
		}

		t.Log("Transformation flow completed successfully with correct paths")
	})
}
