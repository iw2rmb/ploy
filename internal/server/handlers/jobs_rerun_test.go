package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestRerunJobHandler_HealCreatesNewAttemptAndTail(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	sourceID := domaintypes.NewJobID()
	nextID := domaintypes.NewJobID()

	st := &jobStore{}
	st.getJobResult = store.Job{
		ID:        sourceID,
		RunID:     runID,
		RepoID:    repoID,
		Attempt:   2,
		Status:    domaintypes.JobStatusFail,
		JobType:   domaintypes.JobTypeHeal,
		JobImage:  "docker.io/test/heal:v1",
		RepoShaIn: "0123456789abcdef0123456789abcdef01234567",
		NextID:    &nextID,
		Meta:      []byte(`{"kind":"mig"}`),
	}
	st.getJobResults = map[domaintypes.JobID]store.Job{
		nextID: {
			ID:      nextID,
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 2,
			JobType: domaintypes.JobTypeReGate,
			Meta:    []byte(`{"kind":"gate","recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}`),
		},
	}
	st.getRun.val = store.Run{ID: runID, Status: domaintypes.RunStatusFinished}
	st.getRunRepoResults = []store.RunRepo{
		{RunID: runID, RepoID: repoID, RepoBaseRef: "main", Attempt: 2},
		{RunID: runID, RepoID: repoID, RepoBaseRef: "main", Attempt: 3},
	}

	h := rerunJobHandler(st)
	rr := doRequest(t, h, http.MethodPost, "/v1/jobs/"+sourceID.String()+"/rerun", map[string]any{
		"alter": map[string]any{
			"image":      "docker.io/test/heal:debug",
			"envs":       map[string]any{"DEBUG": "1"},
			"in":         []any{"abc1234:/in/build-log.txt"},
			"bundle_map": map[string]any{"abc1234": "bundle_1"},
		},
	}, "job_id", sourceID.String())

	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !st.incrementRunRepoAttempt.called {
		t.Fatal("expected IncrementRunRepoAttempt")
	}
	if !st.updateRunStatus.called {
		t.Fatal("expected terminal run to be reopened")
	}
	if got := len(st.createJob.calls); got != 2 {
		t.Fatalf("expected 2 created jobs (re_gate + root), got %d", got)
	}
	root := st.createJob.calls[1]
	if root.Attempt != 3 {
		t.Fatalf("root attempt=%d want 3", root.Attempt)
	}
	if root.Status != domaintypes.JobStatusQueued {
		t.Fatalf("root status=%s want Queued", root.Status)
	}
	if root.JobType != domaintypes.JobTypeHeal {
		t.Fatalf("root type=%s want heal", root.JobType)
	}
	if root.JobImage != "docker.io/test/heal:debug" {
		t.Fatalf("root image=%q", root.JobImage)
	}

	var rootMeta map[string]any
	if err := json.Unmarshal(root.Meta, &rootMeta); err != nil {
		t.Fatalf("unmarshal root meta: %v", err)
	}
	rerunMeta, _ := rootMeta[rerunMetaKey].(map[string]any)
	if rerunMeta == nil {
		t.Fatalf("expected %s metadata", rerunMetaKey)
	}
	if got := rerunMeta["source_job_id"]; got != sourceID.String() {
		t.Fatalf("source_job_id=%v want %s", got, sourceID)
	}
	alterMeta, _ := rerunMeta[rerunMetaAlterKey].(map[string]any)
	if alterMeta == nil {
		t.Fatalf("expected %s.%s metadata object", rerunMetaKey, rerunMetaAlterKey)
	}
	bundleMap, _ := alterMeta["bundle_map"].(map[string]any)
	if bundleMap == nil {
		t.Fatalf("expected alter.bundle_map metadata")
	}
	if got := bundleMap["abc1234"]; got != "bundle_1" {
		t.Fatalf("bundle_map[abc1234]=%v want bundle_1", got)
	}
}

