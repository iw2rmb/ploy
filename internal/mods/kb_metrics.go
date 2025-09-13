package mods

import (
	"context"
	"sync"
	"time"
)

// MetricsConfig contains configuration for KB metrics collection
type MetricsConfig struct {
	// Collection settings
	EnableMetrics        bool          `json:"enable_metrics"`         // true
	CollectionInterval   time.Duration `json:"collection_interval"`    // 15m
	MetricsRetentionDays int           `json:"metrics_retention_days"` // 30

	// Performance tracking
	TrackQueryTimes        bool `json:"track_query_times"`        // true
	TrackStorageUsage      bool `json:"track_storage_usage"`      // true
	TrackDeduplicationRate bool `json:"track_deduplication_rate"` // true

	// Alert thresholds
	StorageGrowthThreshold float64       `json:"storage_growth_threshold"` // 0.1 (10%)
	QueryTimeThreshold     time.Duration `json:"query_time_threshold"`     // 5s
	DuplicationThreshold   float64       `json:"duplication_threshold"`    // 0.3 (30%)
}

// DefaultMetricsConfig returns reasonable defaults for metrics collection
func DefaultMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		EnableMetrics:          true,
		CollectionInterval:     15 * time.Minute,
		MetricsRetentionDays:   30,
		TrackQueryTimes:        true,
		TrackStorageUsage:      true,
		TrackDeduplicationRate: true,
		StorageGrowthThreshold: 0.1,
		QueryTimeThreshold:     5 * time.Second,
		DuplicationThreshold:   0.3,
	}
}

// KBMetrics tracks Knowledge Base performance and deduplication metrics
type KBMetrics struct {
	Timestamp time.Time `json:"timestamp"`

	// Storage metrics
	TotalCases          int   `json:"total_cases"`
	TotalSignatures     int   `json:"total_signatures"`
	TotalPatches        int   `json:"total_patches"`
	StorageSizeBytes    int64 `json:"storage_size_bytes"`
	CompressedSizeBytes int64 `json:"compressed_size_bytes"`

	// Deduplication metrics
	DuplicateCasesFound    int     `json:"duplicate_cases_found"`
	SimilarPatchesFound    int     `json:"similar_patches_found"`
	DeduplicationRate      float64 `json:"deduplication_rate"`
	StorageSavedBytes      int64   `json:"storage_saved_bytes"`
	StorageSavedPercentage float64 `json:"storage_saved_percentage"`

	// Performance metrics
	AvgQueryTimeMs          int64   `json:"avg_query_time_ms"`
	P95QueryTimeMs          int64   `json:"p95_query_time_ms"`
	P99QueryTimeMs          int64   `json:"p99_query_time_ms"`
	CacheHitRate            float64 `json:"cache_hit_rate"`
	SimilarityComputeTimeMs int64   `json:"similarity_compute_time_ms"`

	// Maintenance metrics
	LastCompactionTime      time.Time `json:"last_compaction_time"`
	CompactionDurationMs    int64     `json:"compaction_duration_ms"`
	CompactionEffectiveness float64   `json:"compaction_effectiveness"`

	// Error metrics
	QueryErrors      int `json:"query_errors"`
	StorageErrors    int `json:"storage_errors"`
	CompactionErrors int `json:"compaction_errors"`
	SimilarityErrors int `json:"similarity_errors"`
}

// PerformanceTracker tracks KB operation performance
type PerformanceTracker struct {
	config     *MetricsConfig
	metrics    *KBMetrics
	mutex      sync.RWMutex
	queryTimes []time.Duration

	// Running counters
	queriesProcessed       int64
	casesProcessed         int64
	similarityComputations int64
}

// NewPerformanceTracker creates a new performance tracker
func NewPerformanceTracker(config *MetricsConfig) *PerformanceTracker {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	return &PerformanceTracker{
		config:     config,
		metrics:    &KBMetrics{Timestamp: time.Now()},
		queryTimes: make([]time.Duration, 0, 1000),
	}
}

// TrackQuery tracks the performance of a KB query operation
func (pt *PerformanceTracker) TrackQuery(duration time.Duration) {
	if !pt.config.TrackQueryTimes {
		return
	}

	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.queryTimes = append(pt.queryTimes, duration)
	pt.queriesProcessed++

	// Keep only recent query times for percentile calculations
	if len(pt.queryTimes) > 1000 {
		pt.queryTimes = pt.queryTimes[500:]
	}
}

