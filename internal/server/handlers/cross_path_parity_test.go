package handlers

import (
	"context"
	"errors"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// TestCrossPathParity_StandardJobErrorToChainAction exercises the joint nodeagent→server
// completion path for standard (mig/heal/MR) job error scenarios through concrete production
// code paths.
//
// Nodeagent step: lifecycle.JobStatusFromRunError maps execution errors to job statuses.
// This is the canonical call used in runController.uploadFailureStatus and executeStandardJob.
//
// Server step: CompleteJobService.Complete routes the resulting status through its full
// post-action pipeline (onFail/onCancelled/reconcileRepoRun). Assertions target concrete
// store side-effects rather than re-examining lifecycle function return values in isolation.
func TestCrossPathParity_StandardJobErrorToChainAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		// jobType and hasNext determine the chain action the server should take.
		jobType domaintypes.JobType
		hasNext bool
		// wantCancelSuccessor: cancelRemainingJobsAfterFailure was invoked, producing an
		// UpdateJobStatus call on the queued successor job.
		wantCancelSuccessor bool
	}{
		// context.Canceled: non-gate jobs → Cancelled → onCancelled → CancelRemainder.
		{name: "ctx_canceled/mod/has-next", err: context.Canceled, jobType: domaintypes.JobTypeMod, hasNext: true, wantCancelSuccessor: true},
		{name: "ctx_deadline/heal/has-next", err: context.DeadlineExceeded, jobType: domaintypes.JobTypeHeal, hasNext: true, wantCancelSuccessor: true},
		// MR jobs: Cancelled maps to NoAction — failures do not cascade.
		{name: "ctx_canceled/mr/no-next", err: context.Canceled, jobType: domaintypes.JobTypeMR, hasNext: false, wantCancelSuccessor: false},
		// Runtime errors: non-gate jobs → Fail → onFail → CancelRemainder.
		{name: "runtime_error/mod/has-next", err: errors.New("container exited unexpectedly"), jobType: domaintypes.JobTypeMod, hasNext: true, wantCancelSuccessor: true},
		{name: "runtime_error/heal/has-next", err: errors.New("image pull failed"), jobType: domaintypes.JobTypeHeal, hasNext: true, wantCancelSuccessor: true},
		// MR runtime errors: Fail maps to NoAction.
		{name: "runtime_error/mr/no-next", err: errors.New("git push: authentication failed"), jobType: domaintypes.JobTypeMR, hasNext: false, wantCancelSuccessor: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
			jobID := domaintypes.NewJobID()
			nextID := domaintypes.NewJobID()
			runID := domaintypes.NewRunID()
			repoID := domaintypes.NewRepoID()

			job := store.Job{
				ID:      jobID,
				RunID:   runID,
				RepoID:  repoID,
				NodeID:  &nodeID,
				Status:  domaintypes.JobStatusRunning,
				JobType: tc.jobType,
				Attempt: 1,
			}
			if tc.hasNext {
				job.NextID = &nextID
			}

			// Successor in Queued state: cancelRemainingJobsAfterFailure will issue
			// UpdateJobStatus for it, giving us an observable signal for CancelRemainder.
			successor := store.Job{
				ID:      nextID,
				RunID:   runID,
				RepoID:  repoID,
				Status:  domaintypes.JobStatusQueued,
				JobType: tc.jobType,
				Attempt: 1,
			}

			st := &jobStore{
				getJobResult:                   job,
			}
			st.listJobsByRunRepoAttempt.val = []store.Job{job, successor}

			svc := NewCompleteJobService(st, nil, nil, nil)

			// Nodeagent path: map execution error to job status (mirrors runController.uploadFailureStatus).
			status := lifecycle.JobStatusFromRunError(tc.err)

			// Server path: drive CompleteJobService.Complete with the status emitted by the nodeagent.
			_, err := svc.Complete(context.Background(), CompleteJobInput{
				JobID:      jobID,
				NodeID:     nodeID,
				Status:     status,
				StatsBytes: []byte("{}"),
			})
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}

			// UpdateJobStatus is called by cancelRemainingJobsAfterFailure when a non-terminal
			// successor exists. Its presence or absence locks the CancelRemainder/NoAction branch.
			if tc.wantCancelSuccessor != st.updateJobStatusCalled {
				t.Fatalf("UpdateJobStatus called = %v, want %v (err=%v → status=%s, jobType=%s)",
					st.updateJobStatusCalled, tc.wantCancelSuccessor, tc.err, status, tc.jobType)
			}
			// AdvanceNext is not expected for any error-originated status.
			if st.promoteJobByIDIfUnblockedCalled {
				t.Fatalf("PromoteJobByIDIfUnblocked unexpectedly called (err=%v → status=%s)", tc.err, status)
			}
		})
	}
}

