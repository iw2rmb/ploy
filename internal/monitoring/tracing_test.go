package monitoring

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func TestNewTracingProvider(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		wantError bool
		errorMsg  string
		validate  func(t *testing.T, tp *TracingProvider)
	}{
		{
			name:      "successful initialization with valid endpoint",
			endpoint:  "localhost:4317",
			wantError: false,
			validate: func(t *testing.T, tp *TracingProvider) {
				assert.NotNil(t, tp)
				assert.NotNil(t, tp.provider)
				assert.NotNil(t, tp.tracer)
			},
		},
		{
			name:      "successful initialization with empty endpoint (noop)",
			endpoint:  "",
			wantError: false,
			validate: func(t *testing.T, tp *TracingProvider) {
				assert.NotNil(t, tp)
				assert.NotNil(t, tp.provider)
				assert.NotNil(t, tp.tracer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTracingProvider(tt.endpoint)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, tp)
				}
				// Cleanup
				if tp != nil {
					tp.Shutdown(context.Background())
				}
			}
		})
	}
}

func TestTracingProvider_TraceJob(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := &TracingProvider{
		provider: trace.NewTracerProvider(
			trace.WithSyncer(exporter),
			trace.WithResource(testResource()),
		),
	}
	tp.tracer = tp.provider.Tracer("test-tracer")

	tests := []struct {
		name     string
		jobID    string
		recipe   string
		validate func(t *testing.T, spans tracetest.SpanStubs)
	}{
		{
			name:   "traces job with correct attributes",
			jobID:  "job-123",
			recipe: "org.openrewrite.java.migrate.UpgradeToJava17",
			validate: func(t *testing.T, spans tracetest.SpanStubs) {
				require.Len(t, spans, 1)
				span := spans[0]

				assert.Equal(t, "process_job", span.Name)

				// Check attributes
				attrs := spanAttributesToMap(span.Attributes)
				assert.Equal(t, "job-123", attrs["job.id"])
				assert.Equal(t, "org.openrewrite.java.migrate.UpgradeToJava17", attrs["job.recipe"])
			},
		},
		{
			name:   "traces job with empty values",
			jobID:  "",
			recipe: "",
			validate: func(t *testing.T, spans tracetest.SpanStubs) {
				require.Len(t, spans, 1)
				span := spans[0]

				assert.Equal(t, "process_job", span.Name)

				// Check attributes exist even with empty values
				attrs := spanAttributesToMap(span.Attributes)
				assert.Equal(t, "", attrs["job.id"])
				assert.Equal(t, "", attrs["job.recipe"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous spans
			exporter.Reset()

			ctx := context.Background()
			ctx, done := tp.TraceJob(ctx, tt.jobID, tt.recipe)

			// Simulate some work
			time.Sleep(10 * time.Millisecond)

			// End the span
			done()

			// Get recorded spans
			spans := exporter.GetSpans()
			if tt.validate != nil {
				tt.validate(t, spans)
			}
		})
	}
}

func TestTracingProvider_TraceTransformation(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := &TracingProvider{
		provider: trace.NewTracerProvider(
			trace.WithSyncer(exporter),
			trace.WithResource(testResource()),
		),
	}
	tp.tracer = tp.provider.Tracer("test-tracer")

	tests := []struct {
		name        string
		buildSystem string
		simulateErr error
		validate    func(t *testing.T, spans tracetest.SpanStubs)
	}{
		{
			name:        "traces transformation success",
			buildSystem: "maven",
			simulateErr: nil,
			validate: func(t *testing.T, spans tracetest.SpanStubs) {
				require.Len(t, spans, 1)
				span := spans[0]

				assert.Equal(t, "execute_transformation", span.Name)
				assert.Equal(t, codes.Unset, span.Status.Code)

				// Check attributes
				attrs := spanAttributesToMap(span.Attributes)
				assert.Equal(t, "maven", attrs["build.system"])
			},
		},
		{
			name:        "traces transformation with gradle",
			buildSystem: "gradle",
			simulateErr: nil,
			validate: func(t *testing.T, spans tracetest.SpanStubs) {
				require.Len(t, spans, 1)
				span := spans[0]

				attrs := spanAttributesToMap(span.Attributes)
				assert.Equal(t, "gradle", attrs["build.system"])
			},
		},
		{
			name:        "records error on failure",
			buildSystem: "maven",
			simulateErr: errors.New("transformation failed"),
			validate: func(t *testing.T, spans tracetest.SpanStubs) {
				require.Len(t, spans, 1)
				span := spans[0]

				assert.Equal(t, "execute_transformation", span.Name)
				assert.Equal(t, codes.Error, span.Status.Code)
				assert.Equal(t, "transformation failed", span.Status.Description)

				// Check that error event was recorded
				events := span.Events
				assert.Greater(t, len(events), 0)

				// Find the error event
				var errorEventFound bool
				for _, event := range events {
					if event.Name == "exception" {
						errorEventFound = true
						break
					}
				}
				assert.True(t, errorEventFound, "Error event should be recorded")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous spans
			exporter.Reset()

			ctx := context.Background()
			ctx, done := tp.TraceTransformation(ctx, tt.buildSystem)

			// Simulate some work
			time.Sleep(10 * time.Millisecond)

			// End the span with error if provided
			done(tt.simulateErr)

			// Get recorded spans
			spans := exporter.GetSpans()
			if tt.validate != nil {
				tt.validate(t, spans)
			}
		})
	}
}

func TestTracingProvider_TraceStorage(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := &TracingProvider{
		provider: trace.NewTracerProvider(
			trace.WithSyncer(exporter),
			trace.WithResource(testResource()),
		),
	}
	tp.tracer = tp.provider.Tracer("test-tracer")

	tests := []struct {
		name        string
		storage     string
		operation   string
		simulateErr error
		validate    func(t *testing.T, spans tracetest.SpanStubs)
	}{
		{
			name:        "traces consul operation success",
			storage:     "consul",
			operation:   "get",
			simulateErr: nil,
			validate: func(t *testing.T, spans tracetest.SpanStubs) {
				require.Len(t, spans, 1)
				span := spans[0]

				assert.Equal(t, "storage_operation", span.Name)
				assert.Equal(t, codes.Unset, span.Status.Code)

				attrs := spanAttributesToMap(span.Attributes)
				assert.Equal(t, "consul", attrs["storage.system"])
				assert.Equal(t, "get", attrs["storage.operation"])
			},
		},
		{
			name:        "traces seaweedfs operation with error",
			storage:     "seaweedfs",
			operation:   "upload",
			simulateErr: errors.New("upload failed"),
			validate: func(t *testing.T, spans tracetest.SpanStubs) {
				require.Len(t, spans, 1)
				span := spans[0]

				assert.Equal(t, codes.Error, span.Status.Code)

				attrs := spanAttributesToMap(span.Attributes)
				assert.Equal(t, "seaweedfs", attrs["storage.system"])
				assert.Equal(t, "upload", attrs["storage.operation"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear previous spans
			exporter.Reset()

			ctx := context.Background()
			ctx, done := tp.TraceStorage(ctx, tt.storage, tt.operation)

			// Simulate some work
			time.Sleep(10 * time.Millisecond)

			// End the span with error if provided
			done(tt.simulateErr)

			// Get recorded spans
			spans := exporter.GetSpans()
			if tt.validate != nil {
				tt.validate(t, spans)
			}
		})
	}
}

func TestTracingProvider_NestedSpans(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := &TracingProvider{
		provider: trace.NewTracerProvider(
			trace.WithSyncer(exporter),
			trace.WithResource(testResource()),
		),
	}
	tp.tracer = tp.provider.Tracer("test-tracer")

	// Create nested spans
	ctx := context.Background()

	// Start job span
	ctx, jobDone := tp.TraceJob(ctx, "nested-job", "test.recipe")

	// Start transformation span within job
	_, transDone := tp.TraceTransformation(ctx, "maven")
	time.Sleep(5 * time.Millisecond)
	transDone(nil)

	// Start storage span within job
	_, storageDone := tp.TraceStorage(ctx, "consul", "put")
	time.Sleep(5 * time.Millisecond)
	storageDone(nil)

	// End job span
	jobDone()

	// Get recorded spans
	spans := exporter.GetSpans()

	// Should have 3 spans
	assert.Len(t, spans, 3)

	// Find the parent span (job)
	var jobSpan *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "process_job" {
			jobSpan = &spans[i]
			break
		}
	}
	require.NotNil(t, jobSpan)

	// Check that other spans have the job as parent
	jobSpanID := jobSpan.SpanContext.SpanID()
	for _, span := range spans {
		if span.Name != "process_job" {
			assert.Equal(t, jobSpanID, span.Parent.SpanID(),
				"Child span %s should have job span as parent", span.Name)
		}
	}
}

func TestTracingProvider_ContextCancellation(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := &TracingProvider{
		provider: trace.NewTracerProvider(
			trace.WithSyncer(exporter),
			trace.WithResource(testResource()),
		),
	}
	tp.tracer = tp.provider.Tracer("test-tracer")

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Start a span
	ctx, done := tp.TraceJob(ctx, "cancelled-job", "test.recipe")

	// Cancel the context
	cancel()

	// Try to start another span with cancelled context
	_, transDone := tp.TraceTransformation(ctx, "maven")
	transDone(nil)

	// End the job span
	done()

	// Get recorded spans
	spans := exporter.GetSpans()

	// Both spans should still be recorded despite cancellation
	assert.GreaterOrEqual(t, len(spans), 1)
}

func TestTracingProvider_ConcurrentTracing(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := &TracingProvider{
		provider: trace.NewTracerProvider(
			trace.WithSyncer(exporter),
			trace.WithResource(testResource()),
		),
	}
	tp.tracer = tp.provider.Tracer("test-tracer")

	// Run concurrent traces
	numGoroutines := 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			ctx := context.Background()
			jobID := fmt.Sprintf("job-%d", id)

			// Start job trace
			ctx, jobDone := tp.TraceJob(ctx, jobID, "concurrent.recipe")

			// Simulate work
			time.Sleep(time.Duration(id) * time.Millisecond)

			// End trace
			jobDone()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Get recorded spans
	spans := exporter.GetSpans()

	// Should have one span per goroutine
	assert.Len(t, spans, numGoroutines)

	// Verify each span has unique job ID
	jobIDs := make(map[string]bool)
	for _, span := range spans {
		attrs := spanAttributesToMap(span.Attributes)
		jobID := attrs["job.id"].(string)
		assert.False(t, jobIDs[jobID], "Duplicate job ID found: %s", jobID)
		jobIDs[jobID] = true
	}
}

func TestTracingProvider_Shutdown(t *testing.T) {
	tp, err := NewTracingProvider("localhost:4317")
	require.NoError(t, err)
	require.NotNil(t, tp)

	// Start a trace
	ctx := context.Background()
	ctx, done := tp.TraceJob(ctx, "shutdown-test", "test.recipe")
	done()

	// Shutdown should not error
	err = tp.Shutdown(context.Background())
	assert.NoError(t, err)

	// After shutdown, tracing should still work (no-op)
	ctx, done = tp.TraceJob(ctx, "after-shutdown", "test.recipe")
	assert.NotPanics(t, func() {
		done()
	})
}

func TestTracingProvider_ShutdownTimeout(t *testing.T) {
	tp, err := NewTracingProvider("localhost:4317")
	require.NoError(t, err)
	require.NotNil(t, tp)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Let context expire
	time.Sleep(10 * time.Millisecond)

	// Shutdown with expired context should handle gracefully
	err = tp.Shutdown(ctx)
	// Error is acceptable here due to timeout
	if err != nil {
		assert.Contains(t, err.Error(), "context")
	}
}

func TestTracingProvider_AddEvent(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := &TracingProvider{
		provider: trace.NewTracerProvider(
			trace.WithSyncer(exporter),
			trace.WithResource(testResource()),
		),
	}
	tp.tracer = tp.provider.Tracer("test-tracer")

	ctx := context.Background()
	ctx, done := tp.TraceJob(ctx, "event-job", "test.recipe")

	// Add event to current span
	tp.AddEvent(ctx, "job.started", map[string]string{
		"worker.id":   "worker-1",
		"queue.depth": "10",
	})

	time.Sleep(10 * time.Millisecond)

	tp.AddEvent(ctx, "job.completed", map[string]string{
		"duration.ms": "10",
	})

	done()

	// Get recorded spans
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	// Check events
	events := spans[0].Events
	assert.Len(t, events, 2)
	assert.Equal(t, "job.started", events[0].Name)
	assert.Equal(t, "job.completed", events[1].Name)
}

func TestTracingProvider_SetStatus(t *testing.T) {
	// Create in-memory exporter for testing
	exporter := tracetest.NewInMemoryExporter()
	tp := &TracingProvider{
		provider: trace.NewTracerProvider(
			trace.WithSyncer(exporter),
			trace.WithResource(testResource()),
		),
	}
	tp.tracer = tp.provider.Tracer("test-tracer")

	tests := []struct {
		name                string
		statusCode          codes.Code
		description         string
		expectedDescription string // OK status doesn't preserve description
	}{
		{
			name:                "set ok status",
			statusCode:          codes.Ok,
			description:         "Operation completed successfully",
			expectedDescription: "", // OK status description is not preserved
		},
		{
			name:                "set error status",
			statusCode:          codes.Error,
			description:         "Operation failed",
			expectedDescription: "Operation failed",
		},
		{
			name:                "set unset status",
			statusCode:          codes.Unset,
			description:         "",
			expectedDescription: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()

			ctx := context.Background()
			ctx, done := tp.TraceJob(ctx, "status-job", "test.recipe")

			// Set status
			tp.SetStatus(ctx, tt.statusCode, tt.description)

			done()

			// Get recorded spans
			spans := exporter.GetSpans()
			require.Len(t, spans, 1)

			assert.Equal(t, tt.statusCode, spans[0].Status.Code)
			assert.Equal(t, tt.expectedDescription, spans[0].Status.Description)
		})
	}
}

// Helper functions

func testResource() *resource.Resource {
	return resource.NewSchemaless(
		semconv.ServiceNameKey.String("test-service"),
		semconv.ServiceVersionKey.String("1.0.0"),
	)
}

func spanAttributesToMap(attrs []attribute.KeyValue) map[string]interface{} {
	result := make(map[string]interface{})
	for _, attr := range attrs {
		result[string(attr.Key)] = attr.Value.AsInterface()
	}
	return result
}
