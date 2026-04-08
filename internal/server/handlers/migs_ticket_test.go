package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

const testRepoSHA0 = "0123456789abcdef0123456789abcdef01234567"

func TestCreateSingleRepoRunHandler_SingleRepo(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &jobStore{
		createRunResult: store.Run{
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	handler := createSingleRepoRunHandler(st, nil)
	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())

	assertStatus(t, rr, http.StatusCreated)

	resp := decodeBody[struct {
		RunID  string `json:"run_id"`
		MigID  string `json:"mig_id"`
		SpecID string `json:"spec_id"`
	}](t, rr)

	if resp.RunID == "" {
		t.Fatal("expected run_id to be set")
	}
	if resp.MigID == "" {
		t.Fatal("expected mig_id to be set")
	}
	if resp.SpecID == "" {
		t.Fatal("expected spec_id to be set")
	}

	if !st.createSpecCalled || !st.createMigCalled || !st.createMigRepoCalled || !st.createRunCalled || !st.createRunRepoCalled {
		t.Fatal("expected spec/mig/repo/run creation calls to be made")
	}
	if len(st.createJob.calls) != 0 {
		t.Fatalf("expected no jobs on submission, got %d", len(st.createJob.calls))
	}
}

func TestCreateJobsFromSpec(t *testing.T) {
	t.Parallel()
	const (
		hookHashA = "a1b2c3d4e5f6"
		hookHashB = "b1c2d3e4f5a6"
	)

	tests := []struct {
		name        string
		runID       domaintypes.RunID
		repoID      domaintypes.RepoID
		repoBaseRef string
		attempt     int32
		repoSHA0    string
		spec        []byte
		expected    []expectedJob
		wantErr     string
		useHashHook bool
	}{
		{
			name:        "SingleMig",
			runID:       domaintypes.RunID("run_test_12345678901234567"),
			repoID:      domaintypes.RepoID("repo_abc"),
			repoBaseRef: "main",
			attempt:     1,
			repoSHA0:    testRepoSHA0,
			spec:        []byte(`{"steps":[{"image":"mig1:v1"}]}`),
			expected: []expectedJob{
				{"pre-gate-sbom", domaintypes.JobTypeSBOM, domaintypes.JobStatusQueued, "", testRepoSHA0},
				{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusCreated, "", ""},
				{"mig-0", domaintypes.JobTypeMig, domaintypes.JobStatusCreated, "", ""},
				{"post-gate-sbom", domaintypes.JobTypeSBOM, domaintypes.JobStatusCreated, "", ""},
				{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated, "", ""},
			},
		},
		{
			name:        "MultiStep",
			runID:       domaintypes.RunID("run_multistep_0123456789"),
			repoID:      domaintypes.RepoID("repo_multi"),
			repoBaseRef: "develop",
			attempt:     2,
			repoSHA0:    testRepoSHA0,
			spec:        []byte(`{"steps":[{"image":"mig1:v1"},{"image":"mig2:v2"},{"image":"mig3:v3"}]}`),
			expected: []expectedJob{
				{"pre-gate-sbom", domaintypes.JobTypeSBOM, domaintypes.JobStatusQueued, "", testRepoSHA0},
				{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusCreated, "", ""},
				{"mig-0", domaintypes.JobTypeMig, domaintypes.JobStatusCreated, "mig1:v1", ""},
				{"mig-1", domaintypes.JobTypeMig, domaintypes.JobStatusCreated, "mig2:v2", ""},
				{"mig-2", domaintypes.JobTypeMig, domaintypes.JobStatusCreated, "mig3:v3", ""},
				{"post-gate-sbom", domaintypes.JobTypeSBOM, domaintypes.JobStatusCreated, "", ""},
				{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated, "", ""},
			},
		},
		{
			name:        "CustomAttemptAndRef",
			runID:       domaintypes.RunID("run_v1_direct_addressing_12"),
			repoID:      domaintypes.RepoID("repo_direct_addr"),
			repoBaseRef: "feature/test",
			attempt:     3,
			repoSHA0:    testRepoSHA0,
			spec:        []byte(`{"steps":[{"image":"a"}]}`),
			expected: []expectedJob{
				{"pre-gate-sbom", domaintypes.JobTypeSBOM, domaintypes.JobStatusQueued, "", testRepoSHA0},
				{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusCreated, "", ""},
				{"mig-0", domaintypes.JobTypeMig, domaintypes.JobStatusCreated, "", ""},
				{"post-gate-sbom", domaintypes.JobTypeSBOM, domaintypes.JobStatusCreated, "", ""},
				{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated, "", ""},
			},
		},
		{
			name:        "WithHooks",
			runID:       domaintypes.RunID("run_hooks_123456789012345"),
			repoID:      domaintypes.RepoID("repo_hooks"),
			repoBaseRef: "main",
			attempt:     1,
			repoSHA0:    testRepoSHA0,
			spec:        []byte(`{"hooks":["` + hookHashA + `","` + hookHashB + `"],"bundle_map":{"` + hookHashA + `":"bundle_hooks","` + hookHashB + `":"bundle_hooks"},"steps":[{"image":"mig1:v1"}]}`),
			expected: []expectedJob{
				{"pre-gate-sbom", domaintypes.JobTypeSBOM, domaintypes.JobStatusQueued, "", testRepoSHA0},
				{"pre-gate-hook-000", domaintypes.JobTypeHook, domaintypes.JobStatusCreated, "", ""},
				{"pre-gate-hook-001", domaintypes.JobTypeHook, domaintypes.JobStatusCreated, "", ""},
				{"pre-gate", domaintypes.JobTypePreGate, domaintypes.JobStatusCreated, "", ""},
				{"mig-0", domaintypes.JobTypeMig, domaintypes.JobStatusCreated, "", ""},
				{"post-gate-sbom", domaintypes.JobTypeSBOM, domaintypes.JobStatusCreated, "", ""},
				{"post-gate-hook-000", domaintypes.JobTypeHook, domaintypes.JobStatusCreated, "", ""},
				{"post-gate-hook-001", domaintypes.JobTypeHook, domaintypes.JobStatusCreated, "", ""},
				{"post-gate", domaintypes.JobTypePostGate, domaintypes.JobStatusCreated, "", ""},
			},
			useHashHook: true,
		},
		{
			name:        "InvalidRepoSHA0",
			runID:       domaintypes.RunID("run_123"),
			repoID:      domaintypes.RepoID("repo_456"),
			repoBaseRef: "main",
			attempt:     1,
			repoSHA0:    "not-a-sha",
			spec:        []byte(`{"steps":[{"image":"a"}]}`),
			wantErr:     "repo_sha0 must match",
		},
		{
			name:        "RejectsRawLocalHookSources",
			runID:       domaintypes.RunID("run_raw_hook_reject_123456"),
			repoID:      domaintypes.RepoID("repo_raw_hook"),
			repoBaseRef: "main",
			attempt:     1,
			repoSHA0:    testRepoSHA0,
			spec:        []byte(`{"hooks":["../../hooks"],"steps":[{"image":"a"}]}`),
			wantErr:     "local hook sources must be precompiled by CLI into hash entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &jobStore{}
			var hookBS *bsmock.Store
			if tt.useHashHook {
				hookBS = bsmock.New()
				seedPlanningHookBundle(t, st, hookBS, "bundle_hooks", `
id: hook-bundle
steps:
  - image: hook:latest
`)
			}
			repoSHA0 := tt.repoSHA0
			if repoSHA0 == "" {
				repoSHA0 = testRepoSHA0
			}

			err := createJobsFromSpec(context.Background(), st, tt.runID, tt.repoID, tt.repoBaseRef, tt.attempt, repoSHA0, tt.spec, hookBS)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("createJobsFromSpec failed: %v", err)
			}

			assertJobChain(t, st.createJob.calls, tt.runID, tt.repoID, tt.repoBaseRef, tt.attempt, tt.expected)
		})
	}
}

