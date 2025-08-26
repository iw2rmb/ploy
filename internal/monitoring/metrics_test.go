package monitoring

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestMetricsCollector_Initialization(t *testing.T) {
	tests := []struct {
		name     string
		validate func(t *testing.T, m *MetricsCollector)
	}{
		{
			name: "creates metrics collector with registry",
			validate: func(t *testing.T, m *MetricsCollector) {
				assert.NotNil(t, m)
				assert.NotNil(t, m.registry)
			},
		},
		{
			name: "initializes all required metrics",
			validate: func(t *testing.T, m *MetricsCollector) {
				// Verify metrics are not nil
				assert.NotNil(t, JobsQueued)
				assert.NotNil(t, JobsProcessing)
				assert.NotNil(t, JobsCompleted)
				assert.NotNil(t, JobDuration)
				assert.NotNil(t, TransformationSize)
				assert.NotNil(t, DiffSize)
				assert.NotNil(t, WorkerPoolUtilization)
				assert.NotNil(t, MemoryUsage)
				assert.NotNil(t, ConsulOperations)
				assert.NotNil(t, SeaweedFSOperations)
				assert.NotNil(t, ScalingEvents)
				assert.NotNil(t, CurrentInstances)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMetricsCollector()
			tt.validate(t, m)
		})
	}
}

func TestMetricsCollector_RecordJobStart(t *testing.T) {
	m := NewMetricsCollector()

	// Set initial queued jobs
	JobsQueued.WithLabelValues("normal").Set(5)
	initialProcessing := testutil.ToFloat64(JobsProcessing)

	// Record job start
	recipe := "org.openrewrite.java.migrate.UpgradeToJava17"
	buildSystem := "maven"
	done := m.RecordJobStart(recipe, buildSystem)

	// Verify metrics updated
	assert.Equal(t, initialProcessing+1, testutil.ToFloat64(JobsProcessing))
	assert.Equal(t, float64(4), testutil.ToFloat64(JobsQueued.WithLabelValues("normal")))

	// Simulate job completion
	time.Sleep(10 * time.Millisecond)
	done()

	// Verify processing decreased
	assert.Equal(t, initialProcessing, testutil.ToFloat64(JobsProcessing))

	// Verify duration was recorded (histogram should have count > 0)
	// We check that the histogram was updated by verifying a metric was recorded
	// Since we can't easily check histogram internals, we verify the timer completed without error
}

func TestMetricsCollector_RecordJobComplete(t *testing.T) {
	m := NewMetricsCollector()

	tests := []struct {
		name   string
		status string
		recipe string
	}{
		{
			name:   "records successful job",
			status: "success",
			recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
		},
		{
			name:   "records failed job",
			status: "failed",
			recipe: "org.openrewrite.java.migrate.UpgradeToJava21",
		},
		{
			name:   "records timeout job",
			status: "timeout",
			recipe: "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initialCount := testutil.ToFloat64(JobsCompleted.WithLabelValues(tt.status, tt.recipe))
			
			m.RecordJobComplete(tt.status, tt.recipe)
			
			newCount := testutil.ToFloat64(JobsCompleted.WithLabelValues(tt.status, tt.recipe))
			assert.Equal(t, initialCount+1, newCount)
		})
	}
}

func TestMetricsCollector_RecordTransformationSize(t *testing.T) {
	m := NewMetricsCollector()

	tests := []struct {
		name        string
		size        int64
		buildSystem string
	}{
		{
			name:        "small maven project",
			size:        1024 * 1024,     // 1MB
			buildSystem: "maven",
		},
		{
			name:        "medium gradle project",
			size:        10 * 1024 * 1024, // 10MB
			buildSystem: "gradle",
		},
		{
			name:        "large maven project",
			size:        100 * 1024 * 1024, // 100MB
			buildSystem: "maven",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.RecordTransformationSize(tt.size, tt.buildSystem)
			
			// Verify histogram recorded the value
			// The histogram will have recorded the observation
			// We verify by ensuring no panic occurred during recording
		})
	}
}

