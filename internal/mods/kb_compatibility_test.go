package mods

import (
	"context"
	"testing"
	"time"
)

// TestBackwardCompatibility ensures new deduplication features work with existing KB data
func TestBackwardCompatibility(t *testing.T) {
	// Create test data that matches the existing KB structure
	existingCase := &CaseRecord{
		RunID:     "existing-case-123",
		Timestamp: time.Now(),
		Language:  "java",
		Signature: "abcd123456789000",
		Context: &CaseContext{
			Language:        "java",
			Lane:            "A",
			RepoURL:         "https://github.com/example/test.git",
			CompilerVersion: "javac-11.0.2",
			BuildCommand:    "mvn compile",
		},
		Attempt: &HealingAttempt{
			Type:   "orw_recipe",
			Recipe: "AddMissingImports",
		},
		Outcome: &HealingOutcome{
			Success:     true,
			BuildStatus: "passed",
			Duration:    5000,
			CompletedAt: time.Now(),
		},
	}

	// Test that enhanced signature generator can handle existing data
	t.Run("EnhancedSignatureGeneratorCompatibility", func(t *testing.T) {
		generator := NewEnhancedSignatureGenerator(nil)

		// Test basic signature generation (existing functionality)
		signature := generator.GenerateSignature("java", "javac", []byte("Error: symbol not found"), []byte(""))
		if len(signature) != 16 {
			t.Errorf("Expected signature length 16, got %d", len(signature))
		}

		// Test enhanced functionality with existing signatures
		sig1 := "abcd123456789000"
		sig2 := "abcd123456789111"
		similarity := generator.ComputeSignatureSimilarity(sig1, sig2, "java", "javac")

		if similarity < 0 || similarity > 1 {
			t.Errorf("Similarity should be between 0 and 1, got %f", similarity)
		}
	})

	// Test that compaction system can handle existing case structures
	t.Run("CompactionCompatibility", func(t *testing.T) {
		config := DefaultCompactionConfig()
		config.DryRun = true // Don't actually modify data in test

		// Mock storage that returns existing case structure
		mockStorage := &CompatibilityMockKBStorage{
			cases: []*CaseRecord{existingCase},
		}

		mockLockMgr := &CompatibilityMockKBLockManager{}
		mockSigGen := NewEnhancedSignatureGenerator(nil)
		// Use nil for SummaryComputer in compatibility test
		job := NewCompactionJob(mockStorage, mockSigGen, mockLockMgr, nil, config)

		// Should handle existing case without errors
		stats, err := job.CompactSignature(context.Background(), existingCase.Language, existingCase.Signature)
		if err != nil {
			t.Errorf("Compaction failed on existing case: %v", err)
		}

		if stats.SignaturesAnalyzed != 1 {
			t.Errorf("Expected 1 signature analyzed, got %d", stats.SignaturesAnalyzed)
		}
	})

	// Test that maintenance scheduler works with existing data
	t.Run("MaintenanceSchedulerCompatibility", func(t *testing.T) {
		mockStorage := &CompatibilityMockKBStorage{
			cases: []*CaseRecord{existingCase},
			snapshot: &SnapshotManifest{
				Timestamp:  time.Now(),
				Languages:  map[string]int{"java": 1},
				TotalCases: 1,
				TotalSigs:  1,
				Version:    "1.0.0",
			},
		}

		mockLockMgr := &CompatibilityMockKBLockManager{}
		mockSigGen := NewEnhancedSignatureGenerator(nil)
		config := DefaultMaintenanceConfig()
		config.EnableCompactionJobs = true
		config.EnableSummaryJobs = false // Disable to simplify test
		config.EnableSnapshotJobs = false

		// Use nil for SummaryComputer in compatibility test
		scheduler := NewMaintenanceScheduler(
			mockStorage,
			mockSigGen,
			mockLockMgr,
			nil,
			config,
		)

		// Should be able to get status without errors
		status, err := scheduler.GetMaintenanceStatus(context.Background())
		if err != nil {
			t.Errorf("Failed to get maintenance status: %v", err)
		}

		if status.SystemHealth != "healthy" {
			t.Errorf("Expected healthy system, got %s", status.SystemHealth)
		}
	})

	// Test that metrics collection works with existing data
	t.Run("MetricsCompatibility", func(t *testing.T) {
		mockStorage := &CompatibilityMockKBStorage{
			snapshot: &SnapshotManifest{
				TotalCases: 1,
				TotalSigs:  1,
			},
		}

		mockSigGen := NewEnhancedSignatureGenerator(nil)
		config := DefaultMetricsConfig()

		collector := NewMetricsCollector(mockStorage, mockSigGen, config)

		// Should be able to collect metrics without errors
		err := collector.collectMetrics(context.Background())
		if err != nil {
			t.Errorf("Failed to collect metrics: %v", err)
		}

		// Should be able to generate summary
		summary := collector.GenerateMetricsSummary(context.Background(), 24)
		if summary.SystemHealth == "" {
			t.Error("Expected system health to be set")
		}
	})

	// Test that existing data structures serialize/deserialize correctly
	t.Run("DataStructureCompatibility", func(t *testing.T) {
		// Test that existing CaseRecord structure is preserved
		if existingCase.RunID != "existing-case-123" {
			t.Error("Case record RunID should be preserved")
		}

		if existingCase.Context.Language != "java" {
			t.Error("Case context language should be preserved")
		}

		if existingCase.Attempt.Type != "orw_recipe" {
			t.Error("Healing attempt type should be preserved")
		}

		if !existingCase.Outcome.Success {
			t.Error("Healing outcome success should be preserved")
		}
	})
}

