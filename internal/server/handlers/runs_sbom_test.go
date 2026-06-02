package handlers

import (
	"context"
	"net/http"
	"testing"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestGetRunSBOMHandler_ViewsAndValidation(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewRepoID()

	tests := []struct {
		name       string
		view       string
		spec       []byte
		rows       map[domaintypes.JobType][]store.ListRunSBOMRowsByJobTypeRow
		wantStatus int
		want       func(t *testing.T, body map[string]any)
		wantBody   string
		wantCalls  []domaintypes.JobType
	}{
		{
			name: "pre returns pre-gate packages",
			view: "pre",
			spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
			rows: map[domaintypes.JobType][]store.ListRunSBOMRowsByJobTypeRow{
				domaintypes.JobTypePreGate: {
					{Lib: "alpha", Ver: "1.0.0"},
					{Lib: "beta", Ver: "2.0.0"},
				},
			},
			wantStatus: http.StatusOK,
			wantCalls:  []domaintypes.JobType{domaintypes.JobTypePreGate},
			want: func(t *testing.T, body map[string]any) {
				packages := body["packages"].([]any)
				if len(packages) != 2 {
					t.Fatalf("packages len=%d, want 2", len(packages))
				}
				first := packages[0].(map[string]any)
				if first["package"] != "alpha" || first["version"] != "1.0.0" {
					t.Fatalf("first package=%v, want alpha 1.0.0", first)
				}
			},
		},
		{
			name: "post returns post-gate packages",
			view: "post",
			spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
			rows: map[domaintypes.JobType][]store.ListRunSBOMRowsByJobTypeRow{
				domaintypes.JobTypePostGate: {
					{Lib: "omega", Ver: "9.0.0"},
				},
			},
			wantStatus: http.StatusOK,
			wantCalls:  []domaintypes.JobType{domaintypes.JobTypePostGate},
			want: func(t *testing.T, body map[string]any) {
				packages := body["packages"].([]any)
				if len(packages) != 1 {
					t.Fatalf("packages len=%d, want 1", len(packages))
				}
				first := packages[0].(map[string]any)
				if first["package"] != "omega" || first["version"] != "9.0.0" {
					t.Fatalf("first package=%v, want omega 9.0.0", first)
				}
			},
		},
		{
			name: "diff returns changed added and removed packages",
			view: "diff",
			spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
			rows: map[domaintypes.JobType][]store.ListRunSBOMRowsByJobTypeRow{
				domaintypes.JobTypePreGate: {
					{Lib: "changed-lib", Ver: "1.0.0"},
					{Lib: "common-lib", Ver: "1.0.0"},
					{Lib: "removed-lib", Ver: "1.0.0"},
				},
				domaintypes.JobTypePostGate: {
					{Lib: "added-lib", Ver: "1.0.0"},
					{Lib: "changed-lib", Ver: "2.0.0"},
					{Lib: "common-lib", Ver: "1.0.0"},
				},
			},
			wantStatus: http.StatusOK,
			wantCalls:  []domaintypes.JobType{domaintypes.JobTypePreGate, domaintypes.JobTypePostGate},
			want: func(t *testing.T, body map[string]any) {
				packages := body["packages"].([]any)
				if len(packages) != 3 {
					t.Fatalf("packages len=%d, want 3: %v", len(packages), packages)
				}
				want := []map[string]string{
					{"package": "added-lib", "version_pre": "", "version_post": "1.0.0", "change": "added"},
					{"package": "changed-lib", "version_pre": "1.0.0", "version_post": "2.0.0", "change": "changed"},
					{"package": "removed-lib", "version_pre": "1.0.0", "version_post": "", "change": "removed"},
				}
				for i, item := range packages {
					got := item.(map[string]any)
					for key, value := range want[i] {
						if got[key] != value {
							t.Fatalf("packages[%d][%s]=%v, want %q; package=%v", i, key, got[key], value, got)
						}
					}
				}
			},
		},
		{
			name:       "disabled build gate returns bad request",
			view:       "diff",
			spec:       []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}],"build_gate":{"disabled":true}}`),
			wantStatus: http.StatusBadRequest,
			wantBody:   "build gate disabled for run\n",
		},
		{
			name:       "invalid view returns bad request",
			view:       "current",
			spec:       []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid sbom view\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := &runStore{
				sbomRowsByJobType: tt.rows,
			}
			st.getRun.val = store.Run{ID: runID, SpecID: specID, RepoID: repoID, Attempt: 1}
			st.getSpec.val = store.Spec{ID: specID, Spec: tt.spec}

			handler := getRunSBOMHandler(st, nil)
			rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/sbom/"+tt.view, nil, "run_id", runID.String(), "view", tt.view)

			assertStatus(t, rr, tt.wantStatus)
			if tt.wantBody != "" {
				if rr.Body.String() != tt.wantBody {
					t.Fatalf("body=%q, want %q", rr.Body.String(), tt.wantBody)
				}
				return
			}
			if len(st.listRunSBOMRowsByJobType.calls) != len(tt.wantCalls) {
				t.Fatalf("ListRunSBOMRowsByJobType calls=%d, want %d", len(st.listRunSBOMRowsByJobType.calls), len(tt.wantCalls))
			}
			for i, want := range tt.wantCalls {
				if got := st.listRunSBOMRowsByJobType.calls[i].JobType; got != want {
					t.Fatalf("call %d job_type=%s, want %s", i, got, want)
				}
			}
			body := decodeBody[map[string]any](t, rr)
			if body["run_id"] != runID.String() {
				t.Fatalf("run_id=%v, want %s", body["run_id"], runID)
			}
			if body["view"] != tt.view {
				t.Fatalf("view=%v, want %s", body["view"], tt.view)
			}
			tt.want(t, body)
		})
	}
}

func TestGetRunSBOMHandler_BackfillsRowsFromExistingGateArtifacts(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.NewJobID()
	objKey := "artifacts/run/" + runID.String() + "/bundle/sbom.tar.gz"
	st := &runStore{
		sbomRowsByJobType: map[domaintypes.JobType][]store.ListRunSBOMRowsByJobTypeRow{},
		sbomJobTypeByJobID: map[domaintypes.JobID]domaintypes.JobType{
			jobID: domaintypes.JobTypePostGate,
		},
	}
	st.getRun.val = store.Run{ID: runID, SpecID: specID, RepoID: repoID, Attempt: 1}
	st.getSpec.val = store.Spec{ID: specID, Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`)}
	st.listJobsByRunAttempt.val = []store.Job{{
		ID:      jobID,
		RunID:   runID,
		RepoID:  repoID,
		Attempt: 1,
		Status:  domaintypes.JobStatusSuccess,
		JobType: domaintypes.JobTypePostGate,
	}}
	st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{{
		RunID:     runID,
		JobID:     &jobID,
		ObjectKey: &objKey,
	}}
	bs := bsmock.New()
	bundle := mustTarGzPayload(t, map[string][]byte{
		"artifacts/shared/sbom.spdx.json": []byte(`{
  "spdxVersion":"SPDX-2.3",
  "packages":[{"name":"org.example:lib-a","versionInfo":"1.0.0"}]
}`),
	})
	if _, err := bs.Put(context.Background(), objKey, "application/gzip", bundle); err != nil {
		t.Fatalf("put blob: %v", err)
	}
	handler := getRunSBOMHandler(st, blobpersist.New(st, bs))

	rr := doRequest(t, handler, http.MethodGet, "/v1/runs/"+runID.String()+"/sbom/post", nil, "run_id", runID.String(), "view", "post")

	assertStatus(t, rr, http.StatusOK)
	if len(st.listRunSBOMRowsByJobType.calls) != 2 {
		t.Fatalf("ListRunSBOMRowsByJobType calls=%d, want 2", len(st.listRunSBOMRowsByJobType.calls))
	}
	if len(st.upsertSBOMRow.calls) != 1 {
		t.Fatalf("upsert calls=%d, want 1", len(st.upsertSBOMRow.calls))
	}
	body := decodeBody[map[string]any](t, rr)
	packages := body["packages"].([]any)
	if len(packages) != 1 {
		t.Fatalf("packages len=%d, want 1", len(packages))
	}
	first := packages[0].(map[string]any)
	if first["package"] != "org.example:lib-a" || first["version"] != "1.0.0" {
		t.Fatalf("first package=%v, want org.example:lib-a 1.0.0", first)
	}
}
