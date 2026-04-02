package recovery

import (
	"encoding/json"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestReconcileRepo_EvaluateRepoAttemptTerminalStatus(t *testing.T) {
	t.Parallel()

	mkMeta := func(nextID int64) []byte {
		b, _ := json.Marshal(map[string]any{"next_id": nextID})
		return b
	}

	cases := []struct {
		name       string
		jobs       []store.Job
		wantUpdate bool
		wantStatus domaintypes.RunRepoStatus
		wantErr    bool
	}{
		{
			name: "non terminal job blocks update",
			jobs: []store.Job{
				{ID: domaintypes.NewJobID(), JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusRunning},
			},
			wantUpdate: false,
		},
		{
			name: "uses highest next_id terminal job",
			jobs: []store.Job{
				{ID: domaintypes.NewJobID(), JobType: domaintypes.JobTypePreGate, Status: domaintypes.JobStatusFail, Meta: mkMeta(1000)},
				{ID: domaintypes.NewJobID(), JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusSuccess, Meta: mkMeta(2000)},
			},
			wantUpdate: true,
			wantStatus: domaintypes.RunRepoStatusSuccess,
		},
		{
			name: "ignores MR jobs",
			jobs: []store.Job{
				{ID: domaintypes.NewJobID(), JobType: domaintypes.JobTypeMR, Status: domaintypes.JobStatusSuccess},
			},
			wantUpdate: false,
		},
		{
			name: "error status maps repo to fail",
			jobs: []store.Job{
				{ID: domaintypes.NewJobID(), JobType: domaintypes.JobTypeMig, Status: domaintypes.JobStatusError},
			},
			wantUpdate: true,
			wantStatus: domaintypes.RunRepoStatusFail,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			eval, err := EvaluateRepoAttemptTerminalStatus(tc.jobs)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr=%v", err, tc.wantErr)
			}
			if eval.ShouldUpdate != tc.wantUpdate {
				t.Fatalf("ShouldUpdate = %v, want %v", eval.ShouldUpdate, tc.wantUpdate)
			}
			if tc.wantUpdate && eval.Status != tc.wantStatus {
				t.Fatalf("Status = %q, want %q", eval.Status, tc.wantStatus)
			}
		})
	}
}
