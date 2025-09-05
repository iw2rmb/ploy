package transflow

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock implementations for testing

type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	args := m.Called(ctx, key, reader, opts)
	return args.Error(0)
}

func (m *MockStorage) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func (m *MockStorage) List(ctx context.Context, opts storage.ListOptions) ([]storage.Object, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]storage.Object), args.Error(1)
}

func (m *MockStorage) DeleteBatch(ctx context.Context, keys []string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

func (m *MockStorage) Head(ctx context.Context, key string) (*storage.Object, error) {
	args := m.Called(ctx, key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Object), args.Error(1)
}

func (m *MockStorage) UpdateMetadata(ctx context.Context, key string, metadata map[string]string) error {
	args := m.Called(ctx, key, metadata)
	return args.Error(0)
}

func (m *MockStorage) Copy(ctx context.Context, src, dst string) error {
	args := m.Called(ctx, src, dst)
	return args.Error(0)
}

func (m *MockStorage) Move(ctx context.Context, src, dst string) error {
	args := m.Called(ctx, src, dst)
	return args.Error(0)
}

func (m *MockStorage) Health(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockStorage) Metrics() *storage.StorageMetrics {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*storage.StorageMetrics)
}

// MockLockManager is defined in kb_summary_test.go to avoid duplicate declarations

// Test data

func createTestCaseRecord() *CaseRecord {
	return &CaseRecord{
		RunID:     "test-run-123",
		Timestamp: time.Now(),
		Language:  "java",
		Signature: "abc123def456",
		Context: &CaseContext{
			Language:        "java",
			Lane:            "A",
			RepoURL:         "https://github.com/test/repo.git",
			CompilerVersion: "javac 11.0.1",
		},
		Attempt: &HealingAttempt{
			Type:   "orw_recipe",
			Recipe: "org.openrewrite.java.RemoveUnusedImports",
		},
		Outcome: &HealingOutcome{
			Success:     true,
			BuildStatus: "passed",
			Duration:    5000, // 5 seconds in ms
			CompletedAt: time.Now(),
		},
		BuildLogs: &SanitizedLogs{
			Stdout:    "Build successful",
			Stderr:    "",
			Truncated: false,
		},
	}
}

func createTestSummaryRecord() *SummaryRecord {
	return &SummaryRecord{
		Language:  "java",
		Signature: "abc123def456",
		Promoted: []PromotedFix{
			{
				Kind:          "orw_recipe",
				Ref:           "org.openrewrite.java.RemoveUnusedImports",
				Score:         0.85,
				Wins:          5,
				Failures:      1,
				LastSuccessAt: time.Now(),
				FirstSeenAt:   time.Now().Add(-24 * time.Hour),
			},
		},
		Stats: &SummaryStats{
			TotalCases:   6,
			SuccessCount: 5,
			FailureCount: 1,
			SuccessRate:  0.833,
			LastUpdated:  time.Now(),
			AvgDuration:  4500,
		},
	}
}

// Unit Tests

func TestSeaweedFSKBStorage_WriteCase(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	ctx := context.Background()
	caseRecord := createTestCaseRecord()

	// Mock storage Put call
	mockStorage.On("Put", ctx, "kb/healing/errors/java/abc123def456/cases/test-run-123.json", mock.AnythingOfType("*bytes.Reader"), mock.AnythingOfType("storage.PutOption")).Return(nil)

	err := kbStorage.WriteCase(ctx, "java", "abc123def456", "test-run-123", caseRecord)

	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
}

func TestSeaweedFSKBStorage_ReadCases(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	ctx := context.Background()

	// Create test data
	_ = createTestCaseRecord() // Create but don't use since we're using inline JSON
	caseJSON := `{
		"run_id": "test-run-123",
		"timestamp": "2023-12-01T10:00:00Z",
		"language": "java",
		"signature": "abc123def456",
		"context": {
			"language": "java",
			"lane": "A"
		},
		"attempt": {
			"type": "orw_recipe",
			"recipe": "org.openrewrite.java.RemoveUnusedImports"
		},
		"outcome": {
			"success": true,
			"build_status": "passed",
			"duration_ms": 5000,
			"completed_at": "2023-12-01T10:00:05Z"
		}
	}`

	// Mock objects returned by List
	objects := []storage.Object{
		{
			Key: "kb/healing/errors/java/abc123def456/cases/test-run-123.json",
		},
	}

	// Mock List call
	mockStorage.On("List", ctx, storage.ListOptions{
		Prefix:  "kb/healing/errors/java/abc123def456/cases/",
		MaxKeys: 1000,
	}).Return(objects, nil)

	// Mock Get call
	reader := io.NopCloser(bytes.NewReader([]byte(caseJSON)))
	mockStorage.On("Get", ctx, "kb/healing/errors/java/abc123def456/cases/test-run-123.json").Return(reader, nil)

	cases, err := kbStorage.ReadCases(ctx, "java", "abc123def456")

	assert.NoError(t, err)
	assert.Len(t, cases, 1)
	assert.Equal(t, "test-run-123", cases[0].RunID)
	assert.Equal(t, "java", cases[0].Language)
	assert.Equal(t, "orw_recipe", cases[0].Attempt.Type)
	mockStorage.AssertExpectations(t)
}

