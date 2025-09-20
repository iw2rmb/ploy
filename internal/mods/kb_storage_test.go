package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/storage"
	seastorage "github.com/iw2rmb/ploy/internal/storage/providers/seaweedfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	seaweedMasterAddr     = "http://localhost:9333"
	seaweedFilerAddr      = "http://localhost:8888"
	seaweedTestCollection = "kb-tests"
)

type noopLockManager struct{}

func (noopLockManager) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	return &Lock{Key: key, TTL: ttl}, nil
}

func (noopLockManager) ReleaseLock(ctx context.Context, lock *Lock) error { return nil }

func (noopLockManager) IsLocked(ctx context.Context, key string) (bool, error) { return false, nil }

func (noopLockManager) TryWithLockRetry(ctx context.Context, key string, config *LockConfig, fn func() error) error {
	if fn != nil {
		return fn()
	}
	return nil
}

func requireSeaweedStorage(t *testing.T) storage.Storage {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping SeaweedFS-backed tests in short mode")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(seaweedFilerAddr + "/healthz")
	if err != nil {
		t.Skipf("SeaweedFS not available locally: %v", err)
	}
	_ = resp.Body.Close()

	cfg := seastorage.Config{
		Master:      strings.TrimPrefix(seaweedMasterAddr, "http://"),
		Filer:       strings.TrimPrefix(seaweedFilerAddr, "http://"),
		Collection:  seaweedTestCollection,
		Replication: "000",
		Timeout:     15,
	}

	provider, err := seastorage.New(cfg)
	require.NoError(t, err)

	return provider
}

func uniqueSignature(t *testing.T) string {
	return fmt.Sprintf("sig-%s-%d", sanitizeComponent(t.Name()), time.Now().UnixNano())
}

func uniqueRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
}

func uniqueFingerprint(t *testing.T) string {
	return fmt.Sprintf("fp-%s-%d", sanitizeComponent(t.Name()), time.Now().UnixNano())
}

func sanitizeComponent(name string) string {
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if b.Len() == 0 || b.String()[b.Len()-1] != '-' {
				b.WriteRune('-')
			}
		}
	}
	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "test"
	}
	return cleaned
}

func TestSeaweedFSKBStorage_WriteCase(t *testing.T) {
	storageClient := requireSeaweedStorage(t)
	kbStorage := NewSeaweedFSKBStorage(storageClient, noopLockManager{})

	ctx := context.Background()
	lang := "java"
	signature := uniqueSignature(t)
	runID := uniqueRunID()

	record := createTestCaseRecord()
	record.RunID = runID
	record.Signature = signature
	record.Language = lang

	key := kbStorage.buildCaseKey(lang, signature, runID)
	t.Cleanup(func() { _ = storageClient.Delete(ctx, key) })

	require.NoError(t, kbStorage.WriteCase(ctx, lang, signature, runID, record))

	reader, err := storageClient.Get(ctx, key)
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	require.NoError(t, err)

	var stored CaseRecord
	require.NoError(t, json.Unmarshal(data, &stored))
	assert.Equal(t, runID, stored.RunID)
	assert.Equal(t, signature, stored.Signature)
	assert.Equal(t, lang, stored.Language)
}

func TestSeaweedFSKBStorage_ReadCases(t *testing.T) {
	storageClient := requireSeaweedStorage(t)
	kbStorage := NewSeaweedFSKBStorage(storageClient, noopLockManager{})

	ctx := context.Background()
	lang := "java"
	signature := uniqueSignature(t)

	record1 := createTestCaseRecord()
	record1.RunID = uniqueRunID()
	record1.Signature = signature
	record1.Language = lang

	record2 := createTestCaseRecord()
	record2.RunID = uniqueRunID()
	record2.Signature = signature
	record2.Language = lang

	key1 := kbStorage.buildCaseKey(lang, signature, record1.RunID)
	key2 := kbStorage.buildCaseKey(lang, signature, record2.RunID)
	t.Cleanup(func() {
		_ = storageClient.Delete(ctx, key1)
		_ = storageClient.Delete(ctx, key2)
	})

	require.NoError(t, kbStorage.WriteCase(ctx, lang, signature, record1.RunID, record1))
	require.NoError(t, kbStorage.WriteCase(ctx, lang, signature, record2.RunID, record2))

	cases, err := kbStorage.ReadCases(ctx, lang, signature)
	require.NoError(t, err)
	assert.Len(t, cases, 2)

	runIDs := map[string]bool{}
	for _, c := range cases {
		runIDs[c.RunID] = true
	}
	assert.True(t, runIDs[record1.RunID])
	assert.True(t, runIDs[record2.RunID])
}

