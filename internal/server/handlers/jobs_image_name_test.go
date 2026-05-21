package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestSaveJobImageNameHandler(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name       string
		bodyImage  string
		jobStatus  domaintypes.JobStatus
		jobType    domaintypes.JobType
		wrongNode  bool
		wantStatus int
		wantUpdate bool
		wantImage  string
	}
	tests := []testCase{
		{name: "mig job", bodyImage: "docker.io/example/migs:latest", jobStatus: domaintypes.JobStatusRunning, jobType: domaintypes.JobTypeMig, wantStatus: http.StatusNoContent, wantUpdate: true, wantImage: "docker.io/example/migs:latest"},
		{name: "pre gate job", bodyImage: "docker.io/example/migs:latest", jobStatus: domaintypes.JobStatusRunning, jobType: domaintypes.JobTypePreGate, wantStatus: http.StatusNoContent, wantUpdate: true, wantImage: "docker.io/example/migs:latest"},
		{name: "post gate job", bodyImage: "docker.io/example/gate:latest", jobStatus: domaintypes.JobStatusRunning, jobType: domaintypes.JobTypePostGate, wantStatus: http.StatusNoContent, wantUpdate: true, wantImage: "docker.io/example/gate:latest"},
		{name: "empty image", bodyImage: "   ", wantStatus: http.StatusBadRequest},
		{name: "wrong node", bodyImage: "docker.io/example/migs:latest", jobStatus: domaintypes.JobStatusRunning, jobType: domaintypes.JobTypeMig, wrongNode: true, wantStatus: http.StatusForbidden},
		{name: "job not running", bodyImage: "docker.io/example/migs:latest", jobStatus: domaintypes.JobStatusQueued, jobType: domaintypes.JobTypeMig, wantStatus: http.StatusConflict},
		{name: "wrong job type", bodyImage: "docker.io/example/migs:latest", jobStatus: domaintypes.JobStatusRunning, jobType: domaintypes.JobType("unknown"), wantStatus: http.StatusConflict},
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
			st := &jobStore{}
			st.getJob.val = store.Job{
				ID:      jobID,
				RunID:   domaintypes.NewRunID(),
				NodeID:  &jobNodeID,
				Status:  tt.jobStatus,
				JobType: tt.jobType,
			}
			handler := saveJobImageNameHandler(st)
			body, _ := json.Marshal(map[string]any{"image": tt.bodyImage})
			req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+jobID.String()+"/image", bytes.NewReader(body))
			req.SetPathValue("job_id", jobID.String())
			req.Header.Set(nodeUUIDHeader, nodeIDStr)
			req = req.WithContext(auth.ContextWithIdentity(req.Context(), auth.Identity{Role: auth.RoleWorker, CommonName: nodeIDStr}))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assertStatus(t, rr, tt.wantStatus)
			if st.updateJobImageName.called != tt.wantUpdate {
				t.Fatalf("UpdateJobImageName called = %v, want %v", st.updateJobImageName.called, tt.wantUpdate)
			}
			if !tt.wantUpdate {
				return
			}
			if st.updateJobImageName.params.ID != jobID {
				t.Fatalf("UpdateJobImageName ID = %s, want %s", st.updateJobImageName.params.ID, jobID)
			}
			if st.updateJobImageName.params.JobImage != tt.wantImage {
				t.Fatalf("UpdateJobImageName JobImage = %q, want %q", st.updateJobImageName.params.JobImage, tt.wantImage)
			}
		})
	}
}