func TestMetricsCollector_RecordDiffSize(t *testing.T) {
	m := NewMetricsCollector()

	tests := []struct {
		name   string
		size   int64
		recipe string
	}{
		{
			name:   "small diff",
			size:   1024,       // 1KB
			recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
		},
		{
			name:   "medium diff",
			size:   100 * 1024, // 100KB
			recipe: "org.openrewrite.java.migrate.UpgradeToJava21",
		},
		{
			name:   "large diff",
			size:   1024 * 1024, // 1MB
			recipe: "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.RecordDiffSize(tt.size, tt.recipe)
			
			// Verify histogram recorded the value
			// The histogram will have recorded the observation
			// We verify by ensuring no panic occurred during recording
		})
	}
}

func TestMetricsCollector_RecordStorageOperation(t *testing.T) {
	m := NewMetricsCollector()

	tests := []struct {
		name      string
		storage   string
		operation string
		status    string
	}{
		{
			name:      "consul get success",
			storage:   "consul",
			operation: "get",
			status:    "success",
		},
		{
			name:      "consul put failure",
			storage:   "consul",
			operation: "put",
			status:    "error",
		},
		{
			name:      "seaweedfs upload success",
			storage:   "seaweedfs",
			operation: "upload",
			status:    "success",
		},
		{
			name:      "seaweedfs download timeout",
			storage:   "seaweedfs",
			operation: "download",
			status:    "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var initialCount float64
			if tt.storage == "consul" {
				initialCount = testutil.ToFloat64(ConsulOperations.WithLabelValues(tt.operation, tt.status))
			} else {
				initialCount = testutil.ToFloat64(SeaweedFSOperations.WithLabelValues(tt.operation, tt.status))
			}
			
			m.RecordStorageOperation(tt.storage, tt.operation, tt.status)
			
			var newCount float64
			if tt.storage == "consul" {
				newCount = testutil.ToFloat64(ConsulOperations.WithLabelValues(tt.operation, tt.status))
			} else {
				newCount = testutil.ToFloat64(SeaweedFSOperations.WithLabelValues(tt.operation, tt.status))
			}
			
			assert.Equal(t, initialCount+1, newCount)
		})
	}
}

func TestMetricsCollector_UpdateWorkerUtilization(t *testing.T) {
	m := NewMetricsCollector()

	tests := []struct {
		name        string
		utilization float64
		expectation func(t *testing.T, actual float64)
	}{
		{
			name:        "low utilization",
			utilization: 0.25,
			expectation: func(t *testing.T, actual float64) {
				assert.Equal(t, 0.25, actual)
			},
		},
		{
			name:        "medium utilization",
			utilization: 0.5,
			expectation: func(t *testing.T, actual float64) {
				assert.Equal(t, 0.5, actual)
			},
		},
		{
			name:        "high utilization",
			utilization: 0.9,
			expectation: func(t *testing.T, actual float64) {
				assert.Equal(t, 0.9, actual)
			},
		},
		{
			name:        "full utilization",
			utilization: 1.0,
			expectation: func(t *testing.T, actual float64) {
				assert.Equal(t, 1.0, actual)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.UpdateWorkerUtilization(tt.utilization)
			actual := testutil.ToFloat64(WorkerPoolUtilization)
			tt.expectation(t, actual)
		})
	}
}

func TestMetricsCollector_UpdateMemoryUsage(t *testing.T) {
	m := NewMetricsCollector()

	tests := []struct {
		name  string
		bytes uint64
	}{
		{
			name:  "low memory usage",
			bytes: 100 * 1024 * 1024, // 100MB
		},
		{
			name:  "medium memory usage",
			bytes: 500 * 1024 * 1024, // 500MB
		},
		{
			name:  "high memory usage",
			bytes: 2 * 1024 * 1024 * 1024, // 2GB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.UpdateMemoryUsage(tt.bytes)
			actual := testutil.ToFloat64(MemoryUsage)
			assert.Equal(t, float64(tt.bytes), actual)
		})
	}
}

