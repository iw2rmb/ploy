package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateJobArtifactHandler_PersistsSBOMRowsForSuccessfulGateArtifacts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		jobType     domaintypes.JobType
		jobStatus   domaintypes.JobStatus
		wantPersist bool
	}{
		{name: "successful pre_gate persists", jobType: domaintypes.JobTypePreGate, jobStatus: domaintypes.JobStatusSuccess, wantPersist: true},
		{name: "successful post_gate persists", jobType: domaintypes.JobTypePostGate, jobStatus: domaintypes.JobStatusSuccess, wantPersist: true},
		{name: "successful mig skips", jobType: domaintypes.JobTypeMig, jobStatus: domaintypes.JobStatusSuccess},
		{name: "running pre_gate skips", jobType: domaintypes.JobTypePreGate, jobStatus: domaintypes.JobStatusRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newRepoScopedFixture(tt.jobType)
			f.Job.Status = tt.jobStatus
			artifactID := uuid.New()
			objectKey := "artifacts/run/" + f.RunID.String() + "/bundle/" + artifactID.String() + ".tar.gz"
			cid := "bafy-test"
			digest := "sha256:test"
			st := newJobStoreForFixture(f)
			st.createArtifactBundle.val = store.ArtifactBundle{
				ID:        pgtype.UUID{Bytes: artifactID, Valid: true},
				RunID:     f.RunID,
				JobID:     &f.JobID,
				ObjectKey: &objectKey,
				Cid:       &cid,
				Digest:    &digest,
			}
			st.listArtifactBundlesByRunAndJob.val = []store.ArtifactBundle{st.createArtifactBundle.val}
			bp := blobpersist.New(st, bsmock.New())
			bundle := mustTarGzPayload(t, map[string][]byte{
				"artifacts/shared/sbom.spdx.json": []byte(`{
  "spdxVersion":"SPDX-2.3",
  "packages":[
    {"name":"org.example:lib-a","versionInfo":"1.0.0"},
    {"name":"org.example:lib-b","versionInfo":"2.0.0"}
  ]
}`),
			})
			body, err := json.Marshal(map[string]any{
				"name":   "repo-artifacts",
				"bundle": bundle,
			})
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+f.RunID.String()+"/jobs/"+f.JobID.String()+"/artifact", bytes.NewReader(body))
			req.SetPathValue("run_id", f.RunID.String())
			req.SetPathValue("job_id", f.JobID.String())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
			rr := httptest.NewRecorder()

			createJobArtifactHandler(st, bp).ServeHTTP(rr, req)

			assertStatus(t, rr, http.StatusCreated)
			wantUpserts := 0
			if tt.wantPersist {
				wantUpserts = 2
			}
			if len(st.upsertSBOMRow.calls) != wantUpserts {
				t.Fatalf("upsert calls = %d, want %d", len(st.upsertSBOMRow.calls), wantUpserts)
			}
			if !tt.wantPersist {
				if st.listArtifactBundlesByRunAndJob.called {
					t.Fatal("ListArtifactBundlesByRunAndJob should not be called")
				}
				return
			}
			if !st.listArtifactBundlesByRunAndJob.called {
				t.Fatal("ListArtifactBundlesByRunAndJob was not called")
			}
			if len(st.deleteSBOMRowsByJob.calls) != 1 || st.deleteSBOMRowsByJob.calls[0] != f.JobID {
				t.Fatalf("DeleteSBOMRowsByJob calls = %+v, want [%s]", st.deleteSBOMRowsByJob.calls, f.JobID)
			}
			got := st.upsertSBOMRow.calls[0]
			if got.JobID != f.JobID || got.RepoID != f.Job.RepoID || got.Lib != "org.example:lib-a" || got.Ver != "1.0.0" {
				t.Fatalf("first upsert = %+v", got)
			}
		})
	}
}
