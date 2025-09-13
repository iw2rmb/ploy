package transflow

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformanceAnalyzer_StorageAnalysis(t *testing.T) {
	mockStorage := &CompatibilityMockKBStorage{}
	config := DefaultPerformanceConfig()
	analyzer := NewPerformanceAnalyzer(mockStorage, config)

	ctx := context.Background()
	analysis, err := analyzer.AnalyzeStoragePerformance(ctx)

	require.NoError(t, err)
	assert.NotNil(t, analysis)

	// Verify storage reduction meets 50% target
	assert.Greater(t, analysis.StorageReduction, 0.5, "Storage reduction should exceed 50%")

	// Verify realistic case counts
	assert.Equal(t, config.SampleSize, analysis.OriginalCases)
	assert.Less(t, analysis.DeduplicatedCases, analysis.OriginalCases)

	// Verify savings calculation
	assert.Greater(t, analysis.EstimatedSavingsBytes, int64(0), "Should show storage savings")

	// Verify patch compression
	assert.Greater(t, analysis.PatchReduction, 0.0, "Should achieve patch compression")
	assert.Less(t, analysis.CompressedPatchCount, analysis.OriginalPatchCount)
}

func TestPerformanceAnalyzer_QueryAnalysis(t *testing.T) {
	mockStorage := &CompatibilityMockKBStorage{}
	config := DefaultPerformanceConfig()
	analyzer := NewPerformanceAnalyzer(mockStorage, config)

	ctx := context.Background()
	analysis, err := analyzer.AnalyzeQueryPerformance(ctx)

	require.NoError(t, err)
	assert.NotNil(t, analysis)

	// Verify query speed improvement meets 25% target
	assert.Greater(t, analysis.QuerySpeedImprovement, 25.0, "Query speed improvement should exceed 25%")

	// Verify performance improvements are realistic
	assert.Greater(t, analysis.OriginalAvgQueryTime, analysis.OptimizedAvgQueryTime, "Optimized queries should be faster")
	assert.Greater(t, analysis.OriginalP95QueryTime, analysis.OptimizedP95QueryTime, "P95 should improve")

	// Verify index size reduction
	assert.Greater(t, analysis.IndexSizeReduction, 0.0, "Index size should be reduced")
	assert.Less(t, analysis.OptimizedIndexSize, analysis.OriginalIndexSize)

	// Verify cache hit ratio is reasonable
	assert.Greater(t, analysis.CacheHitRatio, 0.0)
	assert.Less(t, analysis.CacheHitRatio, 1.0)
}

func TestPerformanceAnalyzer_ComprehensiveReport(t *testing.T) {
	mockStorage := &CompatibilityMockKBStorage{}
	config := DefaultPerformanceConfig()
	analyzer := NewPerformanceAnalyzer(mockStorage, config)

	ctx := context.Background()
	report, err := analyzer.GeneratePerformanceReport(ctx)

	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.NotNil(t, report.StorageAnalysis)
	assert.NotNil(t, report.QueryAnalysis)
	assert.NotEmpty(t, report.Summary)

	// Verify both targets are met
	assert.True(t, report.TargetsMet, "Both performance targets should be met")

	// Verify individual target achievement
	storageTarget := report.StorageAnalysis.StorageReduction >= 0.5   // 50%
	queryTarget := report.QueryAnalysis.QuerySpeedImprovement >= 25.0 // 25%

	assert.True(t, storageTarget, "Storage reduction target should be met")
	assert.True(t, queryTarget, "Query speed improvement target should be met")
}

func TestPerformanceAnalyzer_ValidateTargets(t *testing.T) {
	mockStorage := &CompatibilityMockKBStorage{}
	config := DefaultPerformanceConfig()
	analyzer := NewPerformanceAnalyzer(mockStorage, config)

	ctx := context.Background()
	err := analyzer.ValidatePerformanceTargets(ctx)

	assert.NoError(t, err, "Performance targets validation should pass")
}

func TestPerformanceAnalyzer_CustomConfig(t *testing.T) {
	mockStorage := &CompatibilityMockKBStorage{}

	// Test with high deduplication scenario
	highDedupConfig := &PerformanceConfig{
		SampleSize:                1000,
		ExpectedDuplicationRate:   0.60, // 60% duplicates
		QueryCacheHitRatio:        0.50, // 50% cache hits
		CompressionRatio:          0.40, // 40% patch compression
		SignatureSimilarityThresh: 0.80, // 80% similarity threshold
	}

	analyzer := NewPerformanceAnalyzer(mockStorage, highDedupConfig)
	ctx := context.Background()

	report, err := analyzer.GeneratePerformanceReport(ctx)
	require.NoError(t, err)

	// Should achieve even better performance with high deduplication
	assert.Greater(t, report.StorageAnalysis.StorageReduction, 0.45, "High dedup should achieve >45% storage reduction")
	assert.Greater(t, report.QueryAnalysis.QuerySpeedImprovement, 20.0, "High dedup should achieve >20% query improvement")
}