func TestJobQueueingRules_FirstJobQueued(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		spec         []byte
		expectedJobs int
	}{
		{"single_mod", []byte(`{"steps":[{"image":"a"}]}`), 5},
		{"two_migs", []byte(`{"steps":[{"image":"a"},{"image":"b"}]}`), 6},
		{"five_migs", []byte(`{"steps":[{"image":"a"},{"image":"b"},{"image":"c"},{"image":"d"},{"image":"e"}]}`), 9},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			st := &jobStore{}

			err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.RepoID("repo_456"), "main", 1, testRepoSHA0, tc.spec)
			if err != nil {
				t.Fatalf("createJobsFromSpec failed: %v", err)
			}

			if len(st.createJob.calls) != tc.expectedJobs {
				t.Fatalf("expected %d jobs, got %d", tc.expectedJobs, len(st.createJob.calls))
			}

			byName := createJobsByName(st.createJob.calls)
			if byName["pre-gate-sbom"].Status != domaintypes.JobStatusQueued {
				t.Errorf("expected pre-gate-sbom to be Queued, got %s", byName["pre-gate-sbom"].Status)
			}

			for _, p := range st.createJob.calls {
				if p.Name != "pre-gate-sbom" && p.Status != domaintypes.JobStatusCreated {
					t.Errorf("job %q: expected status Created, got %s", p.Name, p.Status)
				}
			}
		})
	}
}