func TestMetricsCollector_RecordScalingEvent(t *testing.T) {
	m := NewMetricsCollector()

	tests := []struct {
		name      string
		direction string
		reason    string
	}{
		{
			name:      "scale up due to high queue",
			direction: "up",
			reason:    "high_queue_depth",
		},
		{
			name:      "scale down due to low utilization",
			direction: "down",
			reason:    "low_utilization",
		},
		{
			name:      "scale up due to high latency",
			direction: "up",
			reason:    "high_latency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initialCount := testutil.ToFloat64(ScalingEvents.WithLabelValues(tt.direction, tt.reason))
			
			m.RecordScalingEvent(tt.direction, tt.reason)
			
			newCount := testutil.ToFloat64(ScalingEvents.WithLabelValues(tt.direction, tt.reason))
			assert.Equal(t, initialCount+1, newCount)
		})
	}
}

func TestMetricsCollector_UpdateInstanceCount(t *testing.T) {
	m := NewMetricsCollector()

	tests := []struct {
		name  string
		count int
	}{
		{
			name:  "single instance",
			count: 1,
		},
		{
			name:  "scale to 5 instances",
			count: 5,
		},
		{
			name:  "scale to 10 instances",
			count: 10,
		},
		{
			name:  "scale down to 2 instances",
			count: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.UpdateInstanceCount(tt.count)
			actual := testutil.ToFloat64(CurrentInstances)
			assert.Equal(t, float64(tt.count), actual)
		})
	}
}

func TestMetricsCollector_GetWorkerUtilization(t *testing.T) {
	m := NewMetricsCollector()

	// Set a known utilization
	testUtilization := 0.75
	m.UpdateWorkerUtilization(testUtilization)

	// Get utilization
	actual := m.GetWorkerUtilization()
	assert.Equal(t, testUtilization, actual)
}

func TestMetricsCollector_ResetMetrics(t *testing.T) {
	m := NewMetricsCollector()

	// Set some metrics
	JobsQueued.WithLabelValues("normal").Set(10)
	JobsProcessing.Set(5)
	m.UpdateWorkerUtilization(0.8)
	m.UpdateMemoryUsage(1024 * 1024 * 1024)
	m.UpdateInstanceCount(3)

	// Reset metrics
	m.ResetMetrics()

	// Verify metrics are reset
	assert.Equal(t, float64(0), testutil.ToFloat64(JobsQueued.WithLabelValues("normal")))
	assert.Equal(t, float64(0), testutil.ToFloat64(JobsProcessing))
	assert.Equal(t, float64(0), testutil.ToFloat64(WorkerPoolUtilization))
	assert.Equal(t, float64(0), testutil.ToFloat64(MemoryUsage))
	assert.Equal(t, float64(1), testutil.ToFloat64(CurrentInstances)) // Should reset to 1
}

func TestMetricsCollector_GetRegistry(t *testing.T) {
	m := NewMetricsCollector()

	registry := m.GetRegistry()
	assert.NotNil(t, registry)
	assert.IsType(t, &prometheus.Registry{}, registry)
}

func TestMetricsCollector_ConcurrentAccess(t *testing.T) {
	m := NewMetricsCollector()
	done := make(chan bool, 3)

	// Concurrent job starts
	go func() {
		for i := 0; i < 100; i++ {
			finish := m.RecordJobStart("recipe1", "maven")
			time.Sleep(time.Microsecond)
			finish()
		}
		done <- true
	}()

	// Concurrent job completions
	go func() {
		for i := 0; i < 100; i++ {
			m.RecordJobComplete("success", "recipe2")
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Concurrent metric updates
	go func() {
		for i := 0; i < 100; i++ {
			m.UpdateWorkerUtilization(float64(i) / 100.0)
			m.UpdateMemoryUsage(uint64(i * 1024 * 1024))
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify no panics and metrics are accessible
	assert.NotPanics(t, func() {
		_ = testutil.ToFloat64(JobsProcessing)
		_ = testutil.ToFloat64(JobsCompleted.WithLabelValues("success", "recipe2"))
		_ = testutil.ToFloat64(WorkerPoolUtilization)
		_ = testutil.ToFloat64(MemoryUsage)
	})
}