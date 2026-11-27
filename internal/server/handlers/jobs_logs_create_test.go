package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

func newJobLogUploadHandler(t *testing.T, st *handlerStore, jobID domaintypes.JobID, runID domaintypes.RunID) http.HandlerFunc {
	t.Helper()
	st.getJob.val = store.Job{ID: jobID, RunID: runID}
	objKey := "logs/job/" + jobID.String() + "/log/1.gz"
	st.createLog.val = store.Log{ID: 1, RunID: runID, JobID: &jobID, ChunkNo: 2, ObjectKey: &objKey}
	eventsService, err := createTestEventsServiceWithStore(st)
	if err != nil {
		t.Fatal(err)
	}
	return createJobLogsHandler(st, blobpersist.New(st, bsmock.New()), eventsService)
}

func TestCreateJobLogsHandler(t *testing.T) {
	t.Parallel()

	chunk := gzipLines(t, "hello")
	for _, tt := range []struct {
		name       string
		runStats   string
		getJobErr  error
		payload    map[string]any
		wantStatus int
	}{
		{
			name:       "fresh execution stores chunk",
			payload:    map[string]any{"chunk_no": 2, "data": chunk},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "resumed execution stores chunk",
			runStats:   `{"resume_count":4}`,
			payload:    map[string]any{"resume_count": 4, "chunk_no": 2, "data": chunk},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "earlier execution is rejected",
			runStats:   `{"resume_count":4}`,
			payload:    map[string]any{"resume_count": 3, "chunk_no": 2, "data": chunk},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "job not found",
			getJobErr:  pgx.ErrNoRows,
			payload:    map[string]any{"chunk_no": 0, "data": chunk},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "empty data",
			payload:    map[string]any{"chunk_no": 0, "data": []byte{}},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "data too large",
			payload:    map[string]any{"chunk_no": 0, "data": make([]byte, ingestMaxDataSize+1)},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:       "body too large",
			payload:    map[string]any{"chunk_no": 0, "data": make([]byte, ingestMaxBodySize)},
			wantStatus: http.StatusRequestEntityTooLarge,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			jobID, runID := domaintypes.NewJobID(), domaintypes.NewRunID()
			st := &handlerStore{}
			st.getJob.err = tt.getJobErr
			st.getRun.val.Stats = []byte(tt.runStats)
			h := newJobLogUploadHandler(t, st, jobID, runID)

			rr := doRequest(t, h, http.MethodPost, "/v1/jobs/"+jobID.String()+"/logs", tt.payload, "job_id", jobID.String())
			assertStatus(t, rr, tt.wantStatus)
			if tt.wantStatus != http.StatusCreated {
				if st.createLog.called {
					t.Fatal("stored a rejected chunk")
				}
				return
			}
			params := st.createLog.params
			if params.JobID == nil || *params.JobID != jobID || params.RunID != runID || params.ChunkNo != 2 {
				t.Fatalf("stored chunk for unexpected job, run, or chunk number: %+v", params)
			}
			var ack struct {
				ID      int64
				ChunkNo int32 `json:"chunk_no"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &ack); err != nil || ack.ID != 1 || ack.ChunkNo != 2 {
				t.Fatalf("unexpected acknowledgement %s: %v", rr.Body.String(), err)
			}
		})
	}
}

func FuzzCreateJobLogsHandler(f *testing.F) {
	f.Add([]byte("hello"), int32(0))
	f.Add([]byte{}, int32(1))
	f.Fuzz(func(t *testing.T, data []byte, chunkNo int32) {
		jobID := domaintypes.NewJobID()
		handler := newJobLogUploadHandler(t, &handlerStore{}, jobID, domaintypes.NewRunID())
		payload := map[string]any{"chunk_no": chunkNo, "data": data}
		rr := doRequest(t, handler, http.MethodPost, "/v1/jobs/"+jobID.String()+"/logs", payload, "job_id", jobID.String())
		switch {
		case len(data) == 0:
			assertStatus(t, rr, http.StatusBadRequest)
		case len(data) > ingestMaxDataSize:
			assertStatus(t, rr, http.StatusRequestEntityTooLarge)
		default:
			assertStatus(t, rr, http.StatusCreated)
		}
	})
}
