//go:build performance
// +build performance

package performance

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/transflow"
	"github.com/iw2rmb/ploy/internal/storage"
)

// setupKBBenchmarkEnvironment creates a KB test environment for benchmarking
func setupKBBenchmarkEnvironment(b *testing.B) *KBBenchmarkEnvironment {
	b.Helper()

	// Create storage client for testing
	storageClient, err := storage.NewStorageClient(&storage.Config{
		Endpoint: "http://localhost:8888", // SeaweedFS test endpoint
	})
	if err != nil {
		b.Skip("SeaweedFS not available for KB benchmarks")
	}

	// Create KB storage with mock lock manager for now
	lockManager := &transflow.MockKBLockManager{}
	kbStorage := transflow.NewSeaweedFSKBStorage(storageClient, lockManager)

	// Create KB integration
	kbConfig := transflow.DefaultKBConfig()
	kbIntegration := &transflow.KBIntegration{}

	env := &KBBenchmarkEnvironment{
		StorageClient: storageClient,
		KBStorage:     kbStorage,
		LockManager:   lockManager,
		KBIntegration: kbIntegration,
		Config:        kbConfig,
	}

	return env
}

type KBBenchmarkEnvironment struct {
	StorageClient *storage.StorageClient
	KBStorage     transflow.KBStorage
	LockManager   transflow.KBLockManager
	KBIntegration *transflow.KBIntegration
	Config        *transflow.KBConfig
}

func (e *KBBenchmarkEnvironment) Cleanup() {
	// Cleanup test data if needed
}

// generateTestHealingCase creates a realistic healing case for benchmarking
func generateTestHealingCase(runID string, errorSig string, success bool) *transflow.CaseRecord {
	return &transflow.CaseRecord{
		RunID:     runID,
		Timestamp: time.Now(),
		Language:  "java",
		Signature: errorSig,
		Context: &transflow.CaseContext{
			Language:        "java",
			Lane:            "C",
			RepoURL:         "https://github.com/test/repo.git",
			CompilerVersion: "javac 11.0.1",
			BuildCommand:    "mvn compile",
		},
		Attempt: &transflow.HealingAttempt{
			Type:             "orw_recipe",
			Recipe:           "org.openrewrite.java.migrate.Java11toJava17",
			PatchFingerprint: fmt.Sprintf("patch-%d", rand.Int31()),
			PatchContent:     generateTestPatch(1024), // 1KB patch
		},
		Outcome: &transflow.HealingOutcome{
			Success: success,
			BuildStatus: func() string {
				if success {
					return "passed"
				} else {
					return "failed"
				}
			}(),
			ErrorChanged: true,
			Duration:     int64(rand.Intn(60000)), // Random duration up to 60s
			CompletedAt:  time.Now(),
		},
		BuildLogs: &transflow.SanitizedLogs{
			Stdout:    "Build output...",
			Stderr:    "Build errors...",
			Truncated: false,
		},
	}
}

// generateTestPatch creates a patch of specified size
func generateTestPatch(sizeBytes int) string {
	patch := "--- a/src/main/java/Example.java\n"
	patch += "+++ b/src/main/java/Example.java\n"
	patch += "@@ -1,5 +1,5 @@\n"

	// Fill to desired size
	for len(patch) < sizeBytes {
		patch += fmt.Sprintf(" public void method%d() { /* code */ }\n", rand.Int())
	}

	return patch
}

// BenchmarkKBWriteCase benchmarks writing healing cases to KB storage
// This should initially fail as KB operations are not optimized
func BenchmarkKBWriteCase(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping KB benchmarks in short mode")
	}

	env := setupKBBenchmarkEnvironment(b)
	defer env.Cleanup()

	ctx := context.Background()
	language := "java"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		errorSig := fmt.Sprintf("benchmark-error-%d", i%10) // 10 distinct error types
		runID := fmt.Sprintf("benchmark-run-%d", i)

		caseRecord := generateTestHealingCase(runID, errorSig, i%3 == 0) // 33% success rate

		// Write case - expected to fail initially due to missing optimizations
		err := env.KBStorage.WriteCase(ctx, language, errorSig, runID, caseRecord)
		if err != nil {
			b.Logf("KB write case iteration %d failed (expected): %v", i, err)
			// Don't fail the benchmark, continue
		}
	}
}

