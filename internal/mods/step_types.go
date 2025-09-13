package transflow

// StepType represents the kind of transformation/healing step.
// Canonical values are used across planner outputs, fanout branches,
// and runner step execution to avoid string literal drift.
type StepType string

const (
	StepTypeORWApply  StepType = "orw-apply"
	StepTypeLLMExec   StepType = "llm-exec"
	StepTypeORWGen    StepType = "orw-gen"
	StepTypeHumanStep StepType = "human-step"
)

// NormalizeStepType converts planner/legacy aliases to canonical StepType.
// Example: planner may emit "human" which maps to "human-step".
func NormalizeStepType(s string) StepType {
	switch s {
	case string(StepTypeORWApply):
		return StepTypeORWApply
	case string(StepTypeLLMExec):
		return StepTypeLLMExec
	case string(StepTypeORWGen):
		return StepTypeORWGen
	case "human", string(StepTypeHumanStep):
		return StepTypeHumanStep
	default:
		// Preserve unknown for forward compatibility; caller may validate.
		return StepType(s)
	}
}

// IsValid returns true if the step type is one of the supported canonical types.
func (t StepType) IsValid() bool {
	switch t {
	case StepTypeORWApply, StepTypeLLMExec, StepTypeORWGen, StepTypeHumanStep:
		return true
	default:
		return false
	}
}
