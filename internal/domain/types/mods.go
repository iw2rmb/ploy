package types

import (
	"fmt"
	"strings"
)

// ModType identifies the job phase in the Mods pipeline.
//
// Known values:
//   - ModTypePreGate: pre-mod Build Gate
//   - ModTypeMod: main mod execution
//   - ModTypePostGate: post-mod Build Gate
//   - ModTypeHeal: healing after gate failure
//   - ModTypeReGate: re-run Build Gate after healing
//   - ModTypeMR: post-run MR creation job
//
// Unknown or empty values should be treated carefully at boundaries; use
// ModType.IsZero/Validate to enforce invariants when appropriate.
type ModType string

const (
	ModTypePreGate  ModType = "pre_gate"
	ModTypeMod      ModType = "mod"
	ModTypePostGate ModType = "post_gate"
	ModTypeHeal     ModType = "heal"
	ModTypeReGate   ModType = "re_gate"
	ModTypeMR       ModType = "mr"
)

// String returns the underlying string value.
func (v ModType) String() string { return string(v) }

// IsZero reports whether the value is empty (after trimming spaces).
func (v ModType) IsZero() bool { return IsEmpty(string(v)) }

// Validate ensures the value is one of the known ModType constants.
func (v ModType) Validate() error {
	s := strings.TrimSpace(string(v))
	switch ModType(s) {
	case ModTypePreGate, ModTypeMod, ModTypePostGate, ModTypeHeal, ModTypeReGate, ModTypeMR:
		return nil
	default:
		return fmt.Errorf("invalid mod_type %q", s)
	}
}
