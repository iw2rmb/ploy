package contracts

import "strings"

// RecoveryLoopKind identifies the universal recovery loop type.
type RecoveryLoopKind string

const (
	RecoveryLoopKindHealing RecoveryLoopKind = "healing"
)

func (k RecoveryLoopKind) String() string { return string(k) }

// RecoveryErrorKind identifies router-classified failure domains for healing selection.
type RecoveryErrorKind string

const (
	RecoveryErrorKindInfra   RecoveryErrorKind = "infra"
	RecoveryErrorKindCode    RecoveryErrorKind = "code"
	RecoveryErrorKindDeps    RecoveryErrorKind = "deps"
	RecoveryErrorKindMixed   RecoveryErrorKind = "mixed"
	RecoveryErrorKindUnknown RecoveryErrorKind = "unknown"
)

func (k RecoveryErrorKind) String() string { return string(k) }

// DefaultRecoveryLoopKind returns the canonical loop kind used when metadata is missing.
func DefaultRecoveryLoopKind() RecoveryLoopKind {
	return RecoveryLoopKindHealing
}

// DefaultRecoveryErrorKind returns the canonical error kind used when classification is missing.
func DefaultRecoveryErrorKind() RecoveryErrorKind {
	return RecoveryErrorKindUnknown
}

// ParseRecoveryLoopKind parses and normalizes a recovery loop kind.
func ParseRecoveryLoopKind(raw string) (RecoveryLoopKind, bool) {
	switch strings.TrimSpace(raw) {
	case RecoveryLoopKindHealing.String():
		return RecoveryLoopKindHealing, true
	default:
		return "", false
	}
}

// ParseRecoveryErrorKind parses and normalizes a recovery error kind.
func ParseRecoveryErrorKind(raw string) (RecoveryErrorKind, bool) {
	switch strings.TrimSpace(raw) {
	case RecoveryErrorKindInfra.String():
		return RecoveryErrorKindInfra, true
	case RecoveryErrorKindCode.String():
		return RecoveryErrorKindCode, true
	case RecoveryErrorKindDeps.String():
		return RecoveryErrorKindDeps, true
	case RecoveryErrorKindMixed.String():
		return RecoveryErrorKindMixed, true
	case RecoveryErrorKindUnknown.String():
		return RecoveryErrorKindUnknown, true
	default:
		return "", false
	}
}

// IsTerminalRecoveryErrorKind reports whether a classification should terminate healing insertion.
func IsTerminalRecoveryErrorKind(kind RecoveryErrorKind) bool {
	return kind == RecoveryErrorKindMixed || kind == RecoveryErrorKindUnknown
}

// IsInfraRecoveryErrorKind reports whether a classification is infra.
func IsInfraRecoveryErrorKind(kind RecoveryErrorKind) bool {
	return kind == RecoveryErrorKindInfra
}

// RecoveryErrorKinds returns canonical recovery error kind values.
func RecoveryErrorKinds() []RecoveryErrorKind {
	return []RecoveryErrorKind{
		RecoveryErrorKindInfra,
		RecoveryErrorKindCode,
		RecoveryErrorKindDeps,
		RecoveryErrorKindMixed,
		RecoveryErrorKindUnknown,
	}
}
