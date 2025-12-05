package graph

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// makeJobID creates a pgtype.UUID from a UUID.
func makeJobID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

// makeTimestamp creates a pgtype.Timestamptz from a time.Time.
func makeTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// TestBuildFromJobs_SimpleRun verifies graph building from a simple 3-job run.
func TestBuildFromJobs_SimpleRun(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	pgRunID := makeJobID(runID)

	job1ID := uuid.New()
	job2ID := uuid.New()
	job3ID := uuid.New()

	jobs := []store.Job{
		{
			ID:        makeJobID(job1ID),
			RunID:     pgRunID,
			Name:      "pre-gate",
			Status:    store.JobStatusSucceeded,
			ModType:   "pre_gate",
			StepIndex: 1000,
		},
		{
			ID:        makeJobID(job2ID),
			RunID:     pgRunID,
			Name:      "mod-0",
			Status:    store.JobStatusRunning,
			ModType:   "mod",
			ModImage:  "mods-orw:latest",
			StepIndex: 2000,
		},
		{
			ID:        makeJobID(job3ID),
			RunID:     pgRunID,
			Name:      "post-gate",
			Status:    store.JobStatusCreated,
			ModType:   "post_gate",
			StepIndex: 3000,
		},
	}

	graph := BuildFromJobs(pgRunID, jobs)

	// Verify run ID.
	if graph.RunID != runID.String() {
		t.Errorf("RunID = %q, want %q", graph.RunID, runID.String())
	}

	// Verify node count.
	if graph.NodeCount() != 3 {
		t.Errorf("NodeCount() = %d, want 3", graph.NodeCount())
	}

	// Verify node types.
	preGate := graph.GetNode(job1ID.String())
	if preGate == nil || preGate.Type != NodeTypePreGate {
		t.Errorf("pre-gate node type = %v, want %v", preGate, NodeTypePreGate)
	}

	mod0 := graph.GetNode(job2ID.String())
	if mod0 == nil || mod0.Type != NodeTypeMod {
		t.Errorf("mod-0 node type = %v, want %v", mod0, NodeTypeMod)
	}
	if mod0.Image != "mods-orw:latest" {
		t.Errorf("mod-0 Image = %q, want %q", mod0.Image, "mods-orw:latest")
	}

	postGate := graph.GetNode(job3ID.String())
	if postGate == nil || postGate.Type != NodeTypePostGate {
		t.Errorf("post-gate node type = %v, want %v", postGate, NodeTypePostGate)
	}

	// Verify edges: pre-gate → mod-0 → post-gate.
	if len(preGate.ChildIDs) != 1 || preGate.ChildIDs[0] != job2ID.String() {
		t.Errorf("pre-gate should have child mod-0, got %v", preGate.ChildIDs)
	}
	if len(mod0.ParentIDs) != 1 || mod0.ParentIDs[0] != job1ID.String() {
		t.Errorf("mod-0 should have parent pre-gate, got %v", mod0.ParentIDs)
	}
	if len(mod0.ChildIDs) != 1 || mod0.ChildIDs[0] != job3ID.String() {
		t.Errorf("mod-0 should have child post-gate, got %v", mod0.ChildIDs)
	}

	// Verify roots and leaves.
	if len(graph.RootIDs) != 1 || graph.RootIDs[0] != job1ID.String() {
		t.Errorf("RootIDs should be [%s], got %v", job1ID.String(), graph.RootIDs)
	}
	if len(graph.LeafIDs) != 1 || graph.LeafIDs[0] != job3ID.String() {
		t.Errorf("LeafIDs should be [%s], got %v", job3ID.String(), graph.LeafIDs)
	}

	// Should be linear.
	if !graph.Linear {
		t.Error("graph should be linear")
	}
}

