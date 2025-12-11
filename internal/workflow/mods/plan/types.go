package plan

const (
	// StageNamePlan identifies the Mods planning stage.
	StageNamePlan = "mods-plan"
	// StageNameORWApply identifies the OpenRewrite apply stage.
	StageNameORWApply = "orw-apply"
	// StageNameORWGenerate identifies the OpenRewrite generate stage.
	StageNameORWGenerate = "orw-gen"
	// StageNameHuman identifies the human review stage.
	StageNameHuman = "mods-human"

	// StageKindPlan mirrors StageNamePlan for consumers expecting the kind value.
	StageKindPlan = StageNamePlan
	// StageKindORWApply mirrors StageNameORWApply for kind classification.
	StageKindORWApply = StageNameORWApply
	// StageKindORWGenerate mirrors StageNameORWGenerate for kind classification.
	StageKindORWGenerate = StageNameORWGenerate
	// StageKindHuman mirrors StageNameHuman for kind classification.
	StageKindHuman = StageNameHuman
)

// Stage models a Mods workflow stage produced by the planner.
type Stage struct {
	Name         string
	Kind         string
	Dependencies []string
	Metadata     StageMetadata
}

// StageMetadata holds Mods-specific metadata for a workflow stage.
type StageMetadata struct {
	Mods *StageModsMetadata
}

// StageModsMetadata captures Mods plan, human, and recommendation payloads.
type StageModsMetadata struct {
	Plan            *StageModsPlan
	Human           *StageModsHuman
	Recommendations []StageModsRecommendation
}

// StageModsPlan describes the Mods planner output that downstream consumers rely on.
type StageModsPlan struct {
	SelectedRecipes []string
	ParallelStages  []string
	HumanGate       bool
	Summary         string
	PlanTimeout     string
	MaxParallel     int
}

// StageModsHuman outlines expectations for the human-in-the-loop checkpoint.
type StageModsHuman struct {
	Required  bool
	Playbooks []string
}

// StageModsRecommendation records individual recommendations surfaced by the Mods advisor.
type StageModsRecommendation struct {
	Source     string
	Message    string
	Confidence float64
}
