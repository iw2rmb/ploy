// stack_gate_spec.go defines Stack Gate types for explicit stack expectations.
//
// Stack Gate allows Mods specs to declare explicit expectations about the
// repository's technology stack (language, build tool, release version).
// This enables:
//   - Validation of contradictory multi-step runs before execution
//   - Stack-based image selection for Mods containers
//   - Chain validation across step boundaries (outbound → inbound)
//
// ## Wire Format
//
// Stack Gate configuration appears in the steps[] array:
//
//	steps:
//	  - name: java11-to-17
//	    image: docker.io/user/migs-orw:latest
//	    stack:
//	      inbound:
//	        enabled: true
//	        expect: { language: java, tool: maven, release: "11" }
//	      outbound:
//	        enabled: true
//	        expect: { language: java, tool: maven, release: "17" }
package contracts

import "encoding/json"

// StackExpectation describes expected stack characteristics.
// All fields are optional; omitted fields indicate "any" for that dimension.
type StackExpectation struct {
	// Language is the expected programming language (e.g., "java", "go", "python").
	Language string `json:"language,omitempty" yaml:"language,omitempty"`

	// Tool is the expected build tool (e.g., "maven", "gradle", "npm").
	Tool string `json:"tool,omitempty" yaml:"tool,omitempty"`

	// Release is the expected version/release (e.g., "11", "17", "3.9").
	// Stored as string to handle both integer and string release values in YAML.
	Release string `json:"release,omitempty" yaml:"release,omitempty"`
}

// IsEmpty returns true if no expectation fields are set.
func (e StackExpectation) IsEmpty() bool {
	return e.Language == "" && e.Tool == "" && e.Release == ""
}

// Equal returns true if both expectations have identical field values.
func (e StackExpectation) Equal(other StackExpectation) bool {
	return e.Language == other.Language &&
		e.Tool == other.Tool &&
		e.Release == other.Release
}

// StackGatePhaseSpec configures a single phase (inbound or outbound) of Stack Gate.
type StackGatePhaseSpec struct {
	// Enabled controls whether this phase is active.
	// When false, the phase is skipped entirely.
	Enabled bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Expect defines the stack expectations for this phase.
	// Only validated when Enabled is true.
	Expect *StackExpectation `json:"expect,omitempty" yaml:"expect,omitempty"`
}

// IsEmpty returns true if the phase spec has default values (disabled, no expect).
func (p StackGatePhaseSpec) IsEmpty() bool {
	return !p.Enabled && (p.Expect == nil || p.Expect.IsEmpty())
}

// Equal returns true if both phase specs have identical configuration.
func (p StackGatePhaseSpec) Equal(other StackGatePhaseSpec) bool {
	if p.Enabled != other.Enabled {
		return false
	}
	if p.Expect == nil && other.Expect == nil {
		return true
	}
	if p.Expect == nil || other.Expect == nil {
		return false
	}
	return p.Expect.Equal(*other.Expect)
}

// StackGateSpec configures Stack Gate for a mig step.
// Inbound validates pre-mig expectations; Outbound validates post-mig expectations.
type StackGateSpec struct {
	// Inbound configures pre-mig stack validation.
	// Validates that the repository matches expectations before the mig runs.
	Inbound *StackGatePhaseSpec `json:"inbound,omitempty" yaml:"inbound,omitempty"`

	// Outbound configures post-mig stack validation.
	// Validates that the mig produced the expected stack transformation.
	Outbound *StackGatePhaseSpec `json:"outbound,omitempty" yaml:"outbound,omitempty"`
}

// MarshalJSON returns nil for empty StackGateSpec so that parent structs with
// omitempty will omit the field entirely.
func (s StackGateSpec) MarshalJSON() ([]byte, error) {
	if s.IsEmpty() {
		return []byte("null"), nil
	}
	type alias StackGateSpec
	return json.Marshal(alias(s))
}

// IsEmpty returns true if no stack gate configuration is present.
func (s StackGateSpec) IsEmpty() bool {
	return (s.Inbound == nil || s.Inbound.IsEmpty()) &&
		(s.Outbound == nil || s.Outbound.IsEmpty())
}

// Equal returns true if both stack gate specs have identical configuration.
func (s StackGateSpec) Equal(other StackGateSpec) bool {
	// Compare Inbound.
	switch {
	case s.Inbound == nil && other.Inbound == nil:
		// Both nil, continue.
	case s.Inbound == nil || other.Inbound == nil:
		return false
	default:
		if !s.Inbound.Equal(*other.Inbound) {
			return false
		}
	}

	// Compare Outbound.
	if s.Outbound == nil && other.Outbound == nil {
		return true
	}
	if s.Outbound == nil || other.Outbound == nil {
		return false
	}
	return s.Outbound.Equal(*other.Outbound)
}