// TestBuildFromJobs_WithHealing verifies graph building with healing jobs.
func TestBuildFromJobs_WithHealing(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	pgRunID := makeJobID(runID)

	// Pre-gate (failed) → heal-1 → re-gate → mod-0 → post-gate
	preGateID := uuid.New()
	heal1ID := uuid.New()
	reGateID := uuid.New()
	mod0ID := uuid.New()
	postGateID := uuid.New()

	jobs := []store.Job{
		{ID: makeJobID(preGateID), RunID: pgRunID, Name: "pre-gate", Status: store.JobStatusFailed, ModType: "pre_gate", StepIndex: 1000},
		{ID: makeJobID(heal1ID), RunID: pgRunID, Name: "heal-1", Status: store.JobStatusSucceeded, ModType: "heal", StepIndex: 1500, ModImage: "mods-codex:latest"},
		{ID: makeJobID(reGateID), RunID: pgRunID, Name: "re-gate", Status: store.JobStatusSucceeded, ModType: "re_gate", StepIndex: 1750},
		{ID: makeJobID(mod0ID), RunID: pgRunID, Name: "mod-0", Status: store.JobStatusRunning, ModType: "mod", StepIndex: 2000},
		{ID: makeJobID(postGateID), RunID: pgRunID, Name: "post-gate", Status: store.JobStatusCreated, ModType: "post_gate", StepIndex: 3000},
	}

	graph := BuildFromJobs(pgRunID, jobs)

	// Verify 5 nodes.
	if graph.NodeCount() != 5 {
		t.Errorf("NodeCount() = %d, want 5", graph.NodeCount())
	}

	// Verify healing node.
	heal1 := graph.GetNode(heal1ID.String())
	if heal1 == nil || heal1.Type != NodeTypeHeal {
		t.Errorf("heal-1 type = %v, want %v", heal1, NodeTypeHeal)
	}
	if !heal1.IsHealingNode() {
		t.Error("heal-1 should be identified as healing node")
	}

	// Verify re-gate node.
	reGate := graph.GetNode(reGateID.String())
	if reGate == nil || reGate.Type != NodeTypeReGate {
		t.Errorf("re-gate type = %v, want %v", reGate, NodeTypeReGate)
	}
	if !reGate.IsGateNode() {
		t.Error("re-gate should be identified as gate node")
	}

	// Verify chain: pre-gate → heal-1 → re-gate → mod-0 → post-gate.
	preGate := graph.GetNode(preGateID.String())
	if len(preGate.ChildIDs) != 1 || preGate.ChildIDs[0] != heal1ID.String() {
		t.Errorf("pre-gate should have child heal-1, got %v", preGate.ChildIDs)
	}
	if len(heal1.ParentIDs) != 1 || heal1.ParentIDs[0] != preGateID.String() {
		t.Errorf("heal-1 should have parent pre-gate, got %v", heal1.ParentIDs)
	}
	if len(heal1.ChildIDs) != 1 || heal1.ChildIDs[0] != reGateID.String() {
		t.Errorf("heal-1 should have child re-gate, got %v", heal1.ChildIDs)
	}
}

// TestBuildFromJobs_EmptyJobs verifies graph building with no jobs.
func TestBuildFromJobs_EmptyJobs(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	pgRunID := makeJobID(runID)

	graph := BuildFromJobs(pgRunID, []store.Job{})

	if graph.NodeCount() != 0 {
		t.Errorf("NodeCount() = %d, want 0", graph.NodeCount())
	}
	if graph.RunID != runID.String() {
		t.Errorf("RunID = %q, want %q", graph.RunID, runID.String())
	}
}

// TestBuildFromJobs_InvalidRunID verifies graph building with invalid run ID.
func TestBuildFromJobs_InvalidRunID(t *testing.T) {
	t.Parallel()

	// Invalid (not valid) pgtype.UUID.
	invalidRunID := pgtype.UUID{Valid: false}

	jobs := []store.Job{
		{ID: makeJobID(uuid.New()), Name: "job-1", Status: store.JobStatusPending, ModType: "mod", StepIndex: 1000},
	}

	graph := BuildFromJobs(invalidRunID, jobs)

	// Should still build graph, but RunID should be empty.
	if graph.RunID != "" {
		t.Errorf("RunID should be empty for invalid input, got %q", graph.RunID)
	}
	if graph.NodeCount() != 1 {
		t.Errorf("NodeCount() = %d, want 1", graph.NodeCount())
	}
}

// TestBuildFromJobs_StatusMapping verifies all status mappings.
func TestBuildFromJobs_StatusMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		storeStatus store.JobStatus
		wantStatus  NodeStatus
	}{
		{store.JobStatusCreated, NodeStatusCreated},
		{store.JobStatusPending, NodeStatusPending},
		{store.JobStatusRunning, NodeStatusRunning},
		{store.JobStatusSucceeded, NodeStatusSucceeded},
		{store.JobStatusFailed, NodeStatusFailed},
		{store.JobStatusSkipped, NodeStatusSkipped},
		{store.JobStatusCanceled, NodeStatusCanceled},
	}

	for _, tt := range tests {
		t.Run(string(tt.storeStatus), func(t *testing.T) {
			t.Parallel()

			runID := makeJobID(uuid.New())
			jobID := uuid.New()

			jobs := []store.Job{
				{ID: makeJobID(jobID), RunID: runID, Name: "job", Status: tt.storeStatus, ModType: "mod", StepIndex: 1000},
			}

			graph := BuildFromJobs(runID, jobs)
			node := graph.GetNode(jobID.String())

			if node.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", node.Status, tt.wantStatus)
			}
		})
	}
}

