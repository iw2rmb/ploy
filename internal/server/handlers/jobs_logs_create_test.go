package handlers

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// POST /v1/jobs/{job_id}/logs

func TestCreateJobLogsHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setupStore     func(jobID domaintypes.JobID, runID domaintypes.RunID) *handlerStore
		payload        map[string]any
		wantStatus     int
		wantGetJobCall bool
	}{
		{
			name: "stale resume generation",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *handlerStore {
				st := &handlerStore{}
				st.getJob.val = store.Job{ID: jobID, RunID: runID}
				st.getRun.val.Stats = []byte(`{"resume_count":1}`)
				return st
			},
			payload:        map[string]any{"chunk_no": 0, "data": []byte("old")},
			wantStatus:     http.StatusConflict,
			wantGetJobCall: true,
		},
		{
			name: "success",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *handlerStore {
				objKey := "logs/job/" + jobID.String() + "/log/1.gz"
				st := &handlerStore{}
				st.getJob.val = store.Job{ID: jobID, RunID: runID}
				st.createLog.val = store.Log{ID: 1, RunID: runID, JobID: &jobID, ChunkNo: 2, DataSize: 5, ObjectKey: &objKey}
				return st
			},
			payload:        map[string]any{"chunk_no": 2, "data": []byte("hello")},
			wantStatus:     http.StatusCreated,
			wantGetJobCall: true,
		},
		{
			name: "job not found",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *handlerStore {
				st := &handlerStore{}
				st.getJob.err = pgx.ErrNoRows
				return st
			},
			payload:        map[string]any{"chunk_no": 0, "data": []byte("x")},
			wantStatus:     http.StatusNotFound,
			wantGetJobCall: true,
		},
		{
			name: "empty data",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *handlerStore {
				st := &handlerStore{}
				st.getJob.val = store.Job{ID: jobID, RunID: runID}
				return st
			},
			payload:        map[string]any{"chunk_no": 0, "data": []byte{}},
			wantStatus:     http.StatusBadRequest,
			wantGetJobCall: true,
		},
		{
			name: "too large",
			setupStore: func(jobID domaintypes.JobID, runID domaintypes.RunID) *handlerStore {
				st := &handlerStore{}
				st.getJob.val = store.Job{ID: jobID, RunID: runID}
				return st
			},
			payload:        map[string]any{"chunk_no": 0, "data": make([]byte, 10<<20+1)},
			wantStatus:     http.StatusRequestEntityTooLarge,
			wantGetJobCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runID := domaintypes.NewRunID()
			jobID := domaintypes.NewJobID()
			st := tt.setupStore(jobID, runID)

			eventsService, err := createTestEventsServiceWithStore(st)
			if err != nil {
				t.Fatalf("events service: %v", err)
			}
			h := createJobLogsHandler(st, blobpersist.New(st, bsmock.New()), eventsService)

			rr := doRequest(t, h, http.MethodPost, "/v1/jobs/"+jobID.String()+"/logs", tt.payload, "job_id", jobID.String())
			assertStatus(t, rr, tt.wantStatus)
			if st.getJob.called != tt.wantGetJobCall {
				t.Fatalf("getJob.called = %v, want %v", st.getJob.called, tt.wantGetJobCall)
			}
		})
	}
}