func TestRerunJobHandler_UnsupportedType(t *testing.T) {
	sourceID := domaintypes.NewJobID()
	st := &jobStore{}
	st.getJobResult = store.Job{
		ID:      sourceID,
		JobType: domaintypes.JobTypeMig,
		Status:  domaintypes.JobStatusFail,
	}

	h := rerunJobHandler(st)
	rr := doRequest(t, h, http.MethodPost, "/v1/jobs/"+sourceID.String()+"/rerun", map[string]any{
		"alter": map[string]any{"envs": map[string]any{"DEBUG": "1"}},
	}, "job_id", sourceID.String())
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestApplyRerunAlterMutator_OverridesSpecValues(t *testing.T) {
	job := store.Job{
		ID:      domaintypes.NewJobID(),
		JobType: domaintypes.JobTypeHeal,
		Meta: []byte(`{
			"kind":"mig",
			"_rerun":{
				"source_job_id":"src",
				"mode":"drift-ok",
				"alter":{
					"image":"docker.io/test/heal:debug",
					"envs":{"DEBUG":"1"},
					"in":["hash-new:/in/build-log.txt"]
				}
			}
		}`),
	}
	input := claimSpecMutatorInput{job: job, jobType: domaintypes.JobTypeHeal}
	m := map[string]any{
		"build_gate": map[string]any{
			"heal": map[string]any{
				"image": "docker.io/test/heal:old",
				"envs":  map[string]any{"A": "a", "DEBUG": "0"},
				"in":    []any{"hash-old:/in/build-log.txt", "hash-keep:/in/other.txt"},
			},
		},
	}

	if err := applyRerunAlterMutator(m, input); err != nil {
		t.Fatalf("applyRerunAlterMutator: %v", err)
	}
	heal := m["build_gate"].(map[string]any)["heal"].(map[string]any)
	if got := heal["image"]; got != "docker.io/test/heal:debug" {
		t.Fatalf("image=%v", got)
	}
	envs := heal["envs"].(map[string]any)
	if envs["DEBUG"] != "1" {
		t.Fatalf("DEBUG env=%v", envs["DEBUG"])
	}
	inVals := heal["in"].([]any)
	if len(inVals) != 2 {
		t.Fatalf("in len=%d want 2", len(inVals))
	}
	if got := inVals[0].(string); got != "hash-new:/in/build-log.txt" {
		t.Fatalf("in[0]=%q", got)
	}
}

func TestNormalizeRerunAlter_AllowsEmpty(t *testing.T) {
	alter, err := normalizeRerunAlter(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alter.Image != "" || len(alter.Envs) != 0 || len(alter.In) != 0 || len(alter.BundleMap) != 0 {
		t.Fatalf("expected zero-value alter, got %#v", alter)
	}
}

func TestNormalizeRerunAlter_AllowsNil(t *testing.T) {
	alter, err := normalizeRerunAlter(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alter.Image != "" || len(alter.Envs) != 0 || len(alter.In) != 0 || len(alter.BundleMap) != 0 {
		t.Fatalf("expected zero-value alter, got %#v", alter)
	}
}

func TestNormalizeRerunAlter_RejectsInvalidBundleMap(t *testing.T) {
	_, err := normalizeRerunAlter(map[string]any{
		"image":      "docker.io/test/heal:debug",
		"bundle_map": "not-an-object",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "bundle_map must be an object" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTerminalJobStatus(t *testing.T) {
	if !isTerminalJobStatus(domaintypes.JobStatusSuccess) {
		t.Fatal("success should be terminal")
	}
	if isTerminalJobStatus(domaintypes.JobStatusRunning) {
		t.Fatal("running should not be terminal")
	}
}

func TestRerunHandler_RunNotFound(t *testing.T) {
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	sourceID := domaintypes.NewJobID()
	st := &jobStore{}
	st.getJobResult = store.Job{ID: sourceID, RunID: runID, RepoID: repoID, JobType: domaintypes.JobTypeHeal, Status: domaintypes.JobStatusFail}
	st.getRun.err = errors.New("db down")
	h := rerunJobHandler(st)
	rr := doRequest(t, h, http.MethodPost, "/v1/jobs/"+sourceID.String()+"/rerun", map[string]any{"alter": map[string]any{"image": "x"}}, "job_id", sourceID.String())
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