// BenchmarkKBReadCases benchmarks reading cases from KB storage
func BenchmarkKBReadCases(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping KB benchmarks in short mode")
	}

	env := setupKBBenchmarkEnvironment(b)
	defer env.Cleanup()

	ctx := context.Background()
	language := "java"

	// Pre-populate with test data
	errorSignatures := make([]string, 10)
	for i := 0; i < 10; i++ {
		errorSig := fmt.Sprintf("benchmark-read-error-%d", i)
		errorSignatures[i] = errorSig

		// Create multiple cases per error signature
		for j := 0; j < 5; j++ {
			runID := fmt.Sprintf("setup-run-%d-%d", i, j)
			caseRecord := generateTestHealingCase(runID, errorSig, j%2 == 0)

			// Best effort setup - ignore failures
			env.KBStorage.WriteCase(ctx, language, errorSig, runID, caseRecord)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		errorSig := errorSignatures[i%len(errorSignatures)]

		// Read cases - expected to fail initially
		cases, err := env.KBStorage.ReadCases(ctx, language, errorSig)
		if err != nil {
			b.Logf("KB read cases iteration %d failed (expected): %v", i, err)
		} else if len(cases) == 0 {
			b.Logf("KB read cases iteration %d returned no cases", i)
		}
	}
}

// BenchmarkKBSummaryOperations benchmarks summary read/write operations
func BenchmarkKBSummaryOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping KB benchmarks in short mode")
	}

	env := setupKBBenchmarkEnvironment(b)
	defer env.Cleanup()

	ctx := context.Background()
	language := "java"

	b.Run("WriteSummary", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			errorSig := fmt.Sprintf("benchmark-summary-%d", i)

			summaryRecord := &transflow.SummaryRecord{
				Language:  language,
				Signature: errorSig,
				Promoted: []transflow.PromotedFix{
					{
						Kind:          "orw_recipe",
						Ref:           "org.openrewrite.java.migrate.Java11toJava17",
						Score:         0.75,
						Wins:          15,
						Failures:      5,
						LastSuccessAt: time.Now(),
						FirstSeenAt:   time.Now().Add(-24 * time.Hour),
					},
				},
				Stats: &transflow.SummaryStats{
					TotalCases:   20,
					SuccessCount: 15,
					FailureCount: 5,
					SuccessRate:  0.75,
					LastUpdated:  time.Now(),
					AvgDuration:  30000, // 30s average
				},
			}

			// Write summary - expected to fail initially
			err := env.KBStorage.WriteSummary(ctx, language, errorSig, summaryRecord)
			if err != nil {
				b.Logf("KB write summary iteration %d failed (expected): %v", i, err)
			}
		}
	})

	b.Run("ReadSummary", func(b *testing.B) {
		// Pre-populate some summaries
		for i := 0; i < 10; i++ {
			errorSig := fmt.Sprintf("benchmark-read-summary-%d", i)
			summaryRecord := &transflow.SummaryRecord{
				Language:  language,
				Signature: errorSig,
				Promoted:  []transflow.PromotedFix{},
				Stats: &transflow.SummaryStats{
					TotalCases:   10,
					SuccessCount: 7,
					FailureCount: 3,
					SuccessRate:  0.7,
					LastUpdated:  time.Now(),
				},
			}
			env.KBStorage.WriteSummary(ctx, language, errorSig, summaryRecord)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			errorSig := fmt.Sprintf("benchmark-read-summary-%d", i%10)

			// Read summary - expected to fail initially
			summary, err := env.KBStorage.ReadSummary(ctx, language, errorSig)
			if err != nil {
				b.Logf("KB read summary iteration %d failed (expected): %v", i, err)
			} else if summary == nil {
				b.Logf("KB read summary iteration %d returned nil", i)
			}
		}
	})
}

// BenchmarkKBPatchOperations benchmarks patch storage and retrieval
func BenchmarkKBPatchOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping KB benchmarks in short mode")
	}

	env := setupKBBenchmarkEnvironment(b)
	defer env.Cleanup()

	ctx := context.Background()
	testPatch := []byte(generateTestPatch(4096)) // 4KB patch

	b.Run("StorePatch", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fingerprint := fmt.Sprintf("patch-fingerprint-%d", i)

			// Store patch - expected to fail initially
			err := env.KBStorage.StorePatch(ctx, fingerprint, testPatch)
			if err != nil {
				b.Logf("KB store patch iteration %d failed (expected): %v", i, err)
			}
		}
	})

	b.Run("GetPatch", func(b *testing.B) {
		// Pre-populate some patches
		fingerprints := make([]string, 10)
		for i := 0; i < 10; i++ {
			fingerprint := fmt.Sprintf("benchmark-get-patch-%d", i)
			fingerprints[i] = fingerprint
			env.KBStorage.StorePatch(ctx, fingerprint, testPatch)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			fingerprint := fingerprints[i%len(fingerprints)]

			// Get patch - expected to fail initially
			patchData, err := env.KBStorage.GetPatch(ctx, fingerprint)
			if err != nil {
				b.Logf("KB get patch iteration %d failed (expected): %v", i, err)
			} else if len(patchData) == 0 {
				b.Logf("KB get patch iteration %d returned empty patch", i)
			}
		}
	})
}

