package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type expectedJob struct {
	name      string
	jobType   domaintypes.JobType
	status    domaintypes.JobStatus
	jobImage  string
	repoSHAIn string
}

func createJobsByName(calls []store.CreateJobParams) map[string]store.CreateJobParams {
	byName := make(map[string]store.CreateJobParams, len(calls))
	for i := range calls {
		byName[calls[i].Name] = calls[i]
	}
	return byName
}

func assertJobChain(
	t *testing.T,
	calls []store.CreateJobParams,
	runID domaintypes.RunID,
	repoID domaintypes.RepoID,
	repoBaseRef string,
	attempt int32,
	expected []expectedJob,
) {
	t.Helper()

	if len(calls) != len(expected) {
		t.Fatalf("create job calls=%d, want=%d", len(calls), len(expected))
	}
	byName := createJobsByName(calls)
	for _, want := range expected {
		got, ok := byName[want.name]
		if !ok {
			t.Fatalf("missing expected job %q", want.name)
		}
		if got.RunID != runID || got.RepoID != repoID || got.RepoBaseRef != repoBaseRef || got.Attempt != attempt {
			t.Fatalf("job %q identity mismatch: run=%s repo=%s base_ref=%q attempt=%d", want.name, got.RunID, got.RepoID, got.RepoBaseRef, got.Attempt)
		}
		if got.JobType != want.jobType {
			t.Fatalf("job %q type=%q, want %q", want.name, got.JobType, want.jobType)
		}
		if got.Status != want.status {
			t.Fatalf("job %q status=%q, want %q", want.name, got.Status, want.status)
		}
		if got.JobImage != want.jobImage {
			t.Fatalf("job %q image=%q, want %q", want.name, got.JobImage, want.jobImage)
		}
		if got.RepoShaIn != want.repoSHAIn {
			t.Fatalf("job %q repo_sha_in=%q, want %q", want.name, got.RepoShaIn, want.repoSHAIn)
		}
	}
}
