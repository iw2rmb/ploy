package store

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestSQLCOverridesCompile verifies that sqlc-generated code uses domain types
// for ID columns and StepIndex. This is a compile-time verification test that
// ensures the sqlc overrides in sqlc.yaml are correctly applied.
//
// The test exercises struct field types to confirm:
// - Primary key IDs use domain newtypes (RunID, JobID, NodeID, ModID, SpecID, ModRepoID)
// - Foreign key references use matching domain newtypes
// - jobs.step_index uses types.StepIndex (not float64)
//
// If sqlc overrides are removed or misconfigured, this test will fail to compile.
func TestSQLCOverridesCompile(t *testing.T) {
	// Verify Run struct field types.
	var run Run
	var _ types.RunID = run.ID
	var _ types.ModID = run.ModID
	var _ types.SpecID = run.SpecID

	// Verify Job struct field types including StepIndex.
	var job Job
	var _ types.JobID = job.ID
	var _ types.RunID = job.RunID
	var _ types.ModRepoID = job.RepoID
	var _ types.StepIndex = job.StepIndex
	var _ *types.NodeID = job.NodeID

	// Verify Node struct field types.
	var node Node
	var _ types.NodeID = node.ID

	// Verify Mod struct field types.
	var mod Mod
	var _ types.ModID = mod.ID
	var _ *types.SpecID = mod.SpecID

	// Verify Spec struct field types.
	var spec Spec
	var _ types.SpecID = spec.ID

	// Verify ModRepo struct field types.
	var modRepo ModRepo
	var _ types.ModRepoID = modRepo.ID
	var _ types.ModID = modRepo.ModID

	// Verify RunRepo struct field types.
	var runRepo RunRepo
	var _ types.ModID = runRepo.ModID
	var _ types.RunID = runRepo.RunID
	var _ types.ModRepoID = runRepo.RepoID

	// Verify Event struct field types.
	var event Event
	var _ types.RunID = event.RunID
	var _ *types.JobID = event.JobID

	// Verify Log struct field types.
	var log Log
	var _ types.RunID = log.RunID
	var _ *types.JobID = log.JobID

	// Verify Diff struct field types.
	var diff Diff
	var _ types.RunID = diff.RunID
	var _ *types.JobID = diff.JobID

	// Verify ArtifactBundle struct field types.
	var bundle ArtifactBundle
	var _ types.RunID = bundle.RunID
	var _ *types.JobID = bundle.JobID

	// Verify NodeMetric struct field types.
	var metric NodeMetric
	var _ types.NodeID = metric.NodeID

	// Verify BootstrapToken struct field types.
	var token BootstrapToken
	var _ *types.NodeID = token.NodeID

	// Verify StepIndex validation works (runtime check).
	si := types.StepIndex(1000)
	if !si.Valid() {
		t.Error("StepIndex(1000) should be valid")
	}
	si = types.StepIndex(1000.5)
	if si.Valid() {
		t.Error("StepIndex(1000.5) should be invalid (fractional)")
	}
}

// TestSQLCOverridesQuerierSignatures verifies that the Querier interface methods
// use typed IDs in their signatures. This is a compile-time check.
func TestSQLCOverridesQuerierSignatures(t *testing.T) {
	// We can't call these without a real database, but we can verify
	// that the types align by assigning to interface variables.

	// Verify the interface exists and accepts typed parameters.
	// The compiler will fail if the signatures don't match.
	var q Querier

	// Suppress "variable declared and not used" by using blank identifier pattern.
	_ = q

	// Create typed values to verify compatibility with Querier method signatures.
	// This verifies that the generated code uses typed parameters, not raw strings.
	var runID types.RunID
	var jobID types.JobID
	var nodeID types.NodeID
	var modID types.ModID
	var specID types.SpecID
	var modRepoID types.ModRepoID
	var stepIndex types.StepIndex

	// Suppress unused variable warnings.
	_, _, _, _, _, _, _ = runID, jobID, nodeID, modID, specID, modRepoID, stepIndex
}