// BenchmarkKBLearningWorkflow benchmarks the complete learning workflow
func BenchmarkKBLearningWorkflow(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping KB benchmarks in short mode")
	}

	env := setupKBBenchmarkEnvironment(b)
	defer env.Cleanup()

	ctx := context.Background()
	language := "java"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		errorSig := fmt.Sprintf("learning-workflow-error-%d", i%5) // 5 distinct errors
		runID := fmt.Sprintf("learning-run-%d", i)

		// Step 1: Load KB context (should fail initially)
		kbContext, err := env.KBIntegration.LoadKBContext(ctx, language, []byte("stdout"), []byte("stderr"))
		if err != nil {
			b.Logf("KB load context iteration %d failed (expected): %v", i, err)
			continue
		}

		// Step 2: Create healing attempt
		attempt := &transflow.HealingAttempt{
			Type:   "orw_recipe",
			Recipe: "org.openrewrite.java.migrate.Java11toJava17",
		}

		outcome := &transflow.HealingOutcome{
			Success: i%3 == 0, // 33% success rate
			BuildStatus: func() string {
				if i%3 == 0 {
					return "passed"
				} else {
					return "failed"
				}
			}(),
			Duration:    int64(rand.Intn(60000)),
			CompletedAt: time.Now(),
		}

		// Step 3: Write healing case (should fail initially)
		err = env.KBIntegration.WriteHealingCase(ctx, kbContext, attempt, outcome, "stdout", "stderr")
		if err != nil {
			b.Logf("KB write healing case iteration %d failed (expected): %v", i, err)
		}
	}
}

// BenchmarkKBConcurrentOperations benchmarks concurrent KB access
func BenchmarkKBConcurrentOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping KB benchmarks in short mode")
	}

	env := setupKBBenchmarkEnvironment(b)
	defer env.Cleanup()

	ctx := context.Background()
	language := "java"

	concurrencyLevels := []int{1, 2, 4, 8}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent_%d", concurrency), func(b *testing.B) {
			b.SetParallelism(concurrency)

			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					errorSig := fmt.Sprintf("concurrent-error-%d", i%10)
					runID := fmt.Sprintf("concurrent-run-%d-%d", concurrency, i)

					caseRecord := generateTestHealingCase(runID, errorSig, i%2 == 0)

					// Concurrent KB write - should fail initially due to missing optimizations
					err := env.KBStorage.WriteCase(ctx, language, errorSig, runID, caseRecord)
					if err != nil {
						b.Logf("concurrent KB operation failed (expected): %v", err)
					}

					i++
				}
			})
		})
	}
}

// Performance target constants for KB operations
const (
	MaxCaseWriteTime    = 150 * time.Millisecond // Target for writing a single case
	MaxCaseReadTime     = 100 * time.Millisecond // Target for reading case history
	MaxSummaryWriteTime = 500 * time.Millisecond // Target for writing summary
	MaxPatchStoreTime   = 100 * time.Millisecond // Target for storing 4KB patch
	MaxKBMemoryUsageMB  = 256                    // Target memory usage for KB operations
)

// BenchmarkKBMemoryEfficiency tracks memory usage during KB operations
func BenchmarkKBMemoryEfficiency(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping KB benchmarks in short mode")
	}

	env := setupKBBenchmarkEnvironment(b)
	defer env.Cleanup()

	ctx := context.Background()
	language := "java"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		errorSig := fmt.Sprintf("memory-test-error-%d", i%20) // 20 distinct errors
		runID := fmt.Sprintf("memory-run-%d", i)

		// Create larger case for memory testing
		caseRecord := generateTestHealingCase(runID, errorSig, true)
		caseRecord.BuildLogs.Stdout = generateTestPatch(8192) // 8KB logs

		// Write case and measure memory impact
		err := env.KBStorage.WriteCase(ctx, language, errorSig, runID, caseRecord)
		if err != nil {
			b.Logf("KB memory test iteration %d failed (expected): %v", i, err)
		}
	}
}
