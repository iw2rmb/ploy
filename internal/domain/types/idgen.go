// Package types provides ID generation helpers for KSUID and NanoID-based identifiers.
// This file centralizes ID generation so call sites do not embed library calls directly.
package types

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/segmentio/ksuid"
)

// alphabet defines the URL-safe character set for NanoID generation.
// This uses the standard NanoID URL-safe alphabet (A-Za-z0-9_-) which provides
// good entropy while remaining safe for use in URLs and file paths.
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_-"

// NewRunID generates a new unique RunID using KSUID.
// KSUID provides time-sortable, globally unique identifiers (27 characters).
// The time-ordering property allows efficient database indexing and querying
// by creation time without a separate timestamp column.
func NewRunID() RunID {
	return RunID(ksuid.New().String())
}

// NewJobID generates a new unique JobID using KSUID.
// Jobs are the unit of work assignment to nodes, and KSUID provides
// time-sortable identifiers that allow efficient queries by creation time.
func NewJobID() JobID {
	return JobID(ksuid.New().String())
}

// NewNodeKey generates a new unique node identifier using NanoID.
// Uses a 6-character NanoID with the URL-safe alphabet for compact identifiers
// suitable for node IDs in nodes.id and node agent configuration.
// The 6-character length balances brevity with sufficient uniqueness for
// typical cluster sizes.
func NewNodeKey() string {
	// Generate returns an error only if the alphabet is invalid or length is <= 0.
	// Since we use a fixed valid alphabet and length, error is effectively impossible.
	id, err := gonanoid.Generate(alphabet, 6)
	if err != nil {
		// Panic on configuration error; this should never happen with valid inputs.
		panic("idgen: failed to generate NanoID: " + err.Error())
	}
	return id
}

// NewMigID generates a new unique MigID using NanoID.
// Uses a 6-character NanoID with the URL-safe alphabet.
// The 6-character length provides sufficient entropy for mig project identifiers
// while remaining compact for CLI usage and display.
func NewMigID() MigID {
	// Generate returns an error only if the alphabet is invalid or length is <= 0.
	// Since we use a fixed valid alphabet and length, error is effectively impossible.
	id, err := gonanoid.Generate(alphabet, 6)
	if err != nil {
		// Panic on configuration error; this should never happen with valid inputs.
		panic("idgen: failed to generate NanoID: " + err.Error())
	}
	return MigID(id)
}

// NewSpecID generates a new unique SpecID using NanoID.
// Uses an 8-character NanoID with the URL-safe alphabet.
// The 8-character length provides sufficient entropy for spec identifiers
// in the append-only specs table.
func NewSpecID() SpecID {
	// Generate returns an error only if the alphabet is invalid or length is <= 0.
	// Since we use a fixed valid alphabet and length, error is effectively impossible.
	id, err := gonanoid.Generate(alphabet, 8)
	if err != nil {
		// Panic on configuration error; this should never happen with valid inputs.
		panic("idgen: failed to generate NanoID: " + err.Error())
	}
	return SpecID(id)
}

// NewMigRepoID generates a new unique MigRepoID using NanoID.
// Uses an 8-character NanoID with the URL-safe alphabet.
// The 8-character length provides sufficient entropy for per-mig repo identifiers.
// Note: This type may also be referred to as "repo_id" in API contexts.
func NewMigRepoID() MigRepoID {
	// Generate returns an error only if the alphabet is invalid or length is <= 0.
	// Since we use a fixed valid alphabet and length, error is effectively impossible.
	id, err := gonanoid.Generate(alphabet, 8)
	if err != nil {
		// Panic on configuration error; this should never happen with valid inputs.
		panic("idgen: failed to generate NanoID: " + err.Error())
	}
	return MigRepoID(id)
}

// NewRepoID generates a new unique global RepoID using NanoID.
// Uses an 8-character NanoID with the URL-safe alphabet.
func NewRepoID() RepoID {
	id, err := gonanoid.Generate(alphabet, 8)
	if err != nil {
		panic("idgen: failed to generate NanoID: " + err.Error())
	}
	return RepoID(id)
}

// NewSpecBundleID generates a new unique SpecBundleID using NanoID.
// Uses an 8-character NanoID with the URL-safe alphabet.
// The returned value becomes the bundle_id in TmpBundleRef once stored.
func NewSpecBundleID() SpecBundleID {
	id, err := gonanoid.Generate(alphabet, 8)
	if err != nil {
		panic("idgen: failed to generate NanoID: " + err.Error())
	}
	return SpecBundleID(id)
}
