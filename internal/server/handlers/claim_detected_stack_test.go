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
	sbom1ID := domaintypes.NewJobID()
	hook1ID := domaintypes.NewJobID()
	reGateID := domaintypes.NewJobID()
	sbom2ID := domaintypes.NewJobID()
	hook2ID := domaintypes.NewJobID()

	jobs := []store.Job{
		{
			ID:      preGateID,
			JobType: domaintypes.JobTypePreGate,
			Status:  domaintypes.JobStatusSuccess,
			NextID:  &sbom1ID,
			Meta:    mustMarshalGateMeta(t, "java", "maven", "17"),
		},
		{
			ID:      sbom1ID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
			NextID:  &hook1ID,
		},
		{
			ID:      hook1ID,
			JobType: domaintypes.JobTypeHook,
			Status:  domaintypes.JobStatusSuccess,
			NextID:  &reGateID,
		},
		{
			ID:      reGateID,
			JobType: domaintypes.JobTypeReGate,
			Status:  domaintypes.JobStatusFail,
			NextID:  &sbom2ID,
			Meta:    mustMarshalGateMeta(t, "java", "gradle", "21"),
		},
		{
			ID:      sbom2ID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
			NextID:  &hook2ID,
		},
		{
			ID:      hook2ID,
			JobType: domaintypes.JobTypeHook,
			Status:  domaintypes.JobStatusCreated,
		},
	}

	got := resolveDetectedStackExpectationFromJobs(hook2ID, jobs)
	if got == nil {
		t.Fatal("resolveDetectedStackExpectationFromJobs() = nil, want non-nil")
	}
	if got.Language != "java" || got.Tool != "gradle" || got.Release != "21" {
		t.Fatalf("detected stack = %+v, want java/gradle/21", *got)
	}
}

func TestResolveDetectedStackExpectationFromJobs_NoGateInChain(t *testing.T) {
	t.Parallel()

	sbomID := domaintypes.NewJobID()
	hookID := domaintypes.NewJobID()
	jobs := []store.Job{
		{
			ID:      sbomID,
			JobType: domaintypes.JobTypeSBOM,
			Status:  domaintypes.JobStatusSuccess,
			NextID:  &hookID,
		},
		{
			ID:      hookID,
			JobType: domaintypes.JobTypeHook,
			Status:  domaintypes.JobStatusCreated,
		},
	}

	got := resolveDetectedStackExpectationFromJobs(hookID, jobs)
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