func TestSeaweedFSKBStorage_SummaryRoundTrip(t *testing.T) {
	storageClient := requireSeaweedStorage(t)
	kbStorage := NewSeaweedFSKBStorage(storageClient, noopLockManager{})

	ctx := context.Background()
	lang := "java"
	signature := uniqueSignature(t)
	summary := createTestSummaryRecord()
	summary.Language = lang
	summary.Signature = signature

	key := kbStorage.buildSummaryKey(lang, signature)
	t.Cleanup(func() { _ = storageClient.Delete(ctx, key) })

	require.NoError(t, kbStorage.WriteSummary(ctx, lang, signature, summary))

	loaded, err := kbStorage.ReadSummary(ctx, lang, signature)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, lang, loaded.Language)
	assert.Equal(t, signature, loaded.Signature)
	if assert.Len(t, loaded.Promoted, 1) {
		assert.Equal(t, summary.Promoted[0].Ref, loaded.Promoted[0].Ref)
	}
}

func TestSeaweedFSKBStorage_PatchRoundTrip(t *testing.T) {
	storageClient := requireSeaweedStorage(t)
	kbStorage := NewSeaweedFSKBStorage(storageClient, noopLockManager{})

	ctx := context.Background()
	fingerprint := uniqueFingerprint(t)
	patchContent := []byte("--- a/Test.java\n+++ b/Test.java\n@@ -1,3 +1,3 @@\n-import java.util.*;\n+\n class Test {\n }")

	key := kbStorage.buildPatchKey(fingerprint)
	t.Cleanup(func() { _ = storageClient.Delete(ctx, key) })

	require.NoError(t, kbStorage.StorePatch(ctx, fingerprint, patchContent))

	loaded, err := kbStorage.GetPatch(ctx, fingerprint)
	require.NoError(t, err)
	assert.Equal(t, patchContent, loaded)
}

func TestSeaweedFSKBStorage_SnapshotRoundTrip(t *testing.T) {
	storageClient := requireSeaweedStorage(t)
	kbStorage := NewSeaweedFSKBStorage(storageClient, noopLockManager{})

	ctx := context.Background()
	snapshot := &SnapshotManifest{
		Timestamp: time.Now().UTC().Round(time.Second),
		Languages: map[string]int{
			"java":       10,
			"typescript": 3,
		},
		TotalCases: 13,
		TotalSigs:  5,
		Version:    "v1.0.0",
	}

	key := "kb/healing/snapshot.json"
	t.Cleanup(func() { _ = storageClient.Delete(ctx, key) })

	require.NoError(t, kbStorage.WriteSnapshot(ctx, snapshot))

	loaded, err := kbStorage.ReadSnapshot(ctx)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, snapshot.TotalCases, loaded.TotalCases)
	assert.Equal(t, snapshot.TotalSigs, loaded.TotalSigs)
	assert.Equal(t, snapshot.Version, loaded.Version)
}

func TestSeaweedFSKBStorage_Health(t *testing.T) {
	storageClient := requireSeaweedStorage(t)
	kbStorage := NewSeaweedFSKBStorage(storageClient, noopLockManager{})

	ctx := context.Background()
	assert.NoError(t, kbStorage.Health(ctx))
}

func TestSeaweedFSKBStorage_KeyBuilding(t *testing.T) {
	kbStorage := NewSeaweedFSKBStorage(nil, noopLockManager{})

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

// Test data builders used across scenarios

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
			Duration:    5000,
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
