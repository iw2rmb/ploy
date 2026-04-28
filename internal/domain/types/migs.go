package types

import (
	"fmt"
	"strings"
)

// JobType identifies the job phase in the Migs pipeline.
//
// Known values:
//   - JobTypePreGate: pre-mig Build Gate
//   - JobTypeMig: main mig execution
//   - JobTypePostGate: post-mig Build Gate
//
// Unknown or empty values should be treated carefully at boundaries; use
// JobType.IsZero/Validate to enforce invariants when appropriate.
type JobType string

const (
	JobTypePreGate  JobType = "pre_gate"
	JobTypeMig      JobType = "mig"
	JobTypePostGate JobType = "post_gate"
)

// String returns the underlying string value.
func (v JobType) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v JobType) IsZero() bool { return IsEmpty(string(v)) }

// Validate ensures the value is one of the known JobType constants.
func (v JobType) Validate() error {
	s := strings.TrimSpace(string(v))
	switch JobType(s) {
	case JobTypePreGate, JobTypeMig, JobTypePostGate:
		return nil
	default:
		return fmt.Errorf("invalid job_type %q", s)
	}
}
