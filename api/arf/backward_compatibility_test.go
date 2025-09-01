package arf

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackwardCompatibility tests that old adapter functions still work
func TestBackwardCompatibility(t *testing.T) {
	mockStorage := NewMockUnifiedStorage()
	ctx := context.Background()
	
	// Test old NewStorageAdapter function
	t.Run("NewStorageAdapter", func(t *testing.T) {
		adapter := NewStorageAdapter(mockStorage)
		require.NotNil(t, adapter)
		
		// Test operations
		testData := []byte("test data")
		err := adapter.Put(ctx, "test-key", testData)
		assert.NoError(t, err)
		
		data, err := adapter.Get(ctx, "test-key")
		assert.NoError(t, err)
		assert.Equal(t, testData, data)
		
		exists, err := adapter.Exists(ctx, "test-key")
		assert.NoError(t, err)
		assert.True(t, exists)
		
		err = adapter.Delete(ctx, "test-key")
		assert.NoError(t, err)
		
		exists, err = adapter.Exists(ctx, "test-key")
		assert.NoError(t, err)
		assert.False(t, exists)
	})
	
	// Test old NewStorageAdapterWithBucket function  
	t.Run("NewStorageAdapterWithBucket", func(t *testing.T) {
		adapter := NewStorageAdapterWithBucket(mockStorage, "test-bucket")
		require.NotNil(t, adapter)
		
		// Test operations
		testData := []byte("test data with bucket")
		err := adapter.Put(ctx, "test-key", testData)
		assert.NoError(t, err)
		
		// Verify the data was stored with correct bucket prefix
		prefixedKey := "test-bucket/test-key"
		reader, err := mockStorage.Get(ctx, prefixedKey)
		assert.NoError(t, err)
		defer reader.Close()
		
		storedData := make([]byte, len(testData))
		_, err = reader.Read(storedData)
		assert.NoError(t, err)
		assert.Equal(t, testData, storedData)
	})
}

// TestAdapterImplementsInterface verifies backward compatibility at compile time
func TestAdapterImplementsInterface(t *testing.T) {
	mockStorage := NewMockUnifiedStorage()
	
	// These should compile and implement StorageService interface
	var service1 StorageService = NewStorageAdapter(mockStorage)
	var service2 StorageService = NewStorageAdapterWithBucket(mockStorage, "test")
	
	assert.NotNil(t, service1)
	assert.NotNil(t, service2)
}