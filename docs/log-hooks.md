# Log Processing Hooks

## Overview

The node agent supports pluggable log processing hooks that transform log data before compression and transmission to the server. The primary use case is scrubbing personally identifiable information (PII) or secrets from logs.

## Architecture

### Interface

```go
type LogHook interface {
    // Process transforms the input log data and returns the processed result.
    // The input slice p must not be modified; implementations should allocate
    // a new buffer if transformations are needed.
    //
    // Process must be safe for concurrent calls from multiple goroutines.
    Process(p []byte) ([]byte, error)
}
```

### Integration Point

Hooks are invoked in `LogStreamer.Write()` before data is passed to the gzip compressor. The data flow is:

1. Application writes to `LogStreamer` (implements `io.Writer`)
2. **Hook processes the data** ← insertion point
3. Processed data is gzipped
4. Gzipped chunks are sent to server when size threshold or time interval is reached

### Default Behavior

By default, `LogStreamer` uses `NoOpLogHook`, a placeholder that returns input unchanged. This ensures zero performance impact when PII scrubbing is not configured.

```go
type NoOpLogHook struct{}

func (h *NoOpLogHook) Process(p []byte) ([]byte, error) {
    return p, nil
}
```

### Error Handling

If a hook returns an error, the log streamer:
1. Logs a warning with `slog.Warn`
2. Falls back to using the original (unprocessed) data
3. Continues operation (non-fatal)

This design ensures that hook failures do not block log delivery, which is critical for observability.

Additionally, if a hook returns a `nil` slice without an error, the streamer defensively
falls back to the original input to preserve `io.Writer` semantics (the write still
counts as having consumed `len(p)` bytes).

## Usage

### Setting a Custom Hook

Hooks must be set before any writes occur (not safe for concurrent use with `Write`):

```go
logStreamer := NewLogStreamer(cfg, runID, stageID)

// Install custom hook before writes begin
customHook := &MyPIIScrubber{}
logStreamer.SetHook(customHook)

// Now use logStreamer as io.Writer
fmt.Fprintf(logStreamer, "User logged in: %s\n", username)
```

### Example: Simple Redaction Hook

```go
type SimpleRedactor struct{}

func (r *SimpleRedactor) Process(p []byte) ([]byte, error) {
    // Example: redact email addresses with regex
    emailRegex := regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)

    result := emailRegex.ReplaceAll(p, []byte("[EMAIL_REDACTED]"))
    return result, nil
}
```

## Performance Considerations

### Synchronous Execution

Hooks are invoked **synchronously** in the write path. They should be:
- **Fast**: Avoid I/O, locks, or heavy computation
- **Streaming**: Process data in-place; avoid buffering entire payloads
- **Non-blocking**: No network calls or file operations

### Concurrency Safety

Hook implementations **must be thread-safe**. `LogStreamer.Write()` may be called from multiple goroutines, although the current implementation serializes writes with a mutex.

### Memory Allocation

Hooks that return unmodified input can return the original slice `p` to avoid allocations. Hooks that transform data should allocate a new buffer:

```go
func (h *MyHook) Process(p []byte) ([]byte, error) {
    if !h.needsTransform(p) {
        return p, nil // Zero allocation fast path
    }

    result := make([]byte, 0, len(p))
    // ... transform into result ...
    return result, nil
}
```

## Future Enhancements

This is a **placeholder implementation** (no-op by default). Future work may include:

1. **Built-in PII scrubbers**: Email, phone, SSN, credit card regex patterns
2. **Configuration**: Load hook settings from config file or environment
3. **Composition**: Chain multiple hooks (e.g., secrets + PII)
4. **Metrics**: Track hook latency and error rates
5. **Testing utilities**: Assertion helpers for validating scrubbing behavior

## Testing

See `internal/nodeagent/loghook_test.go` for examples of testing hook implementations and integration with `LogStreamer`.

## References

- `internal/nodeagent/loghook.go`: Interface and `NoOpLogHook`
- `internal/nodeagent/logstreamer.go`: Integration in `Write()` method
- ROADMAP.md line 107: "Scrub PII from logs via node-side hooks"
