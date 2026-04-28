package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestResolveDetectedStackExpectationFromJobs_UsesNearestUpstreamGate(t *testing.T) {
	t.Parallel()

	preGateID := domaintypes.NewJobID()
	mig1ID := domaintypes.NewJobID()
	reGateID := domaintypes.NewJobID()
	mig2ID := domaintypes.NewJobID()

	jobs := []store.Job{
		{
			ID:      preGateID,
			JobType: domaintypes.JobTypePreGate,
			Status:  domaintypes.JobStatusSuccess,
			NextID:  &mig1ID,
			Meta:    mustMarshalGateMeta(t, "java", "maven", "17"),
		},
		{
			ID:      mig1ID,
			JobType: domaintypes.JobTypeMig,
			Status:  domaintypes.JobStatusSuccess,
			NextID:  &reGateID,
		},
		{
			ID:      reGateID,
			JobType: domaintypes.JobTypePostGate,
			Status:  domaintypes.JobStatusFail,
			NextID:  &mig2ID,
			Meta:    mustMarshalGateMeta(t, "java", "gradle", "21"),
		},
		{
			ID:      mig2ID,
			JobType: domaintypes.JobTypeMig,
			Status:  domaintypes.JobStatusSuccess,
		},
	}

	got := resolveDetectedStackExpectationFromJobs(mig2ID, jobs)
	if got == nil {
		t.Fatal("resolveDetectedStackExpectationFromJobs() = nil, want non-nil")
	}
	if got.Language != "java" || got.Tool != "gradle" || got.Release != "21" {
		t.Fatalf("detected stack = %+v, want java/gradle/21", *got)
	}
}

func TestResolveDetectedStackExpectationFromJobs_NoGateInChain(t *testing.T) {
	t.Parallel()

	mig1ID := domaintypes.NewJobID()
	mig2ID := domaintypes.NewJobID()
	jobs := []store.Job{
		{
			ID:      mig1ID,
			JobType: domaintypes.JobTypeMig,
			Status:  domaintypes.JobStatusSuccess,
			NextID:  &mig2ID,
		},
		{
			ID:      mig2ID,
			JobType: domaintypes.JobTypeMig,
			Status:  domaintypes.JobStatusCreated,
		},
	}

	got := resolveDetectedStackExpectationFromJobs(mig2ID, jobs)
	if got != nil {
		t.Fatalf("resolveDetectedStackExpectationFromJobs() = %+v, want nil", *got)
	}
}

func mustMarshalGateMeta(t *testing.T, language, tool, release string) []byte {
	t.Helper()
	raw, err := contracts.MarshalJobMeta(&contracts.JobMeta{
		Kind: contracts.JobKindGate,
		GateMetadata: &contracts.BuildGateStageMetadata{
			Detected: &contracts.StackExpectation{
				Language: language,
				Tool:     tool,
				Release:  release,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal gate job meta: %v", err)
	}
	return raw
}
