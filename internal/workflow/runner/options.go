package runner

import "github.com/iw2rmb/ploy/internal/workflow/aster"

// Options configures the workflow runner execution.
type Options struct {
	Ticket            string
	Events            EventsClient
	Runtime           RuntimeClient
	Planner           Planner
	WorkspaceRoot     string
	MaxStageRetries   int
	ManifestCompiler  ManifestCompiler
	Aster             AsterOptions
	CacheComposer     CacheComposer
	JobComposer       JobComposer
	Mods              ModsOptions
	WorkspacePreparer WorkspacePreparer
}

// AsterOptions configures Aster bundle lookup for stage execution.
type AsterOptions struct {
	Enabled           bool
	Locator           aster.Locator
	AdditionalToggles []string
	StageOverrides    map[string]AsterStageOverride
}

// AsterStageOverride tailors Aster behaviour for a specific stage.
type AsterStageOverride struct {
	Disable      bool
	ExtraToggles []string
}
