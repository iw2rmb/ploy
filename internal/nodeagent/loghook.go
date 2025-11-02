package nodeagent

// LogHook defines an interface for processing log data before it is
// compressed and sent to the server. Implementations may scrub PII,
// redact secrets, or perform other transformations.
//
// Hooks are invoked synchronously in the log path; they should be fast
// and avoid blocking operations. Heavy transformations should be done
// in a streaming fashion to avoid buffering large amounts of data.
type LogHook interface {
	// Process transforms the input log data and returns the processed result.
	// The input slice p must not be modified; implementations should allocate
	// a new buffer if transformations are needed.
	//
	// Process must be safe for concurrent calls from multiple goroutines.
	Process(p []byte) ([]byte, error)
}

// NoOpLogHook is a placeholder hook that returns input unchanged.
// This is the default hook when no PII scrubbing is configured.
type NoOpLogHook struct{}

// Process returns the input unchanged.
func (h *NoOpLogHook) Process(p []byte) ([]byte, error) {
	return p, nil
}
