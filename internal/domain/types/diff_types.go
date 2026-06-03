package types

import "fmt"

// DiffJobType identifies the diff producer kind stored in diff summaries.
type DiffJobType string

const (
	DiffJobTypeMig DiffJobType = "mig"
)

func (t DiffJobType) String() string { return string(t) }

func (t DiffJobType) Validate() error {
	switch t {
	case DiffJobTypeMig:
		return nil
	default:
		return fmt.Errorf("invalid diff job type %q", t)
	}
}