func TestSeaweedFSKBStorage_WriteSummary(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	ctx := context.Background()
	summary := createTestSummaryRecord()

	// Mock storage Put call
	mockStorage.On("Put", ctx, "kb/healing/errors/java/abc123def456/summary.json", mock.AnythingOfType("*bytes.Reader"), mock.AnythingOfType("storage.PutOption")).Return(nil)

	err := kbStorage.WriteSummary(ctx, "java", "abc123def456", summary)

	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
}

func TestSeaweedFSKBStorage_ReadSummary(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	ctx := context.Background()

	summaryJSON := `{
		"language": "java",
		"signature": "abc123def456",
		"promoted": [
			{
				"kind": "orw_recipe",
				"ref": "org.openrewrite.java.RemoveUnusedImports",
				"score": 0.85,
				"wins": 5,
				"failures": 1
			}
		],
		"stats": {
			"total_cases": 6,
			"success_count": 5,
			"failure_count": 1,
			"success_rate": 0.833
		}
	}`

	reader := io.NopCloser(bytes.NewReader([]byte(summaryJSON)))
	mockStorage.On("Get", ctx, "kb/healing/errors/java/abc123def456/summary.json").Return(reader, nil)

	summary, err := kbStorage.ReadSummary(ctx, "java", "abc123def456")

	assert.NoError(t, err)
	assert.Equal(t, "java", summary.Language)
	assert.Equal(t, "abc123def456", summary.Signature)
	assert.Len(t, summary.Promoted, 1)
	assert.Equal(t, "orw_recipe", summary.Promoted[0].Kind)
	assert.Equal(t, 6, summary.Stats.TotalCases)
	mockStorage.AssertExpectations(t)
}

func TestSeaweedFSKBStorage_StorePatch(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	ctx := context.Background()
	patchContent := []byte("--- a/Test.java\n+++ b/Test.java\n@@ -1,3 +1,3 @@\n-import java.util.*;\n+\n class Test {\n }")
	fingerprint := "abcd1234567890ef"

	// Mock storage Put call
	mockStorage.On("Put", ctx, "kb/healing/patches/abcd1234567890ef.patch", mock.AnythingOfType("*bytes.Reader"), mock.AnythingOfType("storage.PutOption")).Return(nil)

	err := kbStorage.StorePatch(ctx, fingerprint, patchContent)

	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
}

func TestSeaweedFSKBStorage_GetPatch(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	ctx := context.Background()
	fingerprint := "abcd1234567890ef"
	patchContent := []byte("--- a/Test.java\n+++ b/Test.java\n@@ -1,3 +1,3 @@\n-import java.util.*;\n+\n class Test {\n }")

	reader := io.NopCloser(bytes.NewReader(patchContent))
	mockStorage.On("Get", ctx, "kb/healing/patches/abcd1234567890ef.patch").Return(reader, nil)

	result, err := kbStorage.GetPatch(ctx, fingerprint)

	assert.NoError(t, err)
	assert.Equal(t, patchContent, result)
	mockStorage.AssertExpectations(t)
}

func TestSeaweedFSKBStorage_WriteSnapshot(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	ctx := context.Background()
	snapshot := &SnapshotManifest{
		Timestamp: time.Now(),
		Languages: map[string]int{
			"java":       100,
			"typescript": 50,
		},
		TotalCases: 150,
		TotalSigs:  75,
		Version:    "v1.0",
	}

	// Mock storage Put call
	mockStorage.On("Put", ctx, "kb/healing/snapshot.json", mock.AnythingOfType("*bytes.Reader"), mock.AnythingOfType("storage.PutOption")).Return(nil)

	err := kbStorage.WriteSnapshot(ctx, snapshot)

	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
}

func TestSeaweedFSKBStorage_Health(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	ctx := context.Background()

	// Mock health check
	mockStorage.On("Health", ctx).Return(nil)

	err := kbStorage.Health(ctx)

	assert.NoError(t, err)
	mockStorage.AssertExpectations(t)
}

// Integration test style - testing key building logic
func TestSeaweedFSKBStorage_KeyBuilding(t *testing.T) {
	mockStorage := new(MockStorage)
	mockLockMgr := new(MockLockManager)
	kbStorage := NewSeaweedFSKBStorage(mockStorage, mockLockMgr)

	tests := []struct {
		name     string
		method   string
		input    []string
		expected string
	}{
		{
			name:     "case key",
			method:   "buildCaseKey",
			input:    []string{"java", "abc123def456", "run-001"},
			expected: "kb/healing/errors/java/abc123def456/cases/run-001.json",
		},
		{
			name:     "cases prefix",
			method:   "buildCasesPrefix",
			input:    []string{"java", "abc123def456"},
			expected: "kb/healing/errors/java/abc123def456/cases/",
		},
		{
			name:     "summary key",
			method:   "buildSummaryKey",
			input:    []string{"java", "abc123def456"},
			expected: "kb/healing/errors/java/abc123def456/summary.json",
		},
		{
			name:     "patch key",
			method:   "buildPatchKey",
			input:    []string{"abcd1234567890ef"},
			expected: "kb/healing/patches/abcd1234567890ef.patch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result string
			switch test.method {
			case "buildCaseKey":
				result = kbStorage.buildCaseKey(test.input[0], test.input[1], test.input[2])
			case "buildCasesPrefix":
				result = kbStorage.buildCasesPrefix(test.input[0], test.input[1])
			case "buildSummaryKey":
				result = kbStorage.buildSummaryKey(test.input[0], test.input[1])
			case "buildPatchKey":
				result = kbStorage.buildPatchKey(test.input[0])
			}
			assert.Equal(t, test.expected, result)
		})
	}
}
