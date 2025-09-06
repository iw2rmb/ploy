package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/kb/models"
)

func TestKBStorage_StoreRetrieveError(t *testing.T) {
	tests := []struct {
		name  string
		error models.Error
	}{
		{
			name: "java compilation error",
			error: models.Error{
				ID:        "java-compilation-missing-symbol",
				Signature: "java-compilation-missing-symbol",
				Message:   "cannot find symbol: class Optional",
				Location:  "Main.java:15",
				BuildLogs: []string{"[ERROR] Main.java:[15,24] cannot find symbol"},
				Created:   time.Now(),
			},
		},
		{
			name: "syntax error",
			error: models.Error{
				ID:        "java-syntax-semicolon",
				Signature: "java-syntax-semicolon",
				Message:   "';' expected",
				Location:  "App.java:23",
				BuildLogs: []string{"[ERROR] App.java:[23,15] ';' expected"},
				Created:   time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should fail - KBStorage doesn't exist yet
			config := &Config{
				StorageURL: "http://localhost:8888",
				Timeout:    10 * time.Second,
			}
			
			storage := NewKBStorage(config)
			require.NotNil(t, storage)

			ctx := context.Background()

			// Test Store operation
			err := storage.StoreError(ctx, &tt.error)
			assert.NoError(t, err, "Should store error without error")

			// Test Retrieve operation
			retrievedError, err := storage.RetrieveError(ctx, tt.error.Signature)
			assert.NoError(t, err, "Should retrieve error without error")
			assert.NotNil(t, retrievedError)
			
			// Verify data integrity
			assert.Equal(t, tt.error.ID, retrievedError.ID)
			assert.Equal(t, tt.error.Signature, retrievedError.Signature)
			assert.Equal(t, tt.error.Message, retrievedError.Message)
			assert.Equal(t, tt.error.Location, retrievedError.Location)
			assert.Equal(t, tt.error.BuildLogs, retrievedError.BuildLogs)
		})
	}
}

func TestKBStorage_StoreRetrieveCase(t *testing.T) {
	t.Run("store and retrieve case", func(t *testing.T) {
		// This should fail - KBStorage doesn't exist yet
		storage := NewKBStorage(&Config{StorageURL: "http://localhost:8888"})
		ctx := context.Background()

		testCase := &models.Case{
			ID:         "case-123",
			ErrorID:    "java-compilation-error",
			PatchHash:  "patch-hash-abc",
			Patch:      []byte("diff --git a/Main.java b/Main.java\n+import java.util.Optional;"),
			Success:    true,
			Confidence: 0.85,
			Created:    time.Now(),
		}

		// Store case
		err := storage.StoreCase(ctx, testCase)
		assert.NoError(t, err)

		// Retrieve case
		retrievedCase, err := storage.RetrieveCase(ctx, testCase.ID)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedCase)

		// Verify data
		assert.Equal(t, testCase.ID, retrievedCase.ID)
		assert.Equal(t, testCase.ErrorID, retrievedCase.ErrorID)
		assert.Equal(t, testCase.PatchHash, retrievedCase.PatchHash)
		assert.Equal(t, testCase.Patch, retrievedCase.Patch)
		assert.Equal(t, testCase.Success, retrievedCase.Success)
		assert.Equal(t, testCase.Confidence, retrievedCase.Confidence)
	})

	t.Run("list cases by error ID", func(t *testing.T) {
		// This should fail - ListCasesByError doesn't exist yet
		storage := NewKBStorage(&Config{StorageURL: "http://localhost:8888"})
		ctx := context.Background()

		errorID := "test-error-list"

		// Store multiple cases for same error
		cases := []*models.Case{
			{ID: "case-1", ErrorID: errorID, Success: true},
			{ID: "case-2", ErrorID: errorID, Success: false},
			{ID: "case-3", ErrorID: errorID, Success: true},
		}

		for _, c := range cases {
			err := storage.StoreCase(ctx, c)
			assert.NoError(t, err)
		}

		// List cases for this error
		retrievedCases, err := storage.ListCasesByError(ctx, errorID)
		assert.NoError(t, err)
		assert.Len(t, retrievedCases, 3)

		// Verify all cases are present
		caseIDs := make(map[string]bool)
		for _, c := range retrievedCases {
			caseIDs[c.ID] = true
			assert.Equal(t, errorID, c.ErrorID)
		}

		assert.True(t, caseIDs["case-1"])
		assert.True(t, caseIDs["case-2"])
		assert.True(t, caseIDs["case-3"])
	})
}

