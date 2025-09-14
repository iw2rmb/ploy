package mods

// BranchStep describes a new branch step identity and its diff key path.
type BranchStep struct {
	ID      string
	DiffKey string
}

// NewBranchStep generates a new step ID and corresponding diff key under artifacts/mods.
func NewBranchStep(modID, branchID string) BranchStep {
	sid := randomStepID()
	return BranchStep{ID: sid, DiffKey: computeBranchDiffKey(modID, branchID, sid)}
}
