package store

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func assertType[T any](_ T) {}

// TestSQLCOverridesCompile verifies that sqlc-generated code uses domain types
// for ID columns and jobs.next_id. This is a compile-time verification test that
// ensures the sqlc overrides in sqlc.yaml are correctly applied.
//
// The test exercises struct field types to confirm:
// - Primary key IDs use domain newtypes (RunID, JobID, NodeID, MigID, SpecID, MigRepoID)
// - Foreign key references use matching domain newtypes
// - jobs.next_id uses *types.JobID
// - any derived run id columns (e.g. runs_timing.id) also use types.RunID
//
// If sqlc overrides are removed or misconfigured, this test will fail to compile.
func TestSQLCOverridesCompile(t *testing.T) {
	// Verify a few key Querier method signatures use typed IDs (compile-time).
	// This ensures sqlc overrides apply not only to models, but also to query args/returns.
	type typedIDQuerier interface {
		GetRun(ctx context.Context, id types.RunID) (Run, error)
		GetRunTiming(ctx context.Context, id types.RunID) (RunsTiming, error)
		GetJob(ctx context.Context, id types.JobID) (Job, error)
		GetNode(ctx context.Context, id types.NodeID) (Node, error)
		GetMig(ctx context.Context, id types.MigID) (Mig, error)
		GetSpec(ctx context.Context, id types.SpecID) (Spec, error)
		GetMigRepo(ctx context.Context, id types.MigRepoID) (MigRepo, error)
	}
	var _ typedIDQuerier = (Querier)(nil)

	// Verify Run struct field types.
	var run Run
	assertType[types.RunID](run.ID)
	assertType[types.MigID](run.MigID)
	assertType[types.SpecID](run.SpecID)

	// Verify Job struct field types including NextID.
	var job Job
	assertType[types.JobID](job.ID)
	assertType[types.RunID](job.RunID)
	assertType[types.MigRepoID](job.RepoID)
	assertType[*types.JobID](job.NextID)
	assertType[*types.NodeID](job.NodeID)

	// Verify Node struct field types.
	var node Node
	assertType[types.NodeID](node.ID)

	// Verify Mig struct field types.
	var mod Mig
	assertType[types.MigID](mod.ID)
	assertType[*types.SpecID](mod.SpecID)

	// Verify Spec struct field types.
	var spec Spec
	assertType[types.SpecID](spec.ID)

	// Verify MigRepo struct field types.
	var modRepo MigRepo
	assertType[types.MigRepoID](modRepo.ID)
	assertType[types.MigID](modRepo.MigID)

	// Verify RunRepo struct field types.
	var runRepo RunRepo
	assertType[types.MigID](runRepo.MigID)
	assertType[types.RunID](runRepo.RunID)
	assertType[types.MigRepoID](runRepo.RepoID)

	// Verify Event struct field types.
	var event Event
	assertType[types.RunID](event.RunID)
	assertType[*types.JobID](event.JobID)

	// Verify Log struct field types.
	var log Log
	assertType[types.RunID](log.RunID)
	assertType[*types.JobID](log.JobID)

	// Verify Diff struct field types.
	var diff Diff
	assertType[types.RunID](diff.RunID)
	assertType[*types.JobID](diff.JobID)

	// Verify ArtifactBundle struct field types.
	var bundle ArtifactBundle
	assertType[types.RunID](bundle.RunID)
	assertType[*types.JobID](bundle.JobID)

	// Verify NodeMetric struct field types.
	var metric NodeMetric
	assertType[types.NodeID](metric.NodeID)

	// Verify BootstrapToken struct field types.
	var token BootstrapToken
	assertType[*types.NodeID](token.NodeID)

	// Verify derived timing view row types preserve RunID typing.
	var timing RunsTiming
	assertType[types.RunID](timing.ID)
}