func TestPerformanceAnalyzer_LowDuplicationScenario(t *testing.T) {
	mockStorage := &CompatibilityMockKBStorage{}

	// Test with minimal deduplication scenario
	lowDedupConfig := &PerformanceConfig{
		SampleSize:                1000,
		ExpectedDuplicationRate:   0.20, // Only 20% duplicates
		QueryCacheHitRatio:        0.25, // 25% cache hits
		CompressionRatio:          0.15, // 15% patch compression
		SignatureSimilarityThresh: 0.90, // 90% similarity threshold (strict)
	}

	analyzer := NewPerformanceAnalyzer(mockStorage, lowDedupConfig)
	ctx := context.Background()

	report, err := analyzer.GeneratePerformanceReport(ctx)
	require.NoError(t, err)

	// Should still meet minimum targets even with low deduplication
	assert.Greater(t, report.StorageAnalysis.StorageReduction, 0.15, "Even low dedup should achieve >15% storage reduction")
	assert.Greater(t, report.QueryAnalysis.QuerySpeedImprovement, 12.0, "Even low dedup should achieve >12% query improvement")
}

func TestPerformanceAnalyzer_RealisticTimings(t *testing.T) {
	mockStorage := &CompatibilityMockKBStorage{}
	config := DefaultPerformanceConfig()
	analyzer := NewPerformanceAnalyzer(mockStorage, config)

	ctx := context.Background()
	analysis, err := analyzer.AnalyzeQueryPerformance(ctx)

	require.NoError(t, err)

	// Verify timing improvements are realistic (not impossibly fast)
	assert.Greater(t, analysis.OptimizedAvgQueryTime, time.Millisecond, "Optimized queries should take >1ms")
	assert.Less(t, analysis.OptimizedAvgQueryTime, analysis.OriginalAvgQueryTime, "Should be faster than original")

	// Verify P95 improvement
	improvementRatio := float64(analysis.OriginalP95QueryTime) / float64(analysis.OptimizedP95QueryTime)
	assert.Greater(t, improvementRatio, 1.25, "P95 should improve by at least 25%")
}

func TestDefaultPerformanceConfig(t *testing.T) {
	config := DefaultPerformanceConfig()

	assert.Equal(t, 1000, config.SampleSize)
	assert.Equal(t, 0.35, config.ExpectedDuplicationRate)
	assert.Equal(t, 0.40, config.QueryCacheHitRatio)
	assert.Equal(t, 0.25, config.CompressionRatio)
	assert.Equal(t, 0.85, config.SignatureSimilarityThresh)

	// Verify realistic ranges
	assert.Greater(t, config.ExpectedDuplicationRate, 0.0)
	assert.Less(t, config.ExpectedDuplicationRate, 1.0)
	assert.Greater(t, config.QueryCacheHitRatio, 0.0)
	assert.Less(t, config.QueryCacheHitRatio, 1.0)
}

// Benchmark test to ensure performance analysis is efficient
func BenchmarkPerformanceAnalysis(b *testing.B) {
	mockStorage := &CompatibilityMockKBStorage{}
	config := DefaultPerformanceConfig()
	config.SampleSize = 10000 // Larger sample for benchmarking
	analyzer := NewPerformanceAnalyzer(mockStorage, config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzer.GeneratePerformanceReport(ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestPerformanceReport_Summary(t *testing.T) {
	mockStorage := &CompatibilityMockKBStorage{}
	config := DefaultPerformanceConfig()
	analyzer := NewPerformanceAnalyzer(mockStorage, config)

	ctx := context.Background()
	report, err := analyzer.GeneratePerformanceReport(ctx)
	require.NoError(t, err)

	// Verify summary contains key metrics
	assert.Contains(t, report.Summary, "Storage Reduction")
	assert.Contains(t, report.Summary, "Query Speed Improvement")
	assert.Contains(t, report.Summary, "OVERALL TARGETS MET")
	assert.Contains(t, report.Summary, "fuzzy signature matching")
	assert.Contains(t, report.Summary, "Content-addressed patch storage")

	// Verify numerical formatting in summary
	assert.Contains(t, report.Summary, "%")  // Percentage values
	assert.Contains(t, report.Summary, "MB") // Storage savings in MB
}
