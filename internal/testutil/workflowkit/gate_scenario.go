package workflowkit

import "github.com/iw2rmb/ploy/internal/workflow/contracts"

// GateProfileScenario provides canonical StepGateSpec values for gate tests.
type GateProfileScenario struct {
	// PrepCommandSpec is a basic enabled gate spec.
	PrepCommandSpec *contracts.StepGateSpec
}

// NewGateProfileScenario returns canonical gate specs for workflow tests.
func NewGateProfileScenario() GateProfileScenario {
	return GateProfileScenario{
		PrepCommandSpec: &contracts.StepGateSpec{
			Enabled: true,
		},
	}
}
