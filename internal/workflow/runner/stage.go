package runner

import (
	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

type StageKind string

const (
	StageKindModsPlan        StageKind = StageKind(mods.StageKindPlan)
	StageKindModsORWApply    StageKind = StageKind(mods.StageKindORWApply)
	StageKindModsORWGenerate StageKind = StageKind(mods.StageKindORWGenerate)
	StageKindModsLLMPlan     StageKind = StageKind(mods.StageKindLLMPlan)
	StageKindModsLLMExec     StageKind = StageKind(mods.StageKindLLMExec)
	StageKindModsHuman       StageKind = StageKind(mods.StageKindHuman)
	StageKindBuildGate       StageKind = "build-gate"
	StageKindStaticChecks    StageKind = "static-checks"
	StageKindTest            StageKind = "test"
)

type Stage struct {
	Name         string
	Kind         StageKind
	Lane         string
	Dependencies []string
	Constraints  StageConstraints
	Aster        StageAster
	Metadata     StageMetadata
	CacheKey     string
	Job          StageJobSpec
}

// StageMetadata captures stage-specific metadata for checkpoints.
type StageMetadata struct {
	Mods      *StageModsMetadata
	BuildGate *StageBuildGateMetadata
}

// StageBuildGateMetadata records build gate metadata attached to checkpoints.
type StageBuildGateMetadata struct {
	LogDigest    string
	StaticChecks []StageStaticCheck
	LogFindings  []StageLogFinding
}

// StageStaticCheck describes one static analysis tool invocation.
type StageStaticCheck struct {
	Language string
	Tool     string
	Passed   bool
	Failures []StageStaticCheckFailure
}

// StageStaticCheckFailure captures a single diagnostic from a static check.
type StageStaticCheckFailure struct {
	RuleID   string
	File     string
	Line     int
	Column   int
	Severity string
	Message  string
}

// StageLogFinding captures a normalized log finding generated during build ingestion.
type StageLogFinding struct {
	Code     string
	Severity string
	Message  string
	Evidence string
}

// StageModsMetadata carries Mods planner metadata for checkpoints.
type StageModsMetadata struct {
	Plan            *StageModsPlan
	Human           *StageModsHuman
	Recommendations []StageModsRecommendation
}

// StageModsPlan summarises planner output exposed to Grid consumers.
type StageModsPlan struct {
	SelectedRecipes []string
	ParallelStages  []string
	HumanGate       bool
	Summary         string
	PlanTimeout     string
	MaxParallel     int
}

// StageModsHuman captures expectations for human checkpoints.
type StageModsHuman struct {
	Required  bool
	Playbooks []string
}

// StageModsRecommendation records a single Mods recommendation entry.
type StageModsRecommendation struct {
	Source     string
	Message    string
	Confidence float64
}

// Artifact represents a manifest describing an output produced by a workflow
// stage and referenced in checkpoints for downstream consumers.
type Artifact struct {
	Name        string
	ArtifactCID string
	Digest      string
	MediaType   string
}

// StageJobSpec captures job execution configuration attached to a stage.
type StageJobSpec struct {
	Image     string
	Command   []string
	Env       map[string]string
	Resources StageJobResources
	Metadata  map[string]string
}

// StageJobResources expresses resource hints for Grid scheduling.
type StageJobResources struct {
	CPU    string
	Memory string
	Disk   string
	GPU    string
}

type StageConstraints struct {
	Manifest manifests.Compilation
}

type StageAster struct {
	Enabled bool
	Toggles []string
	Bundles []aster.Metadata
}

type StageStatus = contracts.CheckpointStatus

const (
	StageStatusPending   StageStatus = contracts.CheckpointStatusPending
	StageStatusClaimed   StageStatus = contracts.CheckpointStatusClaimed
	StageStatusRunning   StageStatus = contracts.CheckpointStatusRunning
	StageStatusRetrying  StageStatus = contracts.CheckpointStatusRetrying
	StageStatusCompleted StageStatus = contracts.CheckpointStatusCompleted
	StageStatusFailed    StageStatus = contracts.CheckpointStatusFailed
)

type StageOutcome struct {
	Stage     Stage
	Status    StageStatus
	Retryable bool
	Message   string
	Artifacts []Artifact
}
