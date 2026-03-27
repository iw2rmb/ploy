package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestClaimJob_Success(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mig-0",
			Status:      domaintypes.JobStatusRunning,
			JobType:     domaintypes.JobTypeMod,
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
		getModRepoResult: store.MigRepo{
			ID:     domaintypes.NewMigRepoID(),
			RepoID: repoID,
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil, "id", nodeKey)

	assertStatus(t, rr, http.StatusOK)
	if !st.claimJobCalled || string(st.claimJobParams) != nodeID.String() {
		t.Fatalf("expected ClaimJob to be called with node id")
	}
	if len(st.updateRunRepoStatusParams) == 0 {
		t.Fatalf("expected UpdateRunRepoStatus to be called")
	}

	resp := decodeBody[map[string]any](t, rr)

	if resp["id"] != runID.String() {
		t.Fatalf("expected id (run_id) %s, got %v", runID.String(), resp["id"])
	}
	if resp["job_id"] != jobID.String() {
		t.Fatalf("expected job_id %s, got %v", jobID.String(), resp["job_id"])
	}
	if resp["repo_id"] != repoID.String() {
		t.Fatalf("expected repo_id %s, got %v", repoID.String(), resp["repo_id"])
	}
	if resp["repo_url"] != "https://github.com/user/repo.git" {
		t.Fatalf("expected repo_url, got %v", resp["repo_url"])
	}
	if resp["base_ref"] != "main" {
		t.Fatalf("expected base_ref main, got %v", resp["base_ref"])
	}
	if resp["target_ref"] != "feature-branch" {
		t.Fatalf("expected target_ref feature-branch, got %v", resp["target_ref"])
	}
	if got, ok := resp["repo_gate_profile_missing"].(bool); !ok || !got {
		t.Fatalf("expected repo_gate_profile_missing=true, got %v", resp["repo_gate_profile_missing"])
	}
	// v1: run status should be "Started" (not HEAD literals like "assigned"/"running").
	// v1 run status values are Started, Cancelled, or Finished.
	if resp["status"] != "Started" {
		t.Fatalf("expected status Started, got %v", resp["status"])
	}

	spec, ok := resp["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec to be an object, got %T", resp["spec"])
	}
	if spec["job_id"] != jobID.String() {
		t.Fatalf("expected spec.job_id %s, got %v", jobID.String(), spec["job_id"])
	}
	if _, ok := spec["mod_index"]; ok {
		t.Fatalf("expected spec.mod_index to be absent, got %v", spec["mod_index"])
	}
}

func TestClaimJob_SpecFromDBMustBeJSONObject(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mig-0",
			Status:      domaintypes.JobStatusRunning,
			JobType:     domaintypes.JobTypeMod,
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
		getModRepoResult: store.MigRepo{
			ID:     domaintypes.NewMigRepoID(),
			RepoID: repoID,
		},
		// Spec is sourced from the DB at claim time; it must be a JSON object.
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`[]`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil, "id", nodeKey)

	assertStatus(t, rr, http.StatusInternalServerError)
}

func TestClaimJob_MRJob_DoesNotUpdateRunRepoStatus(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mr-0",
			Status:      domaintypes.JobStatusRunning,
			JobType:     domaintypes.JobTypeMR,
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    domaintypes.RunStatusFinished,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        domaintypes.RunRepoStatusSuccess,
			Attempt:       1,
		},
		getModRepoResult: store.MigRepo{
			ID:     domaintypes.NewMigRepoID(),
			RepoID: repoID,
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil, "id", nodeKey)

	assertStatus(t, rr, http.StatusOK)
	if len(st.updateRunRepoStatusParams) != 0 {
		t.Fatalf("expected UpdateRunRepoStatus not to be called for MR jobs")
	}

	resp := decodeBody[map[string]any](t, rr)
	if resp["status"] != "Finished" {
		t.Fatalf("expected status Finished, got %v", resp["status"])
	}
	if resp["repo_url"] != "https://github.com/user/repo.git" {
		t.Fatalf("expected repo_url, got %v", resp["repo_url"])
	}
	if resp["base_ref"] != "main" {
		t.Fatalf("expected base_ref main, got %v", resp["base_ref"])
	}
	if resp["target_ref"] != "feature-branch" {
		t.Fatalf("expected target_ref feature-branch, got %v", resp["target_ref"])
	}
}

func TestClaimJob_NoJobsAvailable(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		// claimJobResult left empty: mock ClaimJob returns pgx.ErrNoRows.
	}

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil, "id", nodeID.String())

	assertStatus(t, rr, http.StatusNoContent)
}

func TestClaimJob_NodeNotFound(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	st := &mockStore{getNodeErr: pgx.ErrNoRows}

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil, "id", nodeID)

	assertStatus(t, rr, http.StatusNotFound)
}

func TestClaimJob_ResponseUsesNextIDContract(t *testing.T) {
	t.Parallel()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	st := &mockStore{
		getNodeResult: store.Node{ID: nodeID},
		claimJobResult: store.Job{
			ID:          jobID,
			RunID:       runID,
			RepoID:      repoID,
			RepoBaseRef: "main",
			Attempt:     1,
			NodeID:      &nodeID,
			Name:        "mig-0",
			Status:      domaintypes.JobStatusRunning,
			JobType:     domaintypes.JobTypeMod,
			Meta:        []byte(`{}`),
		},
		getRunResult: store.Run{
			ID:        runID,
			SpecID:    specID,
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        domaintypes.RunRepoStatusQueued,
			Attempt:       1,
		},
		getModRepoResult: store.MigRepo{
			ID:     domaintypes.NewMigRepoID(),
			RepoID: repoID,
		},
		getSpecResult: store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"a"}]}`)},
	}

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeKey+"/claim", nil, "id", nodeKey)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[map[string]any](t, rr)

	if _, ok := resp["next_id"]; !ok {
		t.Fatalf("expected claim response to include next_id")
	}
}
