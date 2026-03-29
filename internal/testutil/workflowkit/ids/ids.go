// Package ids provides cycle-safe domain key types for cross-module
// orchestration scenario assembly. It imports only internal/domain/types so
// both store-layer tests and the workflowkit package itself can depend on it
// without creating an import cycle.
package ids

import domaintypes "github.com/iw2rmb/ploy/internal/domain/types"

// AttemptKey identifies a unique run-repo-attempt combination used as a map
// key in recovery orchestration scenarios.
type AttemptKey struct {
	RunID   domaintypes.RunID
	RepoID  domaintypes.RepoID
	Attempt int32
}
