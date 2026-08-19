package tui

import (
	"strings"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestJobsLoadedWiresComponentAndRenderedView(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenJobsList
	jobs := []domainapi.JobListItem{
		{Name: "deploy", Status: domaintypes.JobStatusRunning, JobID: domaintypes.JobID("job-1"), MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}
	next, _ := m.Update(jobsLoadedMsg{jobs: jobs})
	nm := next.(model)

	if got := nm.jobList.Jobs(); len(got) != 1 || got[0].JobID != jobs[0].JobID {
		t.Fatalf("job component state = %+v, want job %q", got, jobs[0].JobID)
	}
	rendered := nm.View().Content
	for _, want := range []string{"PLOY", "JOBS", "job-1"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered view missing %q", want)
		}
	}
}

func TestJobSelectionUpdatesContext(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenJobsList
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{ID: domaintypes.RunID("run-1"), MigID: domaintypes.MigID("mig-1"), MigName: "mig", CreatedAt: time.Now()},
	}})
	m = next.(model)
	m.screen = ScreenJobsList
	next, _ = m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{
		{JobID: domaintypes.JobID("job-1"), Name: "deploy", MigName: "mig", RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}})
	m = next.(model)
	m.jobList = m.jobList.Select(0)

	result, _ := m.handleEnter()
	selected := result.(model)
	if selected.screen != ScreenJobsList {
		t.Errorf("job selection screen = %v, want ScreenJobsList", selected.screen)
	}
	if got := selected.jobList.ConfirmedJobID(); got != "job-1" {
		t.Errorf("confirmed job ID = %q, want %q", got, "job-1")
	}
	assertPloyItems(t, selected, []listItem{
		{title: "mig", description: "mig-1"},
		{title: "Run", description: "run-1"},
		{title: "Job", description: "job-1"},
	})
}