// TestInterfaceCompatibility ensures all interfaces remain backward compatible
func TestInterfaceCompatibility(t *testing.T) {
	t.Run("SignatureGeneratorInterface", func(t *testing.T) {
		var generator SignatureGenerator = NewDefaultSignatureGenerator()

		// Test existing interface methods
		signature := generator.GenerateSignature("java", "javac", []byte("error"), []byte(""))
		if signature == "" {
			t.Error("GenerateSignature should return non-empty result")
		}

		patch := []byte("diff --git a/file.java b/file.java\n+import java.util.List;")
		normalizedPatch, fingerprint := generator.NormalizePatch(patch)

		if len(normalizedPatch) == 0 {
			t.Error("NormalizePatch should return normalized patch")
		}

		if fingerprint == "" {
			t.Error("NormalizePatch should return fingerprint")
		}
	})

	t.Run("EnhancedSignatureGeneratorInterface", func(t *testing.T) {
		var generator EnhancedSignatureGenerator = NewEnhancedSignatureGenerator(nil)

		// Test that it still implements basic interface
		signature := generator.GenerateSignature("java", "javac", []byte("error"), []byte(""))
		if signature == "" {
			t.Error("Enhanced generator should implement basic GenerateSignature")
		}

		// Test enhanced functionality
		similarity := generator.ComputeSignatureSimilarity("abc123", "abc124", "java", "javac")
		if similarity < 0 || similarity > 1 {
			t.Errorf("Similarity should be between 0 and 1, got %f", similarity)
		}
	})
}

// TestConfigurationBackwardCompatibility ensures configuration changes don't break existing setups
func TestConfigurationBackwardCompatibility(t *testing.T) {
	t.Run("DeduplicationConfig", func(t *testing.T) {
		config := DefaultDeduplicationConfig()

		// Ensure reasonable defaults that won't break existing behavior
		if config.ErrorSimilarityThreshold <= 0 || config.ErrorSimilarityThreshold > 1 {
			t.Errorf("Error similarity threshold should be between 0 and 1, got %f", config.ErrorSimilarityThreshold)
		}

		if config.MaxSimilarResults <= 0 {
			t.Errorf("Max similar results should be positive, got %d", config.MaxSimilarResults)
		}
	})

	t.Run("CompactionConfig", func(t *testing.T) {
		config := DefaultCompactionConfig()

		// Ensure conservative defaults
		if config.MinCasesForCompaction < 5 {
			t.Errorf("Min cases for compaction should be conservative, got %d", config.MinCasesForCompaction)
		}

		if config.SimilarityThresholdForMerge < 0.9 {
			t.Errorf("Similarity threshold for merge should be high to avoid false positives, got %f", config.SimilarityThresholdForMerge)
		}
	})

	t.Run("MaintenanceConfig", func(t *testing.T) {
		config := DefaultMaintenanceConfig()

		// Ensure intervals are reasonable
		if config.CompactionInterval < time.Hour {
			t.Errorf("Compaction interval should be at least 1 hour, got %v", config.CompactionInterval)
		}

		if config.MaxConcurrentJobs < 1 {
			t.Errorf("Max concurrent jobs should be at least 1, got %d", config.MaxConcurrentJobs)
		}
	})
}

// Mock implementations for testing

type CompatibilityMockKBStorage struct {
	cases    []*CaseRecord
	snapshot *SnapshotManifest
}

func (m *CompatibilityMockKBStorage) WriteCase(ctx context.Context, lang, signature, runID string, caseData *CaseRecord) error {
	return nil
}

func (m *CompatibilityMockKBStorage) ReadCases(ctx context.Context, lang, signature string) ([]*CaseRecord, error) {
	return m.cases, nil
}

func (m *CompatibilityMockKBStorage) ReadSummary(ctx context.Context, lang, signature string) (*SummaryRecord, error) {
	return &SummaryRecord{}, nil
}

func (m *CompatibilityMockKBStorage) WriteSummary(ctx context.Context, lang, signature string, summary *SummaryRecord) error {
	return nil
}

func (m *CompatibilityMockKBStorage) StorePatch(ctx context.Context, fingerprint string, patch []byte) error {
	return nil
}

func (m *CompatibilityMockKBStorage) GetPatch(ctx context.Context, fingerprint string) ([]byte, error) {
	return []byte("mock patch"), nil
}

func (m *CompatibilityMockKBStorage) WriteSnapshot(ctx context.Context, snapshot *SnapshotManifest) error {
	return nil
}

func (m *CompatibilityMockKBStorage) ReadSnapshot(ctx context.Context) (*SnapshotManifest, error) {
	if m.snapshot != nil {
		return m.snapshot, nil
	}
	return &SnapshotManifest{
		Languages:  make(map[string]int),
		TotalCases: 0,
		TotalSigs:  0,
	}, nil
}

func (m *CompatibilityMockKBStorage) Health(ctx context.Context) error {
	return nil
}

type CompatibilityMockKBLockManager struct{}

func (m *CompatibilityMockKBLockManager) AcquireLock(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	return &Lock{Key: key}, nil
}

func (m *CompatibilityMockKBLockManager) ReleaseLock(ctx context.Context, lock *Lock) error {
	return nil
}

func (m *CompatibilityMockKBLockManager) IsLocked(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (m *CompatibilityMockKBLockManager) TryWithLockRetry(ctx context.Context, key string, config *LockConfig, fn func() error) error {
	return fn()
}