func TestKBStorage_StoreSummary(t *testing.T) {
	t.Run("store and retrieve summary", func(t *testing.T) {
		// This should fail - summary storage doesn't exist yet
		storage := NewKBStorage(&Config{StorageURL: "http://localhost:8888"})
		ctx := context.Background()

		summary := &models.Summary{
			ErrorID:     "java-error-summary",
			CaseCount:   10,
			SuccessRate: 0.7,
			TopPatches: []models.PatchSummary{
				{Hash: "patch-1", SuccessRate: 0.9, AttemptCount: 5},
				{Hash: "patch-2", SuccessRate: 0.5, AttemptCount: 2},
			},
			Updated: time.Now(),
		}

		// Store summary
		err := storage.StoreSummary(ctx, summary)
		assert.NoError(t, err)

		// Retrieve summary
		retrievedSummary, err := storage.RetrieveSummary(ctx, summary.ErrorID)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedSummary)

		// Verify data
		assert.Equal(t, summary.ErrorID, retrievedSummary.ErrorID)
		assert.Equal(t, summary.CaseCount, retrievedSummary.CaseCount)
		assert.Equal(t, summary.SuccessRate, retrievedSummary.SuccessRate)
		assert.Len(t, retrievedSummary.TopPatches, 2)
		assert.Equal(t, "patch-1", retrievedSummary.TopPatches[0].Hash)
		assert.Equal(t, 0.9, retrievedSummary.TopPatches[0].SuccessRate)
	})
}

func TestKBStorage_ErrorHandling(t *testing.T) {
	t.Run("retrieve non-existent error", func(t *testing.T) {
		// This should fail - error handling not implemented yet
		storage := NewKBStorage(&Config{StorageURL: "http://localhost:8888"})
		ctx := context.Background()

		_, err := storage.RetrieveError(ctx, "non-existent-error")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("retrieve non-existent case", func(t *testing.T) {
		storage := NewKBStorage(&Config{StorageURL: "http://localhost:8888"})
		ctx := context.Background()

		_, err := storage.RetrieveCase(ctx, "non-existent-case")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("storage timeout", func(t *testing.T) {
		// This should fail - timeout handling not implemented yet
		config := &Config{
			StorageURL: "http://invalid-url:9999", // Invalid URL to trigger timeout
			Timeout:    100 * time.Millisecond,    // Short timeout
		}
		storage := NewKBStorage(config)

		ctx := context.Background()
		err := &models.Error{
			ID:        "timeout-test",
			Signature: "timeout-test",
			Message:   "test error",
		}

		storeErr := storage.StoreError(ctx, err)
		assert.Error(t, storeErr)
		assert.Contains(t, storeErr.Error(), "timeout")
	})
}

func TestKBStorage_Concurrency(t *testing.T) {
	t.Run("concurrent operations", func(t *testing.T) {
		// This should fail - concurrency safety not implemented yet
		storage := NewKBStorage(&Config{StorageURL: "http://localhost:8888"})
		ctx := context.Background()

		errorID := "concurrent-test-error"
		numGoroutines := 10

		// Concurrent case storage
		done := make(chan bool, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				testCase := &models.Case{
					ID:      fmt.Sprintf("case-%d", id),
					ErrorID: errorID,
					Success: id%2 == 0, // Mix of success/failure
				}

				err := storage.StoreCase(ctx, testCase)
				assert.NoError(t, err)
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		// Verify all cases were stored
		cases, err := storage.ListCasesByError(ctx, errorID)
		assert.NoError(t, err)
		assert.Len(t, cases, numGoroutines)
	})
}