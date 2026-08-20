package contracts

// MigClaimContext carries the concrete mig step selected for execution.
type MigClaimContext struct {
	StepIndex int `json:"step_index"`
}

// GateClaimContext carries concrete gate execution routing.
type GateClaimContext struct {
	CycleName string `json:"cycle_name"`
}
