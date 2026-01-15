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

// NormalizeRepoURLSchemless returns a scheme-less, display-oriented form of a repository URL.
//
// It is intended for human-facing CLI output (stdout/stderr). It is NOT a wire format,
// and should not be used for API requests or identity comparisons.
//
// Behavior:
//   - Applies NormalizeRepoURL first (trim, strip trailing "/", strip trailing ".git")
//   - Removes leading scheme prefixes: https://, ssh://, file://
//   - For ssh:// URLs, drops any leading "user@" portion
//   - For SCP-style SSH URLs (git@host:org/repo), drops leading "user@" and converts ":" to "/" when it is not a numeric port
//
// Examples:
//   - https://github.com/org/repo.git      -> github.com/org/repo
//   - ssh://git@github.com/org/repo.git   -> github.com/org/repo
//   - git@github.com:org/repo.git         -> github.com/org/repo
//   - file:///path/to/repo.git            -> /path/to/repo
func NormalizeRepoURLSchemless(raw string) string {
	normalized := NormalizeRepoURL(raw)
	if normalized == "" {
		return ""
	}

	lower := strings.ToLower(normalized)

	switch {
	case strings.HasPrefix(lower, "https://"):
		normalized = normalized[len("https://"):]
	case strings.HasPrefix(lower, "ssh://"):
		normalized = normalized[len("ssh://"):]
	case strings.HasPrefix(lower, "file://"):
		return normalized[len("file://"):]
	}

	// Drop leading user@ if present (e.g., git@github.com/..., git@github.com:...).
	if at := strings.IndexByte(normalized, '@'); at >= 0 {
		normalized = normalized[at+1:]
	}

	// Convert SCP-style host:path to host/path (only when ":" is not a numeric port).
	colon := strings.IndexByte(normalized, ':')
	if colon >= 0 {
		slash := strings.IndexByte(normalized, '/')
		if slash == -1 || colon < slash {
			// Treat as SCP-style unless the ":" introduces a numeric port.
			if colon+1 < len(normalized) && (normalized[colon+1] < '0' || normalized[colon+1] > '9') {
				normalized = normalized[:colon] + "/" + normalized[colon+1:]
			}
		}
	}

	return normalized
}