func TestCreateJobsFromSpec_ChainIntegrity(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	spec := []byte(`{"steps":[{"image":"a"},{"image":"b"}]}`)

	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.RepoID("repo_456"), "main", 1, testRepoSHA0, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	// Verify next_id chain ordering.
	byName := createJobsByName(st.createJob.calls)
	preGateSBOM := byName["pre-gate-sbom"]
	preGate := byName["pre-gate"]
	mig0 := byName["mig-0"]
	mig1 := byName["mig-1"]
	postGateSBOM := byName["post-gate-sbom"]
	postGate := byName["post-gate"]

	if preGateSBOM.NextID == nil || *preGateSBOM.NextID != preGate.ID {
		t.Fatalf("pre-gate-sbom next_id = %v, want %s", preGateSBOM.NextID, preGate.ID)
	}
	if preGate.NextID == nil || *preGate.NextID != mig0.ID {
		t.Fatalf("pre-gate next_id = %v, want %s", preGate.NextID, mig0.ID)
	}
	if mig0.NextID == nil || *mig0.NextID != mig1.ID {
		t.Fatalf("mig-0 next_id = %v, want %s", mig0.NextID, mig1.ID)
	}
	if mig1.NextID == nil || *mig1.NextID != postGateSBOM.ID {
		t.Fatalf("mig-1 next_id = %v, want %s", mig1.NextID, postGateSBOM.ID)
	}
	if postGateSBOM.NextID == nil || *postGateSBOM.NextID != postGate.ID {
		t.Fatalf("post-gate-sbom next_id = %v, want %s", postGateSBOM.NextID, postGate.ID)
	}
	if postGate.NextID != nil {
		t.Fatalf("post-gate next_id = %s, want nil", *postGate.NextID)
	}

	// Verify insert order satisfies immediate next_id FK constraint.
	inserted := make(map[domaintypes.JobID]struct{}, len(st.createJob.calls))
	for i, p := range st.createJob.calls {
		if p.NextID != nil {
			if _, ok := inserted[*p.NextID]; !ok {
				t.Fatalf("insert %d (%s) references next_id %s before it was inserted", i, p.Name, *p.NextID)
			}
		}
		inserted[p.ID] = struct{}{}
	}
}

