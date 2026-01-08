package types

import (
	"fmt"
	"strings"
)

// SSEEventType identifies the type of an SSE event in the streaming system.
//
// Known values form a closed allow-list:
//   - SSEEventLog: structured log record
//   - SSEEventRetention: retention hint metadata
//   - SSEEventRun: run summary snapshot
//   - SSEEventStage: stage status update
//   - SSEEventDone: terminal event signaling stream completion
//
// Unknown or empty values are rejected at publish time; use Validate() to
// enforce invariants at boundaries.
type SSEEventType string

const (
	SSEEventLog       SSEEventType = "log"
	SSEEventRetention SSEEventType = "retention"
	SSEEventRun       SSEEventType = "run"
	SSEEventStage     SSEEventType = "stage"
	SSEEventDone      SSEEventType = "done"
)

// String returns the underlying string value.
func (v SSEEventType) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v SSEEventType) IsZero() bool { return IsEmpty(string(v)) }

// Validate ensures the value is one of the known SSEEventType constants.
func (v SSEEventType) Validate() error {
	s := strings.TrimSpace(string(v))
	switch SSEEventType(s) {
	case SSEEventLog, SSEEventRetention, SSEEventRun, SSEEventStage, SSEEventDone:
		return nil
	default:
		return fmt.Errorf("invalid SSE event type %q", s)
	}
}
