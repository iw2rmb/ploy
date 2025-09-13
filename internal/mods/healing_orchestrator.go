package mods

import "context"

// ProdHealingOrchestrator is a production HealingOrchestrator that delegates to the
// existing fanout orchestrator with the provided submitter and runner.
type ProdHealingOrchestrator struct {
	submitter JobSubmitter
	runner    ProductionBranchRunner
}

func NewProdHealingOrchestrator(submitter JobSubmitter, runner ProductionBranchRunner) *ProdHealingOrchestrator {
	return &ProdHealingOrchestrator{submitter: submitter, runner: runner}
}

func (p *ProdHealingOrchestrator) RunFanout(ctx context.Context, runCtx any, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error) {
	fo := NewFanoutOrchestratorWithRunner(p.submitter, p.runner)
	return fo.RunHealingFanout(ctx, runCtx, branches, maxParallel)
}