func TestCreateJobsFromSpec_PostGatePreludeWithHooks_DeterministicOrder(t *testing.T) {
	t.Parallel()

	const (
		hookHashA = "aa11bb22cc33"
		hookHashB = "dd44ee55ff66"
	)

	st := &jobStore{}
	bs := bsmock.New()
	seedPlanningHookBundle(t, st, bs, "bundle_post_gate", `
id: hook-bundle
steps:
  - image: hook:latest
`)
	spec := []byte(`{"hooks":["` + hookHashA + `","` + hookHashB + `"],"bundle_map":{"` + hookHashA + `":"bundle_post_gate","` + hookHashB + `":"bundle_post_gate"},"steps":[{"image":"a"},{"image":"b"}]}`)

	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_123"), domaintypes.RepoID("repo_456"), "main", 1, testRepoSHA0, spec, bs)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	byName := createJobsByName(st.createJob.calls)
	preSBOM := byName["pre-gate-sbom"]
	preHook0 := byName["pre-gate-hook-000"]
	preHook1 := byName["pre-gate-hook-001"]
	preGate := byName["pre-gate"]
	mig1 := byName["mig-1"]
	postSBOM := byName["post-gate-sbom"]
	postHook0 := byName["post-gate-hook-000"]
	postHook1 := byName["post-gate-hook-001"]
	postGate := byName["post-gate"]

	if preSBOM.NextID == nil || *preSBOM.NextID != preHook0.ID {
		t.Fatalf("pre-gate-sbom next_id = %v, want %s", preSBOM.NextID, preHook0.ID)
	}
	if preHook0.NextID == nil || *preHook0.NextID != preHook1.ID {
		t.Fatalf("pre-gate-hook-000 next_id = %v, want %s", preHook0.NextID, preHook1.ID)
	}
	if preHook1.NextID == nil || *preHook1.NextID != preGate.ID {
		t.Fatalf("pre-gate-hook-001 next_id = %v, want %s", preHook1.NextID, preGate.ID)
	}
	if mig1.NextID == nil || *mig1.NextID != postSBOM.ID {
		t.Fatalf("mig-1 next_id = %v, want %s", mig1.NextID, postSBOM.ID)
	}
	if postSBOM.NextID == nil || *postSBOM.NextID != postHook0.ID {
		t.Fatalf("post-gate-sbom next_id = %v, want %s", postSBOM.NextID, postHook0.ID)
	}
	if postHook0.NextID == nil || *postHook0.NextID != postHook1.ID {
		t.Fatalf("post-gate-hook-000 next_id = %v, want %s", postHook0.NextID, postHook1.ID)
	}
	if postHook1.NextID == nil || *postHook1.NextID != postGate.ID {
		t.Fatalf("post-gate-hook-001 next_id = %v, want %s", postHook1.NextID, postGate.ID)
	}
	if postGate.NextID != nil {
		t.Fatalf("post-gate next_id = %v, want nil", *postGate.NextID)
	}
}

