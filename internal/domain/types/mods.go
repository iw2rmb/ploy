package types

import (
	"fmt"
	"strings"
)

// JobType identifies the job phase in the Mods pipeline.
//
// Known values:
//   - JobTypePreGate: pre-mod Build Gate
//   - JobTypeMod: main mod execution
//   - JobTypePostGate: post-mod Build Gate
//   - JobTypeHeal: healing after gate failure
//   - JobTypeReGate: re-run Build Gate after healing
//   - JobTypeMR: post-run MR creation job
//
// Unknown or empty values should be treated carefully at boundaries; use
// JobType.IsZero/Validate to enforce invariants when appropriate.
type JobType string

const (
	JobTypePreGate  JobType = "pre_gate"
	JobTypeMod      JobType = "mod"
	JobTypePostGate JobType = "post_gate"
	JobTypeHeal     JobType = "heal"
	JobTypeReGate   JobType = "re_gate"
	JobTypeMR       JobType = "mr"
)

// String returns the underlying string value.
func (v JobType) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v JobType) IsZero() bool { return IsEmpty(string(v)) }

// Validate ensures the value is one of the known JobType constants.
func (v JobType) Validate() error {
	s := strings.TrimSpace(string(v))
	switch JobType(s) {
	case JobTypePreGate, JobTypeMod, JobTypePostGate, JobTypeHeal, JobTypeReGate, JobTypeMR:
		return nil
	default:
		return fmt.Errorf("invalid job_type %q", s)
	}
}
