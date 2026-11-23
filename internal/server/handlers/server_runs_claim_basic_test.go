package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
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
				if !f.store.claimJob.called || string(f.store.claimJob.params) != f.nodeID.String() {
					t.Fatalf("expected ClaimJob to be called with node id")
				}
				if len(f.store.updateRunStatus.calls) != 0 {
					t.Fatalf("claim unexpectedly changed run status")
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
				if resp["commit_sha"] != f.sourceCommitSHA {
					t.Fatalf("expected commit_sha %s, got %v", f.sourceCommitSHA, resp["commit_sha"])
				}
				if resp["status"] != "Running" {
					t.Fatalf("expected status Running, got %v", resp["status"])
				}

				spec, ok := resp["spec"].(map[string]any)
				if !ok {
					t.Fatalf("expected spec to be an object, got %T", resp["spec"])
				}
				if spec["job_id"] != f.jobID.String() {
					t.Fatalf("expected spec.job_id %s, got %v", f.jobID.String(), spec["job_id"])
				}
				if _, ok := spec["mig_index"]; ok {
					t.Fatalf("expected spec.mig_index to be absent, got %v", spec["mig_index"])
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
	st := &handlerStore{}
	st.getNode.val = store.Node{ID: nodeID}

	handler := claimJobHandler(st, nil, &ConfigHolder{}, "test-secret")
	rr := serveClaim(handler, nodeID.String())

	assertStatus(t, rr, http.StatusNoContent)
}

func TestClaimJob_NodeNotFound(t *testing.T) {
	t.Parallel()

	nodeID := domaintypes.NewNodeKey()
	st := func() *handlerStore { st := &handlerStore{}; st.getNode.err = pgx.ErrNoRows; return st }()

	handler := claimJobHandler(st, nil, &ConfigHolder{}, "test-secret")
	rr := serveClaim(handler, nodeID)

	assertStatus(t, rr, http.StatusNotFound)
}

func TestClaimJob_IssuesJobToken(t *testing.T) {
	t.Setenv("PLOY_AUTH_TOKEN", "shared-server-token")
	f := newClaimJobFixture(t, claimJobFixtureOptions{})
	rr := f.serve()
	assertStatus(t, rr, http.StatusOK)
	if bytes.Contains(rr.Body.Bytes(), []byte("shared-server-token")) {
		t.Fatal("claim exposed the server process token")
	}
	result := decodeBody[domainapi.NodeClaimResponse](t, rr)
	claims, err := auth.ValidateToken(result.JobToken, "test-secret")
	if err != nil || claims.Role != string(auth.RoleJob) {
		t.Fatal("claim must return a signed job token")
	}
	hash := sha256.Sum256([]byte(result.JobToken))
	row := f.store.upsertJobToken.params
	if row.JobID != f.jobID || row.TokenID != claims.ID || row.TokenHash != hex.EncodeToString(hash[:]) {
		t.Fatal("stored job token does not match the claim")
	}
	if bytes.Contains(result.Spec, []byte(result.JobToken)) || bytes.Contains(f.store.getSpec.val.Spec, []byte(result.JobToken)) {
		t.Fatal("claim put job token in specification")
	}
}

func TestClaimJob_TokenIssuanceFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		requeue bool
	}{
		{"persistence error", errors.New("database unavailable"), http.StatusInternalServerError, true},
		{"claim no longer current", pgx.ErrNoRows, http.StatusNoContent, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newClaimJobFixture(t, claimJobFixtureOptions{})
			f.store.upsertJobToken.err = tc.err
			rr := f.serve()
			assertStatus(t, rr, tc.status)
			if f.store.unclaimJob.called != tc.requeue {
				t.Errorf("UnclaimJob called = %v, want %v", f.store.unclaimJob.called, tc.requeue)
			}
		})
	}
}

func TestClaimJob_RequiresJobTokenCapabilityBeforeClaim(t *testing.T) {
	f := newClaimJobFixture(t, claimJobFixtureOptions{})
	req := newClaimRequest(f.nodeKey)
	req.Header.Del(domainapi.JobTokenCapabilityHeader)
	rr := httptest.NewRecorder()
	claimJobHandler(f.store, nil, f.config, "test-secret").ServeHTTP(rr, req)
	assertStatus(t, rr, http.StatusUpgradeRequired)
	if f.store.claimJob.called {
		t.Fatal("server assigned a job to a node without job token support")
	}
}