func TestCreateJobsFromSpec_ConditionalHooks_MixedCycleMatches(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/hook-java.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
id: hook-java
stack:
  language: java
steps:
  - image: hook:latest
`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	st := &jobStore{}
	spec := []byte(fmt.Sprintf(
		`{"hooks":["%s/hook-java.yaml"],"steps":[{"image":"a"}],"build_gate":{"pre":{"stack":{"enabled":true,"language":"java","release":"17"}},"post":{"stack":{"enabled":true,"language":"go","release":"1.22"}}}}`,
		server.URL,
	))
	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_conditional_123"), domaintypes.RepoID("repo_conditional"), "main", 1, testRepoSHA0, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	byName := createJobsByName(st.createJob.calls)
	preHook, ok := byName["pre-gate-hook-000"]
	if !ok {
		t.Fatal("expected pre-gate-hook-000 to be planned")
	}
	if _, exists := byName["post-gate-hook-000"]; exists {
		t.Fatal("did not expect post-gate-hook-000 when matcher returns false for post-gate cycle")
	}

	preSBOM := byName["pre-gate-sbom"]
	preGate := byName["pre-gate"]
	mig0 := byName["mig-0"]
	postSBOM := byName["post-gate-sbom"]
	postGate := byName["post-gate"]

	if preSBOM.NextID == nil || *preSBOM.NextID != preHook.ID {
		t.Fatalf("pre-gate-sbom next_id = %v, want %s", preSBOM.NextID, preHook.ID)
	}
	if preHook.NextID == nil || *preHook.NextID != preGate.ID {
		t.Fatalf("pre-gate-hook-000 next_id = %v, want %s", preHook.NextID, preGate.ID)
	}
	if mig0.NextID == nil || *mig0.NextID != postSBOM.ID {
		t.Fatalf("mig-0 next_id = %v, want %s", mig0.NextID, postSBOM.ID)
	}
	if postSBOM.NextID == nil || *postSBOM.NextID != postGate.ID {
		t.Fatalf("post-gate-sbom next_id = %v, want %s", postSBOM.NextID, postGate.ID)
	}

	meta, err := contracts.UnmarshalJobMeta(preHook.Meta)
	if err != nil {
		t.Fatalf("unmarshal pre-gate hook meta: %v", err)
	}
	if got, want := meta.HookSource, server.URL+"/hook-java.yaml"; got != want {
		t.Fatalf("hook_source=%q, want %q", got, want)
	}
	if !strings.Contains(meta.ActionSummary, "eval=planned") || !strings.Contains(meta.ActionSummary, "should_run=true") {
		t.Fatalf("expected planned matcher summary in action_summary, got %q", meta.ActionSummary)
	}
}

func TestCreateJobsFromSpec_ConditionalHooks_AllFalseCreatesNoHooks(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/hook-ruby.yaml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
id: hook-ruby
stack:
  language: ruby
steps:
  - image: hook:latest
`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	st := &jobStore{}
	spec := []byte(fmt.Sprintf(
		`{"hooks":["%s/hook-ruby.yaml"],"steps":[{"image":"a"}],"build_gate":{"pre":{"stack":{"enabled":true,"language":"java","release":"17"}},"post":{"stack":{"enabled":true,"language":"go","release":"1.22"}}}}`,
		server.URL,
	))
	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_conditional_false_123"), domaintypes.RepoID("repo_conditional_false"), "main", 1, testRepoSHA0, spec)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	if len(st.createJob.calls) != 5 {
		t.Fatalf("expected 5 jobs (no hooks), got %d", len(st.createJob.calls))
	}
	for _, created := range st.createJob.calls {
		if strings.Contains(created.Name, "-hook-") {
			t.Fatalf("did not expect hook job %q when all matcher decisions are false", created.Name)
		}
	}
}

func TestCreateJobsFromSpec_ConditionalHashHooks_AllFalseCreatesNoHooks(t *testing.T) {
	t.Parallel()

	const hookHash = "aa11bb22cc33"
	st := &jobStore{}
	bs := bsmock.New()
	seedPlanningHookBundle(t, st, bs, "bundle_hash_false", `
id: hook-ruby
stack:
  language: ruby
steps:
  - image: hook:latest
`)
	spec := []byte(`{"hooks":["` + hookHash + `"],"bundle_map":{"` + hookHash + `":"bundle_hash_false"},"steps":[{"image":"a"}],"build_gate":{"pre":{"stack":{"enabled":true,"language":"java","release":"17"}},"post":{"stack":{"enabled":true,"language":"go","release":"1.22"}}}}`)

	err := createJobsFromSpec(context.Background(), st, domaintypes.RunID("run_conditional_hash_false_123"), domaintypes.RepoID("repo_conditional_hash_false"), "main", 1, testRepoSHA0, spec, bs)
	if err != nil {
		t.Fatalf("createJobsFromSpec failed: %v", err)
	}

	if len(st.createJob.calls) != 5 {
		t.Fatalf("expected 5 jobs (no hooks), got %d", len(st.createJob.calls))
	}
	for _, created := range st.createJob.calls {
		if strings.Contains(created.Name, "-hook-") {
			t.Fatalf("did not expect hook job %q when all matcher decisions are false", created.Name)
		}
	}
}

