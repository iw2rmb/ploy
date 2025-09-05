package monitoring

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingProvider manages OpenTelemetry tracing for the OpenRewrite service
type TracingProvider struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	exporter *otlptrace.Exporter
}

// NewTracingProvider creates a new tracing provider with OTLP exporter
func NewTracingProvider(endpoint string) (*TracingProvider, error) {
	ctx := context.Background()

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(
			semconv.ServiceNameKey.String("openrewrite-service"),
			semconv.ServiceVersionKey.String("1.0.0"),
			attribute.String("environment", "production"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create OTLP exporter
	var exporter *otlptrace.Exporter
	var provider *sdktrace.TracerProvider

	if endpoint != "" {
		// Create actual exporter for non-empty endpoint
		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(), // Use insecure for local development
			otlptracegrpc.WithTimeout(30*time.Second),
		)
		if err != nil {
			return nil, err
		}

		// Create trace provider with batch processor
		provider = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		)
	} else {
		// Create no-op provider for empty endpoint
		provider = sdktrace.NewTracerProvider(
			sdktrace.WithResource(res),
			sdktrace.WithSampler(sdktrace.NeverSample()),
		)
	}

	// Register as global provider
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &TracingProvider{
		provider: provider,
		tracer:   provider.Tracer("openrewrite"),
		exporter: exporter,
	}, nil
}

// TraceJob starts a trace for a job processing operation
func (t *TracingProvider) TraceJob(ctx context.Context, jobID, recipe string) (context.Context, func()) {
	ctx, span := t.tracer.Start(ctx, "process_job",
		trace.WithAttributes(
			attribute.String("job.id", jobID),
			attribute.String("job.recipe", recipe),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)

	return ctx, func() {
		span.End()
	}
}

// TraceTransformation starts a trace for a transformation execution
func (t *TracingProvider) TraceTransformation(ctx context.Context, buildSystem string) (context.Context, func(error)) {
	ctx, span := t.tracer.Start(ctx, "execute_transformation",
		trace.WithAttributes(
			attribute.String("build.system", buildSystem),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)

	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}

// TraceStorage starts a trace for a storage operation
func (t *TracingProvider) TraceStorage(ctx context.Context, storage, operation string) (context.Context, func(error)) {
	ctx, span := t.tracer.Start(ctx, "storage_operation",
		trace.WithAttributes(
			attribute.String("storage.system", storage),
			attribute.String("storage.operation", operation),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)

	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}
}

// AddEvent adds an event to the current span
func (t *TracingProvider) AddEvent(ctx context.Context, name string, attributes map[string]string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	attrs := make([]attribute.KeyValue, 0, len(attributes))
	for k, v := range attributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetStatus sets the status of the current span
func (t *TracingProvider) SetStatus(ctx context.Context, code codes.Code, description string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.SetStatus(code, description)
}

// RecordError records an error on the current span
func (t *TracingProvider) RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.RecordError(err)
}

// SetAttributes sets attributes on the current span
func (t *TracingProvider) SetAttributes(ctx context.Context, attributes map[string]string) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	attrs := make([]attribute.KeyValue, 0, len(attributes))
	for k, v := range attributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	span.SetAttributes(attrs...)
}

// Shutdown gracefully shuts down the tracing provider
func (t *TracingProvider) Shutdown(ctx context.Context) error {
	// Shutdown the provider to ensure all spans are exported
	err := t.provider.Shutdown(ctx)
	if err != nil {
		return err
	}

	// Shutdown the exporter if it exists
	if t.exporter != nil {
		return t.exporter.Shutdown(ctx)
	}

	return nil
}

// GetTracer returns the underlying tracer
func (t *TracingProvider) GetTracer() trace.Tracer {
	return t.tracer
}

// IsRecording returns true if the span in the context is recording
func (t *TracingProvider) IsRecording(ctx context.Context) bool {
	span := trace.SpanFromContext(ctx)
	return span.IsRecording()
}

// TraceWithAttributes starts a new span with custom attributes
func (t *TracingProvider) TraceWithAttributes(ctx context.Context, name string, attrs map[string]string) (context.Context, func()) {
	attributes := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		attributes = append(attributes, attribute.String(k, v))
	}

	ctx, span := t.tracer.Start(ctx, name,
		trace.WithAttributes(attributes...),
	)

	return ctx, func() {
		span.End()
	}
}

// ExtractTraceID extracts the trace ID from the current context
func (t *TracingProvider) ExtractTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// ExtractSpanID extracts the span ID from the current context
func (t *TracingProvider) ExtractSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}
