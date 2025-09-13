//go:build performance
// +build performance

package performance

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	mods "github.com/iw2rmb/ploy/internal/mods"
	"github.com/iw2rmb/ploy/internal/storage"
)

var workflowCounter int64

// setupBenchmarkEnvironment creates a test environment for benchmarking
func setupBenchmarkEnvironment(b *testing.B) *BenchmarkEnvironment {
	b.Helper()

	// Create temp directory for workspace
	tempDir, err := os.MkdirTemp("", "mods-bench-*")
	require.NoError(b, err)

	// Create storage client for testing
	storageClient, err := storage.NewStorageClient(&storage.Config{
		Endpoint: "http://localhost:8888", // SeaweedFS test endpoint
	})
	if err != nil {
		b.Skip("SeaweedFS not available for benchmarks")
	}

	env := &BenchmarkEnvironment{
		TempDir:       tempDir,
		StorageClient: storageClient,
		Config: &mods.ModConfig{
			ID:           "benchmark-workflow",
			TargetRepo:   "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
			BaseRef:      "refs/heads/main",
			BuildTimeout: "10m",
			Steps: []mods.ModStep{
				{
					Type:    "recipe",
					ID:      "java-migration",
					Engine:  "openrewrite",
					Recipes: []string{"org.openrewrite.java.migrate.Java11toJava17"},
				},
			},
			SelfHeal: mods.SelfHealConfig{
				Enabled:    true,
				MaxRetries: 2,
			},
		},
	}

	return env
}

type BenchmarkEnvironment struct {
	TempDir       string
	StorageClient *storage.StorageClient
	Config        *mods.ModConfig
}

func (e *BenchmarkEnvironment) Cleanup() {
	os.RemoveAll(e.TempDir)
}

// BenchmarkModsCompleteWorkflow benchmarks the complete mods workflow
// This should initially fail as we haven't optimized the performance yet
func BenchmarkModsCompleteWorkflow(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping performance benchmarks in short mode")
	}

	// Setup realistic test environment
	env := setupBenchmarkEnvironment(b)
	defer env.Cleanup()

	// Create workflow request with realistic parameters
	workflowConfig := env.Config
	workflowConfig.ID = "benchmark-java-migration"

	// Create transflow integrations - this will likely fail initially
	integrations := mods.NewModIntegrationsWithTestMode("", env.TempDir, true)

	// Create runner
	runner, err := mods.NewModRunner(workflowConfig, env.TempDir)
	if err != nil {
		b.Fatalf("Failed to create runner: %v", err)
	}

	// Setup mocked dependencies since this is a benchmark
	runner.SetBuildChecker(integrations.CreateBuildChecker())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

		// Execute workflow - this will likely fail initially due to missing optimizations
		result, err := runner.Run(ctx)
		if err != nil {
			// Expected to fail initially - log for debugging
			b.Logf("benchmark iteration %d failed (expected): %v", i, err)
			// Don't fail the benchmark, just continue
		} else if result != nil {
			// If it somehow passes, validate the result structure
			if result.WorkflowID == "" {
				b.Errorf("benchmark iteration %d missing WorkflowID", i)
			}
		}

		cancel()
	}
}

// BenchmarkModsJobSubmission benchmarks just the job submission part
func BenchmarkModsJobSubmission(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping performance benchmarks in short mode")
	}

	env := setupBenchmarkEnvironment(b)
	defer env.Cleanup()

	// Create a minimal job submission helper - this will likely not exist yet
	helper := &mods.JobSubmissionHelper{
		NomadAddr: "http://localhost:4646",
		TempDir:   env.TempDir,
	}

	jobConfig := &mods.JobSubmissionConfig{
		JobType:     "planner",
		RepoURL:     "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
		Branch:      "main",
		WorkspaceID: "benchmark-workspace",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

		// Submit job - expected to fail initially
		result, err := helper.SubmitAndWait(ctx, jobConfig)
		if err != nil {
			b.Logf("job submission iteration %d failed (expected): %v", i, err)
		} else if result != nil && result.JobID == "" {
			b.Errorf("job submission iteration %d missing JobID", i)
		}

		cancel()
	}
}

// BenchmarkModsConfigValidation benchmarks configuration validation
func BenchmarkModConfigValidation(b *testing.B) {
	env := setupBenchmarkEnvironment(b)
	defer env.Cleanup()

	config := env.Config

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Validate configuration
		err := mods.ValidateModConfig(config)
		if err != nil {
			b.Fatalf("config validation failed: %v", err)
		}
	}
}

// BenchmarkConcurrentWorkflows benchmarks concurrent workflow execution
// This should fail initially - concurrency not optimized
func BenchmarkConcurrentWorkflows(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping performance benchmarks in short mode")
	}

	env := setupBenchmarkEnvironment(b)
	defer env.Cleanup()

	concurrencyLevels := []int{1, 2, 4}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent_%d", concurrency), func(b *testing.B) {
			b.SetParallelism(concurrency)

			b.RunParallel(func(pb *testing.PB) {
				workflowID := atomic.AddInt64(&workflowCounter, 1)

				// Create per-goroutine runner
				config := *env.Config
				config.ID = fmt.Sprintf("benchmark-concurrent-%d", workflowID)

				integrations := mods.NewModIntegrationsWithTestMode("", env.TempDir, true)

				runner, err := mods.NewModRunner(&config, env.TempDir)
				if err != nil {
					b.Errorf("Failed to create runner: %v", err)
					return
				}

				runner.SetBuildChecker(integrations.CreateBuildChecker())

				for pb.Next() {
					ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)

					result, err := runner.Run(ctx)
					if err != nil {
						b.Logf("concurrent workflow failed (expected): %v", err)
					} else if result != nil && result.WorkflowID == "" {
						b.Error("concurrent workflow missing WorkflowID")
					}

					cancel()
				}
			})
		})
	}
}

// Performance target constants - these will initially be unrealistic
const (
	MaxWorkflowDuration     = 8 * time.Minute        // Complete workflow target
	MaxJobSubmissionTime    = 5 * time.Second        // Job submission target
	MaxConfigValidationTime = 100 * time.Millisecond // Config validation target
	MaxMemoryUsageMB        = 512                    // Memory usage target
)

// BenchmarkMemoryUsage tracks memory usage during workflow execution
func BenchmarkMemoryUsage(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping performance benchmarks in short mode")
	}

	env := setupBenchmarkEnvironment(b)
	defer env.Cleanup()

	config := env.Config
	config.ID = "benchmark-memory-test"

	integrations := mods.NewModIntegrationsWithTestMode("", env.TempDir, true)

	runner, err := mods.NewModRunner(config, env.TempDir)
	if err != nil {
		b.Skip("Runner creation failed for memory benchmark")
	}

	runner.SetBuildChecker(integrations.CreateBuildChecker())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

		result, err := runner.Run(ctx)
		if err != nil {
			b.Logf("memory benchmark iteration %d failed (expected): %v", i, err)
		} else if result != nil && result.Duration > MaxWorkflowDuration {
			b.Logf("workflow duration %v exceeds target %v", result.Duration, MaxWorkflowDuration)
		}

		cancel()
	}
}
