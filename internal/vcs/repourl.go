// Package vcs provides shared utilities for version control system operations.
// This package consolidates repository URL handling logic used across the codebase.
package vcs

import "strings"

// NormalizeRepoURL normalizes a git repository URL for comparison and matching.
//
// The normalization applies the following transformations:
//   - Trims leading and trailing whitespace
//   - Removes trailing "/" (trailing slash)
//   - Removes trailing ".git" suffix
//
// This produces a stable, canonical form for comparing repository URLs that may
// differ in superficial ways. For example, the following URLs all normalize to
// "https://github.com/org/repo":
//
//   - "https://github.com/org/repo"
//   - "https://github.com/org/repo/"
//   - "https://github.com/org/repo.git"
//   - "https://github.com/org/repo.git/"
//   - "  https://github.com/org/repo  "
//
// Use this helper for:
//   - Cache key generation (internal/worker/hydration)
//   - Repo URL matching in server handlers
//   - CLI repo URL resolution (cmd/ploy)
func NormalizeRepoURL(raw string) string {
	// Step 1: Trim leading/trailing whitespace.
	normalized := strings.TrimSpace(raw)

	// Step 2: Remove trailing slash if present.
	// This handles URLs ending with "/" (e.g., "https://github.com/org/repo/").
	normalized = strings.TrimSuffix(normalized, "/")

	// Step 3: Remove trailing ".git" suffix if present.
	// This handles URLs ending with ".git" (e.g., "https://github.com/org/repo.git").
	normalized = strings.TrimSuffix(normalized, ".git")

	return normalized
}