// TestBuildFromJobs_TypeMapping verifies all mod_type mappings.
func TestBuildFromJobs_TypeMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		modType  string
		wantType NodeType
	}{
		{"pre_gate", NodeTypePreGate},
		{"mod", NodeTypeMod},
		{"heal", NodeTypeHeal},
		{"re_gate", NodeTypeReGate},
		{"post_gate", NodeTypePostGate},
		{"unknown", NodeTypeMod}, // Fallback.
		{"", NodeTypeMod},        // Empty fallback.
	}

	for _, tt := range tests {
		t.Run(tt.modType, func(t *testing.T) {
			t.Parallel()

			runID := makeJobID(uuid.New())
			jobID := uuid.New()

			jobs := []store.Job{
				{ID: makeJobID(jobID), RunID: runID, Name: "job", Status: store.JobStatusPending, ModType: tt.modType, StepIndex: 1000},
			}

			graph := BuildFromJobs(runID, jobs)
			node := graph.GetNode(jobID.String())

			if node.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", node.Type, tt.wantType)
			}
		})
	}
}

// TestBuildFromJobs_TimestampMapping verifies timestamp mapping.
func TestBuildFromJobs_TimestampMapping(t *testing.T) {
	t.Parallel()

	runID := makeJobID(uuid.New())
	jobID := uuid.New()

	startedAt := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	finishedAt := time.Date(2024, 1, 15, 10, 31, 0, 0, time.UTC)

	jobs := []store.Job{
		{
			ID:         makeJobID(jobID),
			RunID:      runID,
			Name:       "job",
			Status:     store.JobStatusSucceeded,
			ModType:    "mod",
			StepIndex:  1000,
			StartedAt:  makeTimestamp(startedAt),
			FinishedAt: makeTimestamp(finishedAt),
			DurationMs: 60000,
			ExitCode:   intPtr(0),
		},
	}

	graph := BuildFromJobs(runID, jobs)
	node := graph.GetNode(jobID.String())

	if node.StartedAt == nil || !node.StartedAt.Equal(startedAt) {
		t.Errorf("StartedAt = %v, want %v", node.StartedAt, startedAt)
	}
	if node.FinishedAt == nil || !node.FinishedAt.Equal(finishedAt) {
		t.Errorf("FinishedAt = %v, want %v", node.FinishedAt, finishedAt)
	}
	if node.DurationMs != 60000 {
		t.Errorf("DurationMs = %d, want 60000", node.DurationMs)
	}
	if node.ExitCode == nil || *node.ExitCode != 0 {
		t.Errorf("ExitCode = %v, want 0", node.ExitCode)
	}
}

// TestBuildFromJobs_NilTimestamps verifies nil timestamp handling.
func TestBuildFromJobs_NilTimestamps(t *testing.T) {
	t.Parallel()

	runID := makeJobID(uuid.New())
	jobID := uuid.New()

	jobs := []store.Job{
		{
			ID:        makeJobID(jobID),
			RunID:     runID,
			Name:      "job",
			Status:    store.JobStatusPending,
			ModType:   "mod",
			StepIndex: 1000,
			// StartedAt and FinishedAt not set (Invalid).
		},
	}

	graph := BuildFromJobs(runID, jobs)
	node := graph.GetNode(jobID.String())

	if node.StartedAt != nil {
		t.Errorf("StartedAt should be nil for unset timestamp, got %v", node.StartedAt)
	}
	if node.FinishedAt != nil {
		t.Errorf("FinishedAt should be nil for unset timestamp, got %v", node.FinishedAt)
	}
}