// TrackCaseProcessing tracks case processing operations
func (pt *PerformanceTracker) TrackCaseProcessing(count int) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.casesProcessed += int64(count)
}

// TrackSimilarityComputation tracks similarity computation performance
func (pt *PerformanceTracker) TrackSimilarityComputation(duration time.Duration) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.similarityComputations++
	pt.metrics.SimilarityComputeTimeMs = duration.Milliseconds()
}

// TrackError tracks various types of errors
func (pt *PerformanceTracker) TrackError(errorType string) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	switch errorType {
	case "query":
		pt.metrics.QueryErrors++
	case "storage":
		pt.metrics.StorageErrors++
	case "compaction":
		pt.metrics.CompactionErrors++
	case "similarity":
		pt.metrics.SimilarityErrors++
	}
}

// UpdateStorageMetrics updates storage-related metrics
func (pt *PerformanceTracker) UpdateStorageMetrics(totalCases, totalSigs, totalPatches int, storageSize, compressedSize int64) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.metrics.TotalCases = totalCases
	pt.metrics.TotalSignatures = totalSigs
	pt.metrics.TotalPatches = totalPatches
	pt.metrics.StorageSizeBytes = storageSize
	pt.metrics.CompressedSizeBytes = compressedSize
}

// UpdateDeduplicationMetrics updates deduplication effectiveness metrics
func (pt *PerformanceTracker) UpdateDeduplicationMetrics(duplicates, similar int, savedBytes int64) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.metrics.DuplicateCasesFound = duplicates
	pt.metrics.SimilarPatchesFound = similar
	pt.metrics.StorageSavedBytes = savedBytes

	// Calculate rates
	if pt.metrics.TotalCases > 0 {
		pt.metrics.DeduplicationRate = float64(duplicates) / float64(pt.metrics.TotalCases)
	}

	if pt.metrics.StorageSizeBytes > 0 {
		pt.metrics.StorageSavedPercentage = float64(savedBytes) / float64(pt.metrics.StorageSizeBytes) * 100
	}
}

// UpdateCompactionMetrics updates compaction performance metrics
func (pt *PerformanceTracker) UpdateCompactionMetrics(duration time.Duration, effectiveness float64) {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	pt.metrics.LastCompactionTime = time.Now()
	pt.metrics.CompactionDurationMs = duration.Milliseconds()
	pt.metrics.CompactionEffectiveness = effectiveness
}

// ComputePerformanceMetrics calculates derived performance metrics
func (pt *PerformanceTracker) ComputePerformanceMetrics() {
	pt.mutex.Lock()
	defer pt.mutex.Unlock()

	if len(pt.queryTimes) == 0 {
		return
	}

	// Calculate average query time
	var total time.Duration
	for _, d := range pt.queryTimes {
		total += d
	}
	pt.metrics.AvgQueryTimeMs = (total / time.Duration(len(pt.queryTimes))).Milliseconds()

	// Calculate percentiles
	sortedTimes := make([]time.Duration, len(pt.queryTimes))
	copy(sortedTimes, pt.queryTimes)

	// Simple sort for percentile calculation
	for i := 0; i < len(sortedTimes); i++ {
		for j := i + 1; j < len(sortedTimes); j++ {
			if sortedTimes[i] > sortedTimes[j] {
				sortedTimes[i], sortedTimes[j] = sortedTimes[j], sortedTimes[i]
			}
		}
	}

	// Calculate P95 and P99
	p95Index := int(float64(len(sortedTimes)) * 0.95)
	p99Index := int(float64(len(sortedTimes)) * 0.99)

	if p95Index < len(sortedTimes) {
		pt.metrics.P95QueryTimeMs = sortedTimes[p95Index].Milliseconds()
	}

	if p99Index < len(sortedTimes) {
		pt.metrics.P99QueryTimeMs = sortedTimes[p99Index].Milliseconds()
	}
}

// GetCurrentMetrics returns current metrics snapshot
func (pt *PerformanceTracker) GetCurrentMetrics() KBMetrics {
	pt.mutex.RLock()
	defer pt.mutex.RUnlock()

	// Update timestamp and compute derived metrics
	pt.metrics.Timestamp = time.Now()
	pt.ComputePerformanceMetrics()

	return *pt.metrics
}

