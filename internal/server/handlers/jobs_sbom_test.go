package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestSaveJobSBOMHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		jobType    domaintypes.JobType
		jobStatus  domaintypes.JobStatus
		wrongNode  bool
		packages   []migsapi.RunSBOMPackage
		wantStatus int
		wantRows   []store.UpsertSBOMRowParams
	}{
		{
			name:       "pre gate rows replace existing rows",
			jobType:    domaintypes.JobTypePreGate,
			jobStatus:  domaintypes.JobStatusRunning,
			wantStatus: http.StatusOK,
			packages: []migsapi.RunSBOMPackage{
				{Package: " Org.Example:Lib-A ", Version: " 1.0.0 "},
				{Package: "org.example:lib-a", Version: "1.0.0"},
				{Package: "org.example:lib-b", Version: "2.0.0"},
				{Package: "skip-empty-version", Version: ""},
			},
			wantRows: []store.UpsertSBOMRowParams{
				{Lib: "org.example:lib-a", Ver: "1.0.0"},
				{Lib: "org.example:lib-b", Ver: "2.0.0"},
			},
		},
		{
			name:       "post gate rows replace existing rows",
			jobType:    domaintypes.JobTypePostGate,
			jobStatus:  domaintypes.JobStatusRunning,
			wantStatus: http.StatusOK,
			packages:   []migsapi.RunSBOMPackage{{Package: "org.example:post-lib", Version: "3.0.0"}},
			wantRows:   []store.UpsertSBOMRowParams{{Lib: "org.example:post-lib", Ver: "3.0.0"}},
		},
		{
			name:       "non gate job rejected",
			jobType:    domaintypes.JobTypeMig,
			jobStatus:  domaintypes.JobStatusRunning,
			wantStatus: http.StatusConflict,
			packages:   []migsapi.RunSBOMPackage{{Package: "org.example:lib", Version: "1.0.0"}},
		},
		{
			name:       "non running gate rejected",
			jobType:    domaintypes.JobTypePreGate,
			jobStatus:  domaintypes.JobStatusSuccess,
			wantStatus: http.StatusConflict,
			packages:   []migsapi.RunSBOMPackage{{Package: "org.example:lib", Version: "1.0.0"}},
		},
		{
			name:       "wrong node rejected",
			jobType:    domaintypes.JobTypePreGate,
			jobStatus:  domaintypes.JobStatusRunning,
			wrongNode:  true,
			wantStatus: http.StatusForbidden,
			packages:   []migsapi.RunSBOMPackage{{Package: "org.example:lib", Version: "1.0.0"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			nodeIDStr := domaintypes.NewNodeKey()
			nodeID := domaintypes.NodeID(nodeIDStr)
			jobNodeID := nodeID
			if tt.wrongNode {
				jobNodeID = domaintypes.NodeID(domaintypes.NewNodeKey())
			}
			jobID := domaintypes.NewJobID()
			repoID := domaintypes.NewRepoID()
			st := &jobStore{}
			st.getJob.val = store.Job{
				ID:      jobID,
				RunID:   domaintypes.NewRunID(),
				RepoID:  repoID,
				NodeID:  &jobNodeID,
				Status:  tt.jobStatus,
				JobType: tt.jobType,
			}

			body, _ := json.Marshal(migsapi.JobSBOMUploadRequest{Packages: tt.packages})
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/sbom", bytes.NewReader(body))
			req.SetPathValue("job_id", jobID.String())
			req.Header.Set(nodeUUIDHeader, nodeIDStr)
			req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

			rr := httptest.NewRecorder()
			saveJobSBOMHandler(st).ServeHTTP(rr, req)

			assertStatus(t, rr, tt.wantStatus)
			if tt.wantStatus != http.StatusOK {
				if len(st.deleteSBOMRowsByJob.calls) != 0 {
					t.Fatalf("DeleteSBOMRowsByJob calls = %+v, want none", st.deleteSBOMRowsByJob.calls)
				}
				if len(st.upsertSBOMRow.calls) != 0 {
					t.Fatalf("UpsertSBOMRow calls = %+v, want none", st.upsertSBOMRow.calls)
				}
				return
			}
			if len(st.deleteSBOMRowsByJob.calls) != 1 || st.deleteSBOMRowsByJob.calls[0] != jobID {
				t.Fatalf("DeleteSBOMRowsByJob calls = %+v, want [%s]", st.deleteSBOMRowsByJob.calls, jobID)
			}
			if len(st.upsertSBOMRow.calls) != len(tt.wantRows) {
				t.Fatalf("UpsertSBOMRow calls = %d, want %d: %+v", len(st.upsertSBOMRow.calls), len(tt.wantRows), st.upsertSBOMRow.calls)
			}
			for i, want := range tt.wantRows {
				got := st.upsertSBOMRow.calls[i]
				if got.JobID != jobID || got.RepoID != repoID || got.Lib != want.Lib || got.Ver != want.Ver {
					t.Fatalf("upsert[%d] = %+v, want job=%s repo=%s lib=%s ver=%s", i, got, jobID, repoID, want.Lib, want.Ver)
				}
			}
			var resp migsapi.JobSBOMUploadResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp.JobID != jobID || resp.RowCount != len(tt.wantRows) {
				t.Fatalf("response = %+v, want job_id=%s row_count=%d", resp, jobID, len(tt.wantRows))
			}
		})
	}
}
