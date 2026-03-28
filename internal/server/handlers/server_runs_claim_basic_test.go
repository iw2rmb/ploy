package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestClaimJob_HappyPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		opts   claimJobFixtureOptions
		assert func(t *testing.T, rr *httptest.ResponseRecorder, f *claimJobFixture)
	}{
		{
			name: "success returns expected fields",
			opts: claimJobFixtureOptions{},
			assert: func(t *testing.T, rr *httptest.ResponseRecorder, f *claimJobFixture) {
				t.Helper()
				if !f.store.claimJobCalled || string(f.store.claimJobParams) != f.nodeID.String() {
					t.Fatalf("expected ClaimJob to be called with node id")
				}
				if len(f.store.updateRunRepoStatusParams) == 0 {
					t.Fatalf("expected UpdateRunRepoStatus to be called")
				}

				resp := decodeBody[map[string]any](t, rr)
				if resp["id"] != f.runID.String() {
					t.Fatalf("expected id (run_id) %s, got %v", f.runID.String(), resp["id"])
				}
				if resp["job_id"] != f.jobID.String() {
					t.Fatalf("expected job_id %s, got %v", f.jobID.String(), resp["job_id"])
				}
				if resp["repo_id"] != f.repoID.String() {
					t.Fatalf("expected repo_id %s, got %v", f.repoID.String(), resp["repo_id"])
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
				if resp["status"] != "Started" {
					t.Fatalf("expected status Started, got %v", resp["status"])
				}

				spec, ok := resp["spec"].(map[string]any)
				if !ok {
					t.Fatalf("expected spec to be an object, got %T", resp["spec"])
				}
				if spec["job_id"] != f.jobID.String() {
					t.Fatalf("expected spec.job_id %s, got %v", f.jobID.String(), spec["job_id"])
				}
				if _, ok := spec["mod_index"]; ok {
					t.Fatalf("expected spec.mod_index to be absent, got %v", spec["mod_index"])
				}
			},
		},
		{
			name: "MR job does not update run repo status",
			opts: claimJobFixtureOptions{
				jobType:       domaintypes.JobTypeMR,
				jobName:       "mr-0",
				runStatus:     domaintypes.RunStatusFinished,
				runRepoStatus: domaintypes.RunRepoStatusSuccess,
			},
			assert: func(t *testing.T, rr *httptest.ResponseRecorder, f *claimJobFixture) {
				t.Helper()
				if len(f.store.updateRunRepoStatusParams) != 0 {
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
			},
		},
		{
			name: "response includes next_id",
			opts: claimJobFixtureOptions{},
			assert: func(t *testing.T, rr *httptest.ResponseRecorder, f *claimJobFixture) {
				t.Helper()
				resp := decodeBody[map[string]any](t, rr)
				if _, ok := resp["next_id"]; !ok {
					t.Fatalf("expected claim response to include next_id")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newClaimJobFixture(t, tc.opts)
			rr := f.serve()
			assertStatus(t, rr, http.StatusOK)
			tc.assert(t, rr, f)
		})
	}
}

func TestClaimJob_SpecFromDBMustBeJSONObject(t *testing.T) {
	t.Parallel()

	f := newClaimJobFixture(t, claimJobFixtureOptions{
		specJSON: []byte(`[]`),
	})
	rr := f.serve()
	assertStatus(t, rr, http.StatusInternalServerError)
}

func TestClaimJob_NoJobsAvailable(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
	st := &mockStore{}
	st.getNode.val = store.Node{ID: nodeID}

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID.String()+"/claim", nil, "id", nodeID.String())

	assertStatus(t, rr, http.StatusNoContent)
}

func TestClaimJob_NodeNotFound(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	st := func() *mockStore { st := &mockStore{}; st.getNode.err = pgx.ErrNoRows; return st }()

	handler := claimJobHandler(st, &ConfigHolder{})
	rr := doRequest(t, handler, http.MethodPost, "/v1/nodes/"+nodeID+"/claim", nil, "id", nodeID)

	assertStatus(t, rr, http.StatusNotFound)
}
