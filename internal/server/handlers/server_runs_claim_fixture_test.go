package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type claimJobFixtureOptions struct {
	jobType       domaintypes.JobType
	jobName       string
	runStatus     domaintypes.RunStatus
	runRepoStatus domaintypes.RunRepoStatus
	specJSON      []byte
	jobMeta       []byte
	jobImage      string
}

type claimJobFixture struct {
	nodeKey string
	nodeID  domaintypes.NodeID
	runID   domaintypes.RunID
	repoID  domaintypes.RepoID
	specID  domaintypes.SpecID
	jobID   domaintypes.JobID

	store  *jobStore
	config *ConfigHolder
}

func newClaimJobFixture(t testing.TB, opts claimJobFixtureOptions) *claimJobFixture {
	t.Helper()

	nodeKey := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeKey)
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()
	jobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	if opts.jobType == "" {
		opts.jobType = domaintypes.JobTypeMig
	}
	if opts.jobName == "" {
		opts.jobName = "mig-0"
	}
	if opts.runStatus == "" {
		opts.runStatus = domaintypes.RunStatusStarted
	}
	if opts.runRepoStatus == "" {
		opts.runRepoStatus = domaintypes.RunRepoStatusQueued
	}
	if len(opts.specJSON) == 0 {
		opts.specJSON = []byte(`{"steps":[{"image":"a"}]}`)
	}
	if len(opts.jobMeta) == 0 {
		opts.jobMeta = []byte(`{}`)
	}

	st := &jobStore{
		getRunRepoResult: store.RunRepo{
			RunID:         runID,
			RepoID:        repoID,
			RepoBaseRef:   "main",
			RepoTargetRef: "feature-branch",
			Status:        opts.runRepoStatus,
			Attempt:       1,
		},
	}
	st.getNode.val = store.Node{ID: nodeID}
	st.claimJob.val = store.Job{
		ID:          jobID,
		RunID:       runID,
		RepoID:      repoID,
		RepoBaseRef: "main",
		Attempt:     1,
		NodeID:      &nodeID,
		Name:        opts.jobName,
		Status:      domaintypes.JobStatusRunning,
		JobType:     opts.jobType,
		JobImage:    opts.jobImage,
		Meta:        opts.jobMeta,
	}
	st.getRun.val = store.Run{
		ID:        runID,
		SpecID:    specID,
		Status:    opts.runStatus,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		StartedAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	st.getMigRepo.val = store.MigRepo{ID: domaintypes.NewMigRepoID(), RepoID: repoID}
	st.getSpec.val = store.Spec{ID: specID, Spec: opts.specJSON}

	return &claimJobFixture{
		nodeKey: nodeKey,
		nodeID:  nodeID,
		runID:   runID,
		repoID:  repoID,
		specID:  specID,
		jobID:   jobID,
		store:   st,
		config:  &ConfigHolder{},
	}
}

func (f *claimJobFixture) serve() *httptest.ResponseRecorder {
	handler := claimJobHandler(f.store, f.config)
	req := httptest.NewRequest(http.MethodPost, "/v1/nodes/"+f.nodeKey+"/claim", nil)
	req.SetPathValue("id", f.nodeKey)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func decodeClaimResponse(t testing.TB, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}