// TestCrossPathParity_GateJobStatusToChainAction exercises the gate-specific server completion
// paths by driving CompleteJobService.Complete with the three status values that gate nodeagent
// paths emit (execution_orchestrator_gate.go).
//
// Gate status assignment is deliberate and explicit — not via lifecycle.JobStatusFromRunError:
//   - Infrastructure errors → Cancelled (prevents healing activation)
//   - Test failures        → Fail      (triggers EvaluateGateFailure / healing evaluation)
//   - Test successes       → Success   (advances the job chain)
//
// This suite locks that CompleteJobService routes each gate status to the correct post-action
// chain, ensuring the server remains consistent with the gate nodeagent's intentional semantics.
func TestCrossPathParity_GateJobStatusToChainAction(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		jobType domaintypes.JobType
		status  domaintypes.JobStatus
		hasNext bool
		// wantGetRunCalled: loadRunForPostCompletion was invoked (uniquely identifies healing path).
		wantGetRunCalled bool
		// wantCancelSuccessor: UpdateJobStatus issued for the queued successor.
		wantCancelSuccessor bool
		// wantAdvanceNext: PromoteJobByIDIfUnblocked was called.
		wantAdvanceNext bool
	}{
		// Gate infra errors always produce Cancelled → CancelRemainder (no healing path entered).
		{
			name: "pre_gate/infra_cancelled/has-next",
			jobType: domaintypes.JobTypePreGate, status: domaintypes.JobStatusCancelled, hasNext: true,
			wantCancelSuccessor: true,
		},
		{
			name: "post_gate/infra_cancelled/has-next",
			jobType: domaintypes.JobTypePostGate, status: domaintypes.JobStatusCancelled, hasNext: true,
			wantCancelSuccessor: true,
		},
		// Gate test failures produce Fail → EvaluateGateFailure → healing path entered (GetRun called).
		// With no job meta the recovery kind defaults to Unknown (terminal), so healing resolves to
		// CancelRemainder internally — both the healing entry point and the cancel side-effect are present.
		{
			name: "pre_gate/test_fail/has-next",
			jobType: domaintypes.JobTypePreGate, status: domaintypes.JobStatusFail, hasNext: true,
			wantGetRunCalled:    true,
			wantCancelSuccessor: true,
		},
		{
			name: "re_gate/test_fail/has-next",
			jobType: domaintypes.JobTypeReGate, status: domaintypes.JobStatusFail, hasNext: true,
			wantGetRunCalled:    true,
			wantCancelSuccessor: true,
		},
		// Gate successes → AdvanceNext when a successor exists.
		{
			name: "pre_gate/success/has-next",
			jobType: domaintypes.JobTypePreGate, status: domaintypes.JobStatusSuccess, hasNext: true,
			wantAdvanceNext: true,
		},
		// Gate success with no successor → NoAction.
		{
			name: "post_gate/success/no-next",
			jobType: domaintypes.JobTypePostGate, status: domaintypes.JobStatusSuccess, hasNext: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())
			jobID := domaintypes.NewJobID()
			nextID := domaintypes.NewJobID()
			runID := domaintypes.NewRunID()
			repoID := domaintypes.NewRepoID()

			const sha40 = "0123456789abcdef0123456789abcdef01234567"

			job := store.Job{
				ID:        jobID,
				RunID:     runID,
				RepoID:    repoID,
				NodeID:    &nodeID,
				Status:    domaintypes.JobStatusRunning,
				JobType:   tc.jobType,
				Attempt:   1,
				RepoShaIn: sha40, // required for Success+NextID chain progression validation
			}
			if tc.hasNext {
				job.NextID = &nextID
			}

			successor := store.Job{
				ID:      nextID,
				RunID:   runID,
				RepoID:  repoID,
				Status:  domaintypes.JobStatusQueued,
				JobType: tc.jobType,
				Attempt: 1,
			}

			st := &jobStore{
				getJobResult:                   job,
			}
			st.listJobsByRunRepoAttempt.val = []store.Job{job, successor}

			svc := NewCompleteJobService(st, nil, nil, nil)

			input := CompleteJobInput{
				JobID:      jobID,
				NodeID:     nodeID,
				Status:     tc.status,
				StatsBytes: []byte("{}"),
			}
			if tc.status == domaintypes.JobStatusSuccess && tc.hasNext {
				input.RepoSHAOut = sha40
			}

			_, err := svc.Complete(context.Background(), input)
			if err != nil {
				t.Fatalf("Complete() error = %v", err)
			}

			// getRun.called uniquely identifies the healing evaluation path (loadRunForPostCompletion).
			if tc.wantGetRunCalled != st.getRun.called {
				t.Fatalf("GetRun called = %v, want %v (jobType=%s status=%s — healing path entered?)",
					st.getRun.called, tc.wantGetRunCalled, tc.jobType, tc.status)
			}

			// updateJobStatusCalled signals that cancelRemainingJobsAfterFailure cancelled a successor.
			if tc.wantCancelSuccessor != st.updateJobStatusCalled {
				t.Fatalf("UpdateJobStatus called = %v, want %v (jobType=%s status=%s — CancelRemainder path?)",
					st.updateJobStatusCalled, tc.wantCancelSuccessor, tc.jobType, tc.status)
			}

			// promoteJobByIDIfUnblockedCalled signals that the AdvanceNext path was taken.
			if tc.wantAdvanceNext != st.promoteJobByIDIfUnblockedCalled {
				t.Fatalf("PromoteJobByIDIfUnblocked called = %v, want %v (jobType=%s status=%s)",
					st.promoteJobByIDIfUnblockedCalled, tc.wantAdvanceNext, tc.jobType, tc.status)
			}
		})
	}
}
