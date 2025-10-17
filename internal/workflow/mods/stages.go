package mods

const (
	StageNamePlan        = "mods-plan"
	StageNameORWApply    = "orw-apply"
	StageNameORWGenerate = "orw-gen"
	StageNameLLMPlan     = "llm-plan"
	StageNameLLMExec     = "llm-exec"
	StageNameHuman       = "mods-human"

	StageKindPlan        = StageNamePlan
	StageKindORWApply    = StageNameORWApply
	StageKindORWGenerate = StageNameORWGenerate
	StageKindLLMPlan     = StageNameLLMPlan
	StageKindLLMExec     = StageNameLLMExec
	StageKindHuman       = StageNameHuman

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

// StageModsRecommendation records individual recommendations surfaced by the
// Mods advisor.
type StageModsRecommendation struct {
	Source     string
	Message    string
	Confidence float64
}