// MetricsCollector handles periodic metrics collection and alerting
type MetricsCollector struct {
	storage      KBStorage
	sigGenerator EnhancedSignatureGenerator
	perfTracker  *PerformanceTracker
	config       *MetricsConfig

	// Historical data
	historicalMetrics []KBMetrics
	mutex             sync.RWMutex
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(
	storage KBStorage,
	sigGenerator EnhancedSignatureGenerator,
	config *MetricsConfig,
) *MetricsCollector {
	if config == nil {
		config = DefaultMetricsConfig()
	}

	return &MetricsCollector{
		storage:           storage,
		sigGenerator:      sigGenerator,
		perfTracker:       NewPerformanceTracker(config),
		config:            config,
		historicalMetrics: make([]KBMetrics, 0),
	}
}

// StartCollection begins periodic metrics collection
func (mc *MetricsCollector) StartCollection(ctx context.Context) error {
	if !mc.config.EnableMetrics {
		return nil
	}

	ticker := time.NewTicker(mc.config.CollectionInterval)
	defer ticker.Stop()

	// Collect initial metrics
	if err := mc.collectMetrics(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := mc.collectMetrics(ctx); err != nil {
				// Log error but continue collection
				continue
			}
		}
	}
}

// collectMetrics performs a metrics collection cycle
func (mc *MetricsCollector) collectMetrics(ctx context.Context) error {
	// Read KB snapshot for basic counts
	snapshot, err := mc.storage.ReadSnapshot(ctx)
	if err != nil {
		mc.perfTracker.TrackError("storage")
		return err
	}

	// Update storage metrics
	totalCases := snapshot.TotalCases
	totalSigs := snapshot.TotalSigs
	totalPatches := 0          // Would need to count patches from storage
	storageSize := int64(0)    // Would need to calculate actual storage size
	compressedSize := int64(0) // Would need compression metrics

	mc.perfTracker.UpdateStorageMetrics(totalCases, totalSigs, totalPatches, storageSize, compressedSize)

	// Analyze deduplication opportunities
	if mc.config.TrackDeduplicationRate {
		duplicates, similar, savedBytes := mc.analyzeDeduplication(ctx)
		mc.perfTracker.UpdateDeduplicationMetrics(duplicates, similar, savedBytes)
	}

	// Get current metrics and store them
	currentMetrics := mc.perfTracker.GetCurrentMetrics()
	mc.storeMetrics(currentMetrics)

	// Check for alert conditions
	mc.checkAlerts(currentMetrics)

	return nil
}

// analyzeDeduplication analyzes current deduplication effectiveness
func (mc *MetricsCollector) analyzeDeduplication(ctx context.Context) (duplicates, similar int, savedBytes int64) {
	// This would implement analysis of the current KB to find deduplication opportunities
	// For now, return placeholder values
	return 0, 0, 0
}

// storeMetrics stores metrics in historical data
func (mc *MetricsCollector) storeMetrics(metrics KBMetrics) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.historicalMetrics = append(mc.historicalMetrics, metrics)

	// Retain only recent metrics based on retention policy
	retentionCutoff := time.Now().AddDate(0, 0, -mc.config.MetricsRetentionDays)

	var retained []KBMetrics
	for _, m := range mc.historicalMetrics {
		if m.Timestamp.After(retentionCutoff) {
			retained = append(retained, m)
		}
	}
	mc.historicalMetrics = retained
}

// checkAlerts checks metrics against configured thresholds
func (mc *MetricsCollector) checkAlerts(metrics KBMetrics) {
	// Check query time threshold
	if mc.config.TrackQueryTimes && metrics.AvgQueryTimeMs > mc.config.QueryTimeThreshold.Milliseconds() {
		mc.triggerAlert("high_query_time", map[string]interface{}{
			"avg_time_ms":  metrics.AvgQueryTimeMs,
			"threshold_ms": mc.config.QueryTimeThreshold.Milliseconds(),
		})
	}

	// Check deduplication threshold
	if mc.config.TrackDeduplicationRate && metrics.DeduplicationRate > mc.config.DuplicationThreshold {
		mc.triggerAlert("high_duplication_rate", map[string]interface{}{
			"duplication_rate": metrics.DeduplicationRate,
			"threshold":        mc.config.DuplicationThreshold,
		})
	}

	// Check storage growth
	if len(mc.historicalMetrics) > 1 {
		previous := mc.historicalMetrics[len(mc.historicalMetrics)-2]
		if previous.StorageSizeBytes > 0 {
			growthRate := float64(metrics.StorageSizeBytes-previous.StorageSizeBytes) / float64(previous.StorageSizeBytes)
			if growthRate > mc.config.StorageGrowthThreshold {
				mc.triggerAlert("high_storage_growth", map[string]interface{}{
					"growth_rate": growthRate,
					"threshold":   mc.config.StorageGrowthThreshold,
				})
			}
		}
	}
}

