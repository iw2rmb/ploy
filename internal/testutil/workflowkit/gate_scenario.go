package workflowkit

import "github.com/iw2rmb/ploy/internal/workflow/contracts"

// GateProfileScenario provides canonical StepGateSpec values for gate-profile
// override orchestration tests in workflow/step and related modules.
// Use NewGateProfileScenario to construct one.
type GateProfileScenario struct {
	// PrepCommandSpec is a gate spec with a profile override prep command.
	PrepCommandSpec *contracts.StepGateSpec
}

// NewGateProfileScenario returns canonical gate specs for gate-profile override
// orchestration tests. The returned specs match the system contract between the
// server-side profile configuration and the workflow step executor.
func NewGateProfileScenario() GateProfileScenario {
	return GateProfileScenario{
		PrepCommandSpec: &contracts.StepGateSpec{
			Enabled: true,
			GateProfile: &contracts.BuildGateProfileOverride{
				Command: contracts.CommandSpec{Shell: "echo prep-gate"},
			},
		},
	}
}