// TestBuildFromJobsWithEdgeStrategy verifies custom edge strategy support.
func TestBuildFromJobsWithEdgeStrategy(t *testing.T) {
	t.Parallel()

	runID := makeJobID(uuid.New())
	job1ID := uuid.New()
	job2ID := uuid.New()

	jobs := []store.Job{
		{ID: makeJobID(job1ID), RunID: runID, Name: "a", Status: store.JobStatusPending, ModType: "mod", StepIndex: 1000},
		{ID: makeJobID(job2ID), RunID: runID, Name: "b", Status: store.JobStatusCreated, ModType: "mod", StepIndex: 2000},
	}

	// Use linear strategy (default).
	graph := BuildFromJobsWithEdgeStrategy(runID, jobs, &LinearEdgeStrategy{})

	// Verify linear edges.
	nodeA := graph.GetNode(job1ID.String())
	if len(nodeA.ChildIDs) != 1 || nodeA.ChildIDs[0] != job2ID.String() {
		t.Errorf("'a' should have child 'b', got %v", nodeA.ChildIDs)
	}
}

// TestBuildFromJobsWithEdgeStrategy_NilStrategy verifies nil strategy handling.
func TestBuildFromJobsWithEdgeStrategy_NilStrategy(t *testing.T) {
	t.Parallel()

	runID := makeJobID(uuid.New())
	jobs := []store.Job{
		{ID: makeJobID(uuid.New()), RunID: runID, Name: "job", Status: store.JobStatusPending, ModType: "mod", StepIndex: 1000},
	}

	// Nil strategy should not panic; uses default edges.
	graph := BuildFromJobsWithEdgeStrategy(runID, jobs, nil)
	if graph.NodeCount() != 1 {
		t.Errorf("NodeCount() = %d, want 1", graph.NodeCount())
	}
}

// TestHealingWindowEdgeStrategy verifies healing window strategy.
func TestHealingWindowEdgeStrategy(t *testing.T) {
	t.Parallel()

	runID := makeJobID(uuid.New())
	jobs := []store.Job{
		{ID: makeJobID(uuid.New()), RunID: runID, Name: "a", Status: store.JobStatusPending, ModType: "mod", StepIndex: 1000},
		{ID: makeJobID(uuid.New()), RunID: runID, Name: "b", Status: store.JobStatusCreated, ModType: "mod", StepIndex: 2000},
	}

	// Healing window strategy should work (currently delegates to linear).
	graph := BuildFromJobsWithEdgeStrategy(runID, jobs, &HealingWindowEdgeStrategy{})
	if graph.NodeCount() != 2 {
		t.Errorf("NodeCount() = %d, want 2", graph.NodeCount())
	}
}

// TestBuildFromJobs_MultiStepRun verifies multi-step run graph (mods[] array).
func TestBuildFromJobs_MultiStepRun(t *testing.T) {
	t.Parallel()

	runID := makeJobID(uuid.New())

	// Multi-step: pre-gate → mod-0 → mod-1 → mod-2 → post-gate
	preGateID := uuid.New()
	mod0ID := uuid.New()
	mod1ID := uuid.New()
	mod2ID := uuid.New()
	postGateID := uuid.New()

	jobs := []store.Job{
		{ID: makeJobID(preGateID), RunID: runID, Name: "pre-gate", Status: store.JobStatusSucceeded, ModType: "pre_gate", StepIndex: 1000},
		{ID: makeJobID(mod0ID), RunID: runID, Name: "mod-0", Status: store.JobStatusSucceeded, ModType: "mod", StepIndex: 2000},
		{ID: makeJobID(mod1ID), RunID: runID, Name: "mod-1", Status: store.JobStatusSucceeded, ModType: "mod", StepIndex: 3000},
		{ID: makeJobID(mod2ID), RunID: runID, Name: "mod-2", Status: store.JobStatusRunning, ModType: "mod", StepIndex: 4000},
		{ID: makeJobID(postGateID), RunID: runID, Name: "post-gate", Status: store.JobStatusCreated, ModType: "post_gate", StepIndex: 5000},
	}

	graph := BuildFromJobs(runID, jobs)

	// Verify 5 nodes in linear chain.
	if graph.NodeCount() != 5 {
		t.Errorf("NodeCount() = %d, want 5", graph.NodeCount())
	}
	if !graph.Linear {
		t.Error("multi-step run should be linear")
	}

	// Verify chain: pre-gate → mod-0 → mod-1 → mod-2 → post-gate.
	ordered := graph.OrderedNodes()
	expectedOrder := []string{"pre-gate", "mod-0", "mod-1", "mod-2", "post-gate"}
	for i, node := range ordered {
		if node.Name != expectedOrder[i] {
			t.Errorf("ordered[%d].Name = %q, want %q", i, node.Name, expectedOrder[i])
		}
	}
}

// intPtr returns a pointer to the given int32.
func intPtr(i int32) *int32 {
	return &i
}