// triggerAlert triggers an alert for a specific condition
func (mc *MetricsCollector) triggerAlert(alertType string, details map[string]interface{}) {
	// This would integrate with an alerting system
	// For now, just log the alert
	println("ALERT:", alertType, details)
}

// GetMetricsHistory returns historical metrics
func (mc *MetricsCollector) GetMetricsHistory(ctx context.Context, hours int) []KBMetrics {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	var result []KBMetrics

	for _, m := range mc.historicalMetrics {
		if m.Timestamp.After(cutoff) {
			result = append(result, m)
		}
	}

	return result
}

// GetPerformanceTracker returns the performance tracker for direct use
func (mc *MetricsCollector) GetPerformanceTracker() *PerformanceTracker {
	return mc.perfTracker
}

// MetricsSummary provides a high-level summary of KB effectiveness
type MetricsSummary struct {
	Period              string   `json:"period"`
	TotalQueries        int64    `json:"total_queries"`
	AvgQueryTimeMs      int64    `json:"avg_query_time_ms"`
	StorageEfficiency   float64  `json:"storage_efficiency"`
	DeduplicationRate   float64  `json:"deduplication_rate"`
	CompactionFrequency string   `json:"compaction_frequency"`
	SystemHealth        string   `json:"system_health"`
	RecommendedActions  []string `json:"recommended_actions"`
}

// GenerateMetricsSummary creates a summary of KB performance over a period
func (mc *MetricsCollector) GenerateMetricsSummary(ctx context.Context, hours int) MetricsSummary {
	history := mc.GetMetricsHistory(ctx, hours)

	if len(history) == 0 {
		return MetricsSummary{
			Period:       "No data",
			SystemHealth: "unknown",
		}
	}

	// Calculate averages and trends
	var totalQueries int64
	var totalQueryTime int64
	var totalDeduplicationRate float64

	for _, m := range history {
		totalQueryTime += m.AvgQueryTimeMs
		totalDeduplicationRate += m.DeduplicationRate
	}

	avgQueryTime := totalQueryTime / int64(len(history))
	avgDeduplicationRate := totalDeduplicationRate / float64(len(history))

	latest := history[len(history)-1]

	// Calculate storage efficiency
	storageEfficiency := float64(100)
	if latest.StorageSizeBytes > 0 {
		storageEfficiency = (1.0 - float64(latest.StorageSavedBytes)/float64(latest.StorageSizeBytes)) * 100
	}

	// Determine system health
	systemHealth := "healthy"
	var recommendations []string

	if avgQueryTime > mc.config.QueryTimeThreshold.Milliseconds() {
		systemHealth = "degraded"
		recommendations = append(recommendations, "Consider running compaction to improve query performance")
	}

	if avgDeduplicationRate > mc.config.DuplicationThreshold {
		systemHealth = "degraded"
		recommendations = append(recommendations, "High duplication rate detected - run deduplication job")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "System operating normally")
	}

	return MetricsSummary{
		Period:              getTimeAgoString(hours),
		TotalQueries:        totalQueries,
		AvgQueryTimeMs:      avgQueryTime,
		StorageEfficiency:   storageEfficiency,
		DeduplicationRate:   avgDeduplicationRate,
		CompactionFrequency: "24h",
		SystemHealth:        systemHealth,
		RecommendedActions:  recommendations,
	}
}

// Helper function to format time period strings
func getTimeAgoString(hours int) string {
	if hours < 24 {
		return "Last " + string(rune(hours)) + " hours"
	} else if hours < 24*7 {
		days := hours / 24
		return "Last " + string(rune(days)) + " days"
	} else {
		weeks := hours / (24 * 7)
		return "Last " + string(rune(weeks)) + " weeks"
	}
}
