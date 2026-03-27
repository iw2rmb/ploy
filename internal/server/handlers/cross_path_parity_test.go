package handlers

// cross_path_parity_test.go verifies that calling CompleteJobService.Complete
// through the real HTTP handler produces store side effects that are consistent
// with lifecycle.EvaluateCompletionDecision for every (jobType, status, hasNext)
// combination.
//
// This is the server-side counterpart to lifecycle.TestCrossPathTransitionParity.
// While the lifecycle fixture validates the mapping in isolation, this test
// exercises the actual server handler code path so that divergence between the
// lifecycle helper and the real dispatch branches (onFail / onSuccess / onCancelled)
// is caught at the same time.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func TestCompleteJobService_ServerPathChainActionParity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		jobType    domaintypes.JobType
		status     domaintypes.JobStatus
		hasNext    bool
		wantAction lifecycle.CompletionChainAction
	}{
		// ── Mod job ─────────────────────────────────────────────────────────────

		{
			name:       "mod success with next advances chain",
			jobType:    domaintypes.JobTypeMod,
			status:     domaintypes.JobStatusSuccess,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:       "mod success without next takes no action",
			jobType:    domaintypes.JobTypeMod,
			status:     domaintypes.JobStatusSuccess,
			hasNext:    false,
			wantAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:       "mod fail cancels remainder",
			jobType:    domaintypes.JobTypeMod,
			status:     domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:       "mod cancelled cancels remainder",
			jobType:    domaintypes.JobTypeMod,
			status:     domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── Gate job: pre_gate ───────────────────────────────────────────────

		{
			name:       "pre-gate success with next advances chain",
			jobType:    domaintypes.JobTypePreGate,
			status:     domaintypes.JobStatusSuccess,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:       "pre-gate fail evaluates gate failure",
			jobType:    domaintypes.JobTypePreGate,
			status:     domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:       "pre-gate cancelled cancels remainder",
			jobType:    domaintypes.JobTypePreGate,
			status:     domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── Gate job: post_gate ──────────────────────────────────────────────

		{
			name:       "post-gate success with next advances chain",
			jobType:    domaintypes.JobTypePostGate,
			status:     domaintypes.JobStatusSuccess,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:       "post-gate fail evaluates gate failure",
			jobType:    domaintypes.JobTypePostGate,
			status:     domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainEvaluateGateFailure,
		},
		{
			name:       "post-gate cancelled cancels remainder",
			jobType:    domaintypes.JobTypePostGate,
			status:     domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── Heal job ─────────────────────────────────────────────────────────

		{
			name:       "heal success with next advances chain",
			jobType:    domaintypes.JobTypeHeal,
			status:     domaintypes.JobStatusSuccess,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainAdvanceNext,
		},
		{
			name:       "heal fail cancels remainder",
			jobType:    domaintypes.JobTypeHeal,
			status:     domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},
		{
			name:       "heal cancelled cancels remainder",
			jobType:    domaintypes.JobTypeHeal,
			status:     domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainCancelRemainder,
		},

		// ── MR job ───────────────────────────────────────────────────────────

		{
			name:       "MR success takes no chain action",
			jobType:    domaintypes.JobTypeMR,
			status:     domaintypes.JobStatusSuccess,
			hasNext:    false,
			wantAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:       "MR fail takes no chain action",
			jobType:    domaintypes.JobTypeMR,
			status:     domaintypes.JobStatusFail,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainNoAction,
		},
		{
			name:       "MR cancelled takes no chain action",
			jobType:    domaintypes.JobTypeMR,
			status:     domaintypes.JobStatusCancelled,
			hasNext:    true,
			wantAction: lifecycle.CompletionChainNoAction,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Pin the lifecycle mapping used by the server dispatch to the expected
			// action so that test breakage pinpoints which layer diverged.
			decision := lifecycle.EvaluateCompletionDecision(tc.jobType, tc.status, tc.hasNext)
			if decision.ChainAction != tc.wantAction {
				t.Fatalf(
					"lifecycle contract mismatch: EvaluateCompletionDecision(jobType=%v, status=%v, hasNext=%v) = %v, want %v",
					tc.jobType, tc.status, tc.hasNext, decision.ChainAction, tc.wantAction,
				)
			}

			repoID := domaintypes.NewRepoID()
			f := newJobFixture(tc.jobType, 0)
			f.Job.RepoID = repoID
			f.Job.RepoBaseRef = "main"
			f.Job.Attempt = 1

			var nextJobID domaintypes.JobID
			var nextJob store.Job
			if tc.hasNext {
				nextJobID = domaintypes.NewJobID()
				f.Job.NextID = &nextJobID
				nextJob = store.Job{
					ID:          nextJobID,
					RunID:       f.RunID,
					RepoID:      repoID,
					RepoBaseRef: "main",
					Attempt:     1,
					Status:      domaintypes.JobStatusCreated,
					JobType:     domaintypes.JobTypeMod,
				}
			}

			allJobs := []store.Job{f.Job}
			if tc.hasNext {
				allJobs = append(allJobs, nextJob)
			}

			reqBody := map[string]any{
				"status": tc.status.String(),
			}
			if tc.status == domaintypes.JobStatusSuccess && tc.hasNext {
				reqBody["repo_sha_out"] = "0123456789abcdef0123456789abcdef01234567"
			}

			st := &mockStore{
				getJobResult:                    f.Job,
				getRunResult:                    store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
				listJobsByRunResult:             allJobs,
				listJobsByRunRepoAttemptResult:  allJobs,
				promoteJobByIDIfUnblockedResult: nextJob,
			}

			handler := completeJobHandler(st, nil, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, f.completeJobReq(reqBody))

			if rr.Code != http.StatusNoContent {
				t.Fatalf("handler returned %d: %s", rr.Code, rr.Body.String())
			}

			// Assert store side effects match the chain action prescribed by the
			// lifecycle helper that the server dispatch calls internally.
			switch tc.wantAction {

			case lifecycle.CompletionChainAdvanceNext:
				if !st.promoteJobByIDIfUnblockedCalled {
					t.Fatal("CompletionChainAdvanceNext: expected PromoteJobByIDIfUnblocked to be called")
				}
				if st.promoteJobByIDIfUnblockedParam != nextJobID {
					t.Fatalf("CompletionChainAdvanceNext: expected promotion of %s, got %s", nextJobID, st.promoteJobByIDIfUnblockedParam)
				}
				if st.updateJobStatusCalled {
					for _, call := range st.updateJobStatusCalls {
						if call.Status == domaintypes.JobStatusCancelled {
							t.Fatal("CompletionChainAdvanceNext: did not expect remainder cancellation")
						}
					}
				}

			case lifecycle.CompletionChainNoAction:
				if st.promoteJobByIDIfUnblockedCalled {
					t.Fatal("CompletionChainNoAction: did not expect PromoteJobByIDIfUnblocked")
				}
				for _, call := range st.updateJobStatusCalls {
					if call.Status == domaintypes.JobStatusCancelled {
						t.Fatalf("CompletionChainNoAction: did not expect remainder cancellation, got call for job %s", call.ID)
					}
				}

			case lifecycle.CompletionChainCancelRemainder:
				if st.promoteJobByIDIfUnblockedCalled {
					t.Fatal("CompletionChainCancelRemainder: did not expect PromoteJobByIDIfUnblocked")
				}
				if tc.hasNext {
					if !st.updateJobStatusCalled {
						t.Fatal("CompletionChainCancelRemainder: expected UpdateJobStatus to cancel remaining jobs")
					}
					foundCancel := false
					for _, call := range st.updateJobStatusCalls {
						if call.ID == nextJob.ID && call.Status == domaintypes.JobStatusCancelled {
							foundCancel = true
						}
					}
					if !foundCancel {
						t.Fatalf("CompletionChainCancelRemainder: expected next job %s to be cancelled, calls: %v", nextJob.ID, st.updateJobStatusCalls)
					}
				}

			case lifecycle.CompletionChainEvaluateGateFailure:
				// Gate failure enters maybeCreateHealingJobs. Depending on whether a
				// healable spec is present, that function either creates healing jobs or
				// cancels the remaining chain via terminal recovery classification. Both
				// outcomes are correct. The key distinction from CompletionChainAdvanceNext
				// is that chain promotion must NOT happen.
				if st.promoteJobByIDIfUnblockedCalled {
					t.Fatal("CompletionChainEvaluateGateFailure: did not expect PromoteJobByIDIfUnblocked")
				}
			}
		})
	}
}
