package contracts

// HookRuntimeDecision carries per-hook execution decisions resolved at claim time.
// It is consumed by nodeagent hook runtime and mirrored in completion metadata.
type HookRuntimeDecision struct {
	HookHash           string `json:"hook_hash,omitempty"`
	HookShouldRun      bool   `json:"hook_should_run"`
	HookOnceSkipMarked bool   `json:"hook_once_skip_marked"`
	MatchedPredicate   string `json:"matched_predicate,omitempty"`
	MatchedPackage     string `json:"matched_package,omitempty"`
	PreviousVersion    string `json:"previous_version,omitempty"`
	CurrentVersion     string `json:"current_version,omitempty"`
}