func TestCreateJobsFromSpec_MissingBundleBlobFailsBeforeQueueingJobs(t *testing.T) {
	t.Parallel()

	const (
		hookHash = "deadc0de1234"
		bundleID = "bundle_missing_blob"
	)

	st := &jobStore{}
	bs := bsmock.New() // intentionally empty; metadata exists but blob is missing
	objKey := "spec_bundles/" + bundleID + "/bundle.tar.gz"
	st.getSpecBundle.val = store.SpecBundle{
		ID:        bundleID,
		ObjectKey: &objKey,
	}

	spec := []byte(`{"hooks":["` + hookHash + `"],"bundle_map":{"` + hookHash + `":"` + bundleID + `"},"steps":[{"image":"a"}]}`)
	err := createJobsFromSpec(
		context.Background(),
		st,
		domaintypes.RunID("run_missing_bundle_blob_123"),
		domaintypes.RepoID("repo_missing_bundle_blob"),
		"main",
		1,
		testRepoSHA0,
		spec,
		bs,
	)
	if err == nil {
		t.Fatal("expected createJobsFromSpec to fail when hook bundle blob is missing")
	}
	want := `spec bundle "bundle_missing_blob" blob is missing from object storage`
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected error to contain %q, got %v", want, err)
	}
	if len(st.createJob.calls) != 0 {
		t.Fatalf("expected no jobs queued when hook bundle preflight fails, got %d", len(st.createJob.calls))
	}
}

func TestCreateSingleRepoRunHandler_ValidationErrors(t *testing.T) {
	t.Parallel()

	st := &jobStore{}
	handler := createSingleRepoRunHandler(st, nil)

	tests := []struct {
		name       string
		body       any
		wantSubstr string
	}{
		{"empty repo_url", validRunRequestBodyWith(map[string]any{"repo_url": ""}), "empty"},
		{"no repo_url", validRunRequestBodyWithout("repo_url"), "empty"},
		{"empty base_ref", validRunRequestBodyWith(map[string]any{"base_ref": ""}), "empty"},
		{"no base_ref", validRunRequestBodyWithout("base_ref"), "empty"},
		{"empty target_ref", validRunRequestBodyWith(map[string]any{"target_ref": ""}), "empty"},
		{"no target_ref", validRunRequestBodyWithout("target_ref"), "empty"},
		{"no spec", validRunRequestBodyWithout("spec"), "spec is required"},
		{"invalid JSON", "not json", "invalid request"},
		{"http scheme repo_url", validRunRequestBodyWith(map[string]any{"repo_url": "http://github.com/user/repo.git"}), "invalid repo url"},
		{"git scheme repo_url", validRunRequestBodyWith(map[string]any{"repo_url": "git://github.com/user/repo.git"}), "invalid repo url"},
		{"no scheme repo_url", validRunRequestBodyWith(map[string]any{"repo_url": "github.com/user/repo.git"}), "invalid repo url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := doRequest(t, handler, http.MethodPost, "/v1/runs", tt.body)
			assertStatus(t, rr, http.StatusBadRequest)
			assertBodyContains(t, rr, tt.wantSubstr)
		})
	}
}

func TestCreateSingleRepoRunHandler_PublishesEvent(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	st := &jobStore{
		createRunResult: store.Run{
			Status:    domaintypes.RunStatusStarted,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}

	eventsService, _ := createTestEventsService()
	handler := createSingleRepoRunHandler(st, eventsService)

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBody())
	assertStatus(t, rr, http.StatusCreated)

	resp := decodeBody[struct {
		RunID string `json:"run_id"`
	}](t, rr)
	runID := resp.RunID

	snapshot := eventsService.Hub().Snapshot(domaintypes.RunID(runID))
	if len(snapshot) == 0 {
		t.Fatal("expected at least one run event to be published")
	}

	foundRunEvent := false
	for _, evt := range snapshot {
		if evt.Type == domaintypes.SSEEventRun {
			foundRunEvent = true
			if !strings.Contains(string(evt.Data), "\"state\":\"running\"") {
				t.Fatalf("expected run event data to contain state \"running\", got: %s", string(evt.Data))
			}
			break
		}
	}
	if !foundRunEvent {
		t.Fatal("expected to find a 'run' event in the snapshot")
	}
}

