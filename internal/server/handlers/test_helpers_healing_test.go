package handlers

import (
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// ---------------------------------------------------------------------------
// Gate-failure fixture
// ---------------------------------------------------------------------------

// gateFailureFixture holds the shared setup for gate-failure healing tests.
type gateFailureFixture struct {
	jobTestFixture
	Successor store.Job
	SpecID    domaintypes.SpecID
	SpecBytes []byte
	Store     *jobStore
}

// newGateFailureFixture creates a pre-gate job fixture with recovery metadata,
// a successor mig job, healing spec, and a fully wired mock store.
// recoveryMeta is the raw job meta JSON for the failed gate.
func newGateFailureFixture(t *testing.T, recoveryMeta []byte) gateFailureFixture {
	t.Helper()
	f := newRepoScopedFixture(domaintypes.JobTypePreGate)
	specID := domaintypes.NewSpecID()
	f.Job.Meta = recoveryMeta
	specBytes := buildHealingSpec(t, 1) // uses build_gate.heal

	successor := store.Job{
		ID:          domaintypes.NewJobID(),
		RunID:       f.RunID,
		RepoID:      f.Job.RepoID,
		RepoBaseRef: "main",
		Attempt:     1,
		Name:        "mig-0",
		Status:      domaintypes.JobStatusCreated,
		JobType:     domaintypes.JobTypeMig,
		Meta:        []byte(`{}`),
	}
	f.Job.NextID = &successor.ID

	jobs := []store.Job{f.Job, successor}
	st := newJobStoreForFixture(f,
		withSpec(specID, specBytes),
		withListJobsByRun(jobs),
		withRepoAttemptJobs(jobs),
	)

	return gateFailureFixture{
		jobTestFixture: f,
		Successor:      successor,
		SpecID:         specID,
		SpecBytes:      specBytes,
		Store:          st,
	}
}

// ---------------------------------------------------------------------------
// Healing spec builder
// ---------------------------------------------------------------------------

// healSpecOpt customises the heal spec built by buildHealSpec.
type healSpecOpt func(heal map[string]any)

// withArtifactExpectations adds a standard expectations.artifacts block.
func withArtifactExpectations() healSpecOpt {
	return func(heal map[string]any) {
		heal["expectations"] = map[string]any{
			"artifacts": []any{
				map[string]any{
					"path":   "/out/gate-profile-candidate.json",
					"schema": "gate_profile_v1",
				},
			},
		}
	}
}

// buildHealingSpec returns a serialized spec with standard build_gate.heal config.
// Use opts (e.g. withArtifactExpectations) to extend the heal entry.
func buildHealingSpec(t *testing.T, retries int, opts ...healSpecOpt) []byte {
	t.Helper()
	heal := map[string]any{
		"retries": float64(retries),
		"image":   "codex:latest",
	}
	for _, o := range opts {
		o(heal)
	}
	b, err := json.Marshal(map[string]any{
		"steps": []any{
			map[string]any{"image": "migs-orw:latest"},
		},
		"build_gate": map[string]any{
			"heal": heal,
		},
	})
	if err != nil {
		t.Fatalf("marshal healing spec: %v", err)
	}
	return b
}

// ---------------------------------------------------------------------------
// Job chain assertions
// ---------------------------------------------------------------------------

// assertCancelsSuccessor asserts that no jobs were created and exactly one
// successor was cancelled with the given ID.
func assertCancelsSuccessor(t *testing.T, st *jobStore, successorID domaintypes.JobID) {
	t.Helper()
	if len(st.createJob.calls) != 0 {
		t.Fatalf("expected no healing jobs, got %d CreateJob calls", len(st.createJob.calls))
	}
	if len(st.updateJobStatus.calls) != 1 {
		t.Fatalf("expected one cancelled successor, got %d calls", len(st.updateJobStatus.calls))
	}
	if st.updateJobStatus.calls[0].ID != successorID || st.updateJobStatus.calls[0].Status != domaintypes.JobStatusCancelled {
		t.Fatalf("unexpected cancellation params: %+v", st.updateJobStatus.calls[0])
	}
}

// expectedJob describes the expected attributes of a single job in a chain.
type expectedJob struct {
	Name      string
	JobType   domaintypes.JobType
	Status    domaintypes.JobStatus
	Image     string
	RepoShaIn string // expected repo_sha_in; empty means assert empty
}

// createJobsByName indexes CreateJobParams by name.
func createJobsByName(params []store.CreateJobParams) map[string]store.CreateJobParams {
	byName := make(map[string]store.CreateJobParams, len(params))
	for _, p := range params {
		byName[p.Name] = p
	}
	return byName
}

// assertJobChain verifies that the created jobs match the expected chain,
// checking job_type, status, image, repo_id, repo_base_ref, attempt, run_id, and repo_sha_in.
func assertJobChain(t *testing.T, params []store.CreateJobParams, runID domaintypes.RunID, repoID domaintypes.RepoID, repoBaseRef string, attempt int32, expected []expectedJob) {
	t.Helper()

	if len(params) != len(expected) {
		t.Fatalf("expected %d jobs, got %d", len(expected), len(params))
	}

	byName := createJobsByName(params)
	for _, exp := range expected {
		got, ok := byName[exp.Name]
		if !ok {
			t.Fatalf("missing job %q", exp.Name)
		}
		if got.JobType != exp.JobType {
			t.Errorf("job %q: expected job_type %q, got %q", exp.Name, exp.JobType, got.JobType)
		}
		if got.Status != exp.Status {
			t.Errorf("job %q: expected status %s, got %s", exp.Name, exp.Status, got.Status)
		}
		if exp.Image != "" && got.JobImage != exp.Image {
			t.Errorf("job %q: expected job_image %q, got %q", exp.Name, exp.Image, got.JobImage)
		}
		if got.RepoID != repoID {
			t.Errorf("job %q: expected repo_id %q, got %q", exp.Name, repoID, got.RepoID)
		}
		if got.RepoBaseRef != repoBaseRef {
			t.Errorf("job %q: expected repo_base_ref %q, got %q", exp.Name, repoBaseRef, got.RepoBaseRef)
		}
		if got.Attempt != attempt {
			t.Errorf("job %q: expected attempt %d, got %d", exp.Name, attempt, got.Attempt)
		}
		if got.RunID != runID {
			t.Errorf("job %q: expected run_id %q, got %q", exp.Name, runID, got.RunID)
		}
		if exp.RepoShaIn != "" {
			if got.RepoShaIn != exp.RepoShaIn {
				t.Errorf("job %q: expected repo_sha_in %q, got %q", exp.Name, exp.RepoShaIn, got.RepoShaIn)
			}
		} else {
			if got.RepoShaIn != "" {
				t.Errorf("job %q: expected empty repo_sha_in, got %q", exp.Name, got.RepoShaIn)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Healing chain fixture builder
// ---------------------------------------------------------------------------

// healingChainFixture provides a pre-wired job chain for testing
// maybeCreateHealingJobs with minimal boilerplate.
type healingChainFixture struct {
	RunID       domaintypes.RunID
	RepoID      domaintypes.RepoID
	SpecID      domaintypes.SpecID
	Jobs        []store.Job
	FailedJob   store.Job
	SuccessorID domaintypes.JobID // ID of the terminal mig-0 job
	Run         store.Run
	Store       *jobStore
}

// priorHealJob describes an intermediate job to insert between pre-gate and mig-0.
type priorHealJob struct {
	Name    string
	JobType domaintypes.JobType
	Status  domaintypes.JobStatus
	Meta    []byte
	ShaIn   string
}

type healingChainConfig struct {
	preGateMeta []byte
	specFn      func(*testing.T) []byte
	repoShaIn   string
	priorHeals  []priorHealJob
	storeOpts   []func(*jobStore)
}

func withHealingMeta(meta []byte) func(*healingChainConfig) {
	return func(c *healingChainConfig) { c.preGateMeta = meta }
}

func withHealingSpec(fn func(*testing.T) []byte) func(*healingChainConfig) {
	return func(c *healingChainConfig) { c.specFn = fn }
}

func withHealingRepoShaIn(sha string) func(*healingChainConfig) {
	return func(c *healingChainConfig) { c.repoShaIn = sha }
}

func withPriorHeals(heals ...priorHealJob) func(*healingChainConfig) {
	return func(c *healingChainConfig) { c.priorHeals = heals }
}

func withHealingStoreOpts(opts ...func(*jobStore)) func(*healingChainConfig) {
	return func(c *healingChainConfig) { c.storeOpts = opts }
}

// newHealingChain builds a chain of jobs: pre-gate → [prior heals] → mig-0,
// wires NextID pointers, and configures a jobStore with the chain and spec.
//
// The "failed" job is the last gate/re-gate type in the chain (pre-gate when
// there are no prior heals, or the last re-gate when there are).
func newHealingChain(t *testing.T, opts ...func(*healingChainConfig)) healingChainFixture {
	t.Helper()

	cfg := healingChainConfig{
		preGateMeta: []byte(`{"kind":"gate","gate":{"recovery":{"loop_kind":"healing","error_kind":"infra","strategy_id":"infra-default"}}}`),
		specFn:      func(t *testing.T) []byte { return buildHealingSpec(t, 2) },
		repoShaIn:   healingTestRepoSHAIn,
	}
	for _, o := range opts {
		o(&cfg)
	}

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	specID := domaintypes.NewSpecID()

	baseJob := func(name string, jt domaintypes.JobType, status domaintypes.JobStatus) store.Job {
		return store.Job{
			ID: domaintypes.NewJobID(), RunID: runID, RepoID: repoID,
			RepoBaseRef: "main", Attempt: 1,
			Name: name, JobType: jt, Status: status,
		}
	}

	var jobs []store.Job

	preGate := baseJob("pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusFail)
	preGate.RepoShaIn = cfg.repoShaIn
	preGate.Meta = cfg.preGateMeta
	jobs = append(jobs, preGate)

	for _, ph := range cfg.priorHeals {
		j := baseJob(ph.Name, ph.JobType, ph.Status)
		j.Meta = ph.Meta
		if ph.ShaIn != "" {
			j.RepoShaIn = ph.ShaIn
		}
		jobs = append(jobs, j)
	}

	mig0 := baseJob("mig-0", domaintypes.JobTypeMig, domaintypes.JobStatusCreated)
	mig0.Meta = []byte(`{}`)
	jobs = append(jobs, mig0)

	// Wire NextID chain.
	for i := 0; i < len(jobs)-1; i++ {
		nextID := jobs[i+1].ID
		jobs[i].NextID = &nextID
	}

	// Failed job = last gate/re-gate in the chain.
	failedIdx := 0
	for i := len(jobs) - 1; i >= 0; i-- {
		if jobs[i].JobType == domaintypes.JobTypePreGate || jobs[i].JobType == domaintypes.JobTypeReGate {
			failedIdx = i
			break
		}
	}

	specBytes := cfg.specFn(t)
	st := &jobStore{}
	st.getSpec.val = store.Spec{ID: specID, Spec: specBytes}
	st.listJobsByRunRepoAttempt.val = jobs
	for _, o := range cfg.storeOpts {
		o(st)
	}

	return healingChainFixture{
		RunID: runID, RepoID: repoID, SpecID: specID,
		Jobs:        jobs,
		FailedJob:   jobs[failedIdx],
		SuccessorID: mig0.ID,
		Run:         store.Run{ID: runID, SpecID: specID, Status: domaintypes.RunStatusStarted},
		Store:       st,
	}
}
