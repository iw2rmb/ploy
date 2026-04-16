package handlers

import (
	"context"
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestCreateJobBuildReGateChild(t *testing.T) {
	t.Parallel()

	f := newRepoScopedFixture(domaintypes.JobTypeMig)
	f.Job.JobImage = "docker.io/test/re-gate:latest"

	st := &jobStore{}
	child, err := createJobBuildReGateChild(context.Background(), st, f.Job, buildKindReGate)
	if err != nil {
		t.Fatalf("create child build: %v", err)
	}
	if child.ID.IsZero() {
		t.Fatal("child id must be set")
	}
	if got := len(st.createJob.calls); got != 1 {
		t.Fatalf("CreateJob calls = %d, want 1", got)
	}

	params := st.createJob.calls[0]
	if params.JobType != domaintypes.JobTypeReGate {
		t.Fatalf("job_type = %q, want %q", params.JobType, domaintypes.JobTypeReGate)
	}
	if params.Status != domaintypes.JobStatusQueued {
		t.Fatalf("status = %q, want %q", params.Status, domaintypes.JobStatusQueued)
	}
	if params.RunID != f.Job.RunID || params.RepoID != f.Job.RepoID || params.Attempt != f.Job.Attempt {
		t.Fatalf("run/repo/attempt mismatch: %+v", params)
	}
	if params.RepoShaIn != f.Job.RepoShaIn {
		t.Fatalf("repo_sha_in = %q, want %q", params.RepoShaIn, f.Job.RepoShaIn)
	}

	var meta childBuildMeta
	if err := json.Unmarshal(params.Meta, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if meta.Kind != "mig" {
		t.Fatalf("meta.kind = %q, want %q", meta.Kind, "mig")
	}
	if meta.Trigger.Kind != childBuildTriggerKind {
		t.Fatalf("meta.trigger.kind = %q, want %q", meta.Trigger.Kind, childBuildTriggerKind)
	}
	if meta.Trigger.ParentJobID != f.JobID {
		t.Fatalf("meta.trigger.parent_job_id = %q, want %q", meta.Trigger.ParentJobID, f.JobID)
	}
}

func TestGetLinkedJobBuildChild(t *testing.T) {
	t.Parallel()

	parentID := domaintypes.NewJobID()
	otherParentID := domaintypes.NewJobID()
	childID := domaintypes.NewJobID()
	parentRunID := domaintypes.NewRunID()
	parentRepoID := domaintypes.NewRepoID()
	parent := store.Job{ID: parentID, RunID: parentRunID, RepoID: parentRepoID, Attempt: 2}

	tests := []struct {
		name      string
		childJob  store.Job
		wantError bool
	}{
		{
			name: "linked re_gate child",
			childJob: store.Job{
				ID:      childID,
				RunID:   parentRunID,
				RepoID:  parentRepoID,
				Attempt: parent.Attempt,
				JobType: domaintypes.JobTypeReGate,
				Status:  domaintypes.JobStatusRunning,
				Meta:    []byte(`{"kind":"mig","trigger":{"kind":"child_gate_request","parent_job_id":"` + parentID.String() + `"}}`),
			},
			wantError: false,
		},
		{
			name: "rejects non re_gate child",
			childJob: store.Job{
				ID:      childID,
				RunID:   parentRunID,
				RepoID:  parentRepoID,
				Attempt: parent.Attempt,
				JobType: domaintypes.JobTypeMig,
				Status:  domaintypes.JobStatusRunning,
				Meta:    []byte(`{"kind":"mig","trigger":{"kind":"child_gate_request","parent_job_id":"` + parentID.String() + `"}}`),
			},
			wantError: true,
		},
		{
			name: "rejects mismatched parent linkage",
			childJob: store.Job{
				ID:      childID,
				RunID:   parentRunID,
				RepoID:  parentRepoID,
				Attempt: parent.Attempt,
				JobType: domaintypes.JobTypeReGate,
				Status:  domaintypes.JobStatusRunning,
				Meta:    []byte(`{"kind":"mig","trigger":{"kind":"child_gate_request","parent_job_id":"` + otherParentID.String() + `"}}`),
			},
			wantError: true,
		},
		{
			name: "rejects malformed metadata",
			childJob: store.Job{
				ID:      childID,
				RunID:   parentRunID,
				RepoID:  parentRepoID,
				Attempt: parent.Attempt,
				JobType: domaintypes.JobTypeReGate,
				Status:  domaintypes.JobStatusRunning,
				Meta:    []byte(`{`),
			},
			wantError: true,
		},
		{
			name: "rejects run repo attempt mismatch",
			childJob: store.Job{
				ID:      childID,
				RunID:   domaintypes.NewRunID(),
				RepoID:  parentRepoID,
				Attempt: parent.Attempt,
				JobType: domaintypes.JobTypeReGate,
				Status:  domaintypes.JobStatusRunning,
				Meta:    []byte(`{"kind":"mig","trigger":{"kind":"child_gate_request","parent_job_id":"` + parentID.String() + `"}}`),
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := &jobStore{
				getJobResults: map[domaintypes.JobID]store.Job{
					childID: tt.childJob,
				},
			}

			_, err := getLinkedJobBuildChild(context.Background(), st, parent, childID)
			if tt.wantError && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