func TestGetRunStatusHandler(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	runIDStr := runID.String()
	jobID := domaintypes.NewJobID()
	jobIDStr := jobID.String()
	nextJobID := domaintypes.NewJobID()
	now := time.Now().UTC()

	tests := []struct {
		name       string
		setupStore func() *jobStore
		reqRunID   string
		wantStatus int
		verify     func(t *testing.T, st *jobStore, rr *httptest.ResponseRecorder)
	}{
		{
			name: "success",
			setupStore: func() *jobStore {
				st := &jobStore{
					listJobsByRunResult: []store.Job{
						{ID: jobID, RunID: runID, Status: domaintypes.JobStatusQueued, NextID: &nextJobID, Meta: withNextIDMeta([]byte(`{}`), float64(1000))},
					},
				}
				st.getRun.val = store.Run{
					ID:        runID,
					Status:    domaintypes.RunStatusStarted,
					CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
				}
				st.listRunReposWithURLByRun.val = []store.ListRunReposWithURLByRunRow{
					{
						RunID:         runID,
						RepoID:        "repo_123",
						RepoBaseRef:   "main",
						RepoTargetRef: "feature",
						RepoUrl:       "https://github.com/user/repo.git",
					},
				}
				return st
			},
			reqRunID:   runIDStr,
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *jobStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				resp := decodeBody[migsapi.RunSummary](t, rr)
				if resp.RunID.String() != runIDStr {
					t.Fatalf("expected run_id %s, got %s", runIDStr, resp.RunID.String())
				}
				if resp.State != migsapi.RunStateRunning {
					t.Fatalf("expected status running, got %s", resp.State)
				}
				if resp.Repository != "https://github.com/user/repo.git" {
					t.Fatalf("expected repo_url https://github.com/user/repo.git, got %s", resp.Repository)
				}
				if resp.Metadata["repo_base_ref"] != "main" {
					t.Fatalf("expected base_ref main, got %s", resp.Metadata["repo_base_ref"])
				}
				if resp.Metadata["repo_target_ref"] != "feature" {
					t.Fatalf("expected target_ref feature, got %s", resp.Metadata["repo_target_ref"])
				}
				if len(resp.Stages) != 1 {
					t.Fatalf("expected 1 stage, got %d", len(resp.Stages))
				}
				if got := resp.Stages[domaintypes.JobID(jobIDStr)].State; got != migsapi.StageStatePending {
					t.Fatalf("expected stage to be pending, got %s", got)
				}
				if got := resp.Stages[domaintypes.JobID(jobIDStr)].NextID; got == nil || *got != nextJobID {
					t.Fatalf("expected stage next_id %s, got %v", nextJobID, got)
				}
				assertCalled(t, "GetRun", st.getRun.called)
				assertCalled(t, "ListRunReposWithURLByRun", st.listRunReposWithURLByRun.called)
				assertCalled(t, "ListJobsByRun", st.listJobsByRunCalled)
			},
		},
		{
			name: "not found",
			setupStore: func() *jobStore {
				st := &jobStore{}
				st.getRun.err = pgx.ErrNoRows
				return st
			},
			reqRunID:   domaintypes.NewRunID().String(),
			wantStatus: http.StatusNotFound,
			verify: func(t *testing.T, _ *jobStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertBodyContains(t, rr, "not found")
			},
		},
		{
			name: "empty ID",
			setupStore: func() *jobStore {
				return &jobStore{}
			},
			reqRunID:   "",
			wantStatus: http.StatusBadRequest,
			verify: func(t *testing.T, _ *jobStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertBodyContains(t, rr, "path parameter is required")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := tt.setupStore()
			handler := getRunStatusHandler(st)
			path := "/v1/runs/" + tt.reqRunID + "/status"
			rr := doRequest(t, handler, http.MethodGet, path, nil, "id", tt.reqRunID)
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, st, rr)
			}
		})
	}
}
