package runs

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func TestOrderRepoJobsByChain_ReconstructsLinkedOrder(t *testing.T) {
	t.Parallel()

	pre := domaintypes.NewJobID()
	mig0 := domaintypes.NewJobID()
	mig1 := domaintypes.NewJobID()
	post := domaintypes.NewJobID()

	jobs := []RepoJobEntry{
		// Deliberately out of chain order (mirrors current broken render shape).
		{JobID: post, JobType: "post_gate", Status: store.JobStatusCreated, NextID: nil},
		{JobID: mig1, JobType: "mig", Status: store.JobStatusCreated, NextID: &post},
		{JobID: mig0, JobType: "mig", Status: store.JobStatusCreated, NextID: &mig1},
		{JobID: pre, JobType: "pre_gate", Status: store.JobStatusRunning, NextID: &mig0},
	}

	got := orderRepoJobsByChain(jobs)
	if len(got) != 4 {
		t.Fatalf("expected 4 jobs, got %d", len(got))
	}

	if got[0].JobID != pre || got[1].JobID != mig0 || got[2].JobID != mig1 || got[3].JobID != post {
		t.Fatalf("unexpected chain order: got [%s, %s, %s, %s]", got[0].JobID, got[1].JobID, got[2].JobID, got[3].JobID)
	}
}
