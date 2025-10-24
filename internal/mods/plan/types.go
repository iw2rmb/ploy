package plan

const (
	// StageNamePlan identifies the Mods planning stage.
	StageNamePlan = "mods-plan"
	// StageNameORWApply identifies the OpenRewrite apply stage.
	StageNameORWApply = "orw-apply"
	// StageNameORWGenerate identifies the OpenRewrite generate stage.
	StageNameORWGenerate = "orw-gen"
	// StageNameLLMPlan identifies the LLM planning stage.
	StageNameLLMPlan = "llm-plan"
	// StageNameLLMExec identifies the LLM execution stage.
	StageNameLLMExec = "llm-exec"
	// StageNameHuman identifies the human review stage.
	StageNameHuman = "mods-human"

	// StageKindPlan mirrors StageNamePlan for consumers expecting the kind value.
	StageKindPlan = StageNamePlan
	// StageKindORWApply mirrors StageNameORWApply for kind classification.
	StageKindORWApply = StageNameORWApply
	// StageKindORWGenerate mirrors StageNameORWGenerate for kind classification.
	StageKindORWGenerate = StageNameORWGenerate
	// StageKindLLMPlan mirrors StageNameLLMPlan for kind classification.
	StageKindLLMPlan = StageNameLLMPlan
	// StageKindLLMExec mirrors StageNameLLMExec for kind classification.
	StageKindLLMExec = StageNameLLMExec
	// StageKindHuman mirrors StageNameHuman for kind classification.
	StageKindHuman = StageNameHuman

	defaultPlanLane        = "node-wasm"
	defaultOpenRewriteLane = "node-wasm"
	defaultLLMPlanLane     = "gpu-ml"
	defaultLLMExecLane     = "gpu-ml"
	defaultHumanLane       = "mods-human"
)

// Stage models a Mods workflow stage produced by the planner.
type Stage struct {
	Name         string
	Kind         string
	Lane         string
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

// StageModsPlan describes the Mods planner output that Grid consumers rely on.
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
