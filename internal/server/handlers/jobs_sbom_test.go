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
		name        string
		jobType     domaintypes.JobType
		jobStatus   domaintypes.JobStatus
		wrongNode   bool
		resumeCount int
		runStats    string
		packages    []migsapi.RunSBOMPackage
		wantStatus  int
		wantRows    []migsapi.RunSBOMPackage
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
				{ModuleID: "module/one", Package: "org.example:lib-a", Version: "1.0.0"},
				{ModuleID: "module/two", Package: "org.example:lib-a", Version: "1.0.0"},
			},
			wantRows: []migsapi.RunSBOMPackage{
				{Package: "org.example:lib-a", Version: "1.0.0"},
				{Package: "org.example:lib-b", Version: "2.0.0"},
				{ModuleID: "module/one", Package: "org.example:lib-a", Version: "1.0.0"},
				{ModuleID: "module/two", Package: "org.example:lib-a", Version: "1.0.0"},
			},
		},
		{
			name:        "current resumed post gate replaces rows",
			resumeCount: 2,
			runStats:    `{"resume_count":2}`,
			jobType:     domaintypes.JobTypePostGate,
			jobStatus:   domaintypes.JobStatusRunning,
			wantStatus:  http.StatusOK,
			packages:    []migsapi.RunSBOMPackage{{Package: "org.example:post-lib", Version: "3.0.0"}},
			wantRows:    []migsapi.RunSBOMPackage{{Package: "org.example:post-lib", Version: "3.0.0"}},
		},
		{
			name:        "previous execution rejected",
			jobType:     domaintypes.JobTypePostGate,
			jobStatus:   domaintypes.JobStatusRunning,
			runStats:    `{"resume_count":2}`,
			resumeCount: 1,
			wantStatus:  http.StatusConflict,
		}, {
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
			st := &handlerStore{}
			st.getRun.val.Stats = []byte(tt.runStats)
			st.getJob.val = store.Job{
				ID:      jobID,
				RunID:   domaintypes.NewRunID(),
				RepoID:  repoID,
				NodeID:  &jobNodeID,
				Status:  tt.jobStatus,
				JobType: tt.jobType,
			}

			body, _ := json.Marshal(migsapi.JobSBOMUploadRequest{ResumeCount: tt.resumeCount, Packages: tt.packages})
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/sbom", bytes.NewReader(body))
			req.SetPathValue("job_id", jobID.String())
			req.Header.Set(nodeUUIDHeader, nodeIDStr)
			req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

			rr := httptest.NewRecorder()
			saveJobSBOMHandler(st).ServeHTTP(rr, req)

			assertStatus(t, rr, tt.wantStatus)
			if tt.wantStatus != http.StatusOK {
				if st.savedGateJob != "" {
					t.Fatal("unexpected gate SBOM replacement")
				}
				return
			}
			if st.savedGateJob != jobID || len(st.savedGateSBOM) != len(tt.wantRows) {
				t.Fatalf("saved gate %s packages %+v, want %s %+v", st.savedGateJob, st.savedGateSBOM, jobID, tt.wantRows)
			}
			for i, expected := range tt.wantRows {
				got := st.savedGateSBOM[i]
				var module string
				if got.ModuleID != nil {
					module = *got.ModuleID
				}
				if (expected.ModuleID == "") != (got.ModuleID == nil) || got.JobID != jobID || got.RepoID != repoID || module != expected.ModuleID || got.Lib != expected.Package || got.Ver != expected.Version {
					t.Fatalf("saved row %d: %+v, module %q, want %+v", i, got, module, expected)
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
