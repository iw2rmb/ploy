package types

import (
	"fmt"
	"strings"
)

// GlobalEnvScope identifies the injection scope for a global environment variable.
// It determines which job types receive the env var based on their phase in the pipeline.
//
// Known values:
//   - GlobalEnvScopeAll: Inject into every job type
//   - GlobalEnvScopeMods: Inject into mod and post_gate jobs (code modification phases)
//   - GlobalEnvScopeHeal: Inject into heal and re_gate jobs (healing/retry phases)
//   - GlobalEnvScopeGate: Inject into pre_gate, re_gate, and post_gate jobs (gate execution phases)
//
// Unknown or empty values should be rejected at API boundaries using Validate().
type GlobalEnvScope string

const (
	// GlobalEnvScopeAll injects into every job type.
	GlobalEnvScopeAll GlobalEnvScope = "all"
	// GlobalEnvScopeMods injects into mod and post_gate jobs (modification phases).
	GlobalEnvScopeMods GlobalEnvScope = "mods"
	// GlobalEnvScopeHeal injects into heal and re_gate jobs (healing phases).
	GlobalEnvScopeHeal GlobalEnvScope = "heal"
	// GlobalEnvScopeGate injects into pre_gate, re_gate, and post_gate jobs (gate phases).
	GlobalEnvScopeGate GlobalEnvScope = "gate"
)

// String returns the underlying string value.
func (s GlobalEnvScope) String() string { return string(s) }

// IsZero reports whether the value is empty (after trimming spaces).
func (s GlobalEnvScope) IsZero() bool { return IsEmpty(string(s)) }

// Validate ensures the value is one of the known GlobalEnvScope constants.
// Returns an error for unknown or empty values.
func (s GlobalEnvScope) Validate() error {
	normalized := GlobalEnvScope(strings.TrimSpace(string(s)))
	switch normalized {
	case GlobalEnvScopeAll, GlobalEnvScopeMods, GlobalEnvScopeHeal, GlobalEnvScopeGate:
		return nil
	default:
		if normalized == "" {
			return fmt.Errorf("scope is required")
		}
		return fmt.Errorf("invalid scope %q (must be one of: all, mods, heal, gate)", s)
	}
}

// ParseGlobalEnvScope parses a string into a GlobalEnvScope, returning an error
// if the value is not one of the known constants. Empty strings default to "all".
func ParseGlobalEnvScope(s string) (GlobalEnvScope, error) {
	normalized := strings.TrimSpace(s)
	if normalized == "" {
		return GlobalEnvScopeAll, nil // Default to "all" if not specified.
	}
	scope := GlobalEnvScope(normalized)
	if err := scope.Validate(); err != nil {
		return "", err
	}
	return scope, nil
}

// MatchesModType determines whether this scope applies to the given job type (ModType).
// This is the core scope-matching logic for global env var injection.
//
// Scope semantics:
//   - "all"  → inject into every job type
//   - "mods" → inject into mod and post_gate jobs (code modification phases)
//   - "heal" → inject into heal and re_gate jobs (healing/retry phases)
//   - "gate" → inject into pre_gate, re_gate, and post_gate jobs (gate execution phases)
func (s GlobalEnvScope) MatchesModType(modType ModType) bool {
	switch s {
	case GlobalEnvScopeAll:
		return true
	case GlobalEnvScopeMods:
		// Mods scope applies to mod and post_gate jobs (modification phases).
		return modType == ModTypeMod || modType == ModTypePostGate
	case GlobalEnvScopeHeal:
		// Heal scope applies to heal and re_gate jobs (healing phases).
		return modType == ModTypeHeal || modType == ModTypeReGate
	case GlobalEnvScopeGate:
		// Gate scope applies to all gate-related jobs.
		return modType == ModTypePreGate || modType == ModTypeReGate || modType == ModTypePostGate
	default:
		return false
	}
}
