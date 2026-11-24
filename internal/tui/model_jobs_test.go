package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestJobsLoadedWiresComponentAndRenderedView(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenJobsList
	jobs := []domainapi.JobListItem{
		{DisplayName: "deploy", Status: domaintypes.JobStatusRunning, JobID: domaintypes.JobID("job-1"), RunID: domaintypes.RunID("run-1"), RepoID: domaintypes.RepoID("repo-1")},
	}
	next, _ := m.Update(jobsLoadedMsg{jobs: jobs})
	nm := next.(model)

	if got := nm.jobList.Jobs(); len(got) != 1 || got[0].JobID != jobs[0].JobID {
		t.Fatalf("job component state = %+v, want job %q", got, jobs[0].JobID)
	}
	rendered := nm.View().Content
	for _, want := range []string{"PLOY", "JOBS", "deploy"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered view missing %q", want)
		}
	}
}

func TestJobsLoadedKeepsAvailableRowsWhenMetadataFails(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenJobsList
	m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "removed"}})
	failure := errors.New("run metadata unavailable")
	jobs := []domainapi.JobListItem{
		{JobID: "running", RunID: "run", Status: domaintypes.JobStatusRunning},
		{JobID: "complete", RunID: "other-run", Status: domaintypes.JobStatusSuccess, DisplayName: "healing"},
	}
	next, cmd := m.Update(jobsLoadedMsg{jobs: jobs, err: failure})
	m = next.(model)
	if len(m.jobList.Jobs()) != 2 || m.jobList.Jobs()[0].JobID != "running" || m.jobList.Jobs()[1].DisplayName != "healing" {
		t.Fatalf("partial metadata discarded current rows: %+v", m.jobList.Jobs())
	}
	if !errors.Is(m.lastErr, failure) || cmd == nil || !m.refreshScheduled {
		t.Fatal("metadata failure lost its error or stopped active refresh")
	}

	next, _ = m.Update(jobsLoadedMsg{err: errors.New("list unavailable")})
	if len(next.(model).jobList.Jobs()) != 2 {
		t.Fatal("list failure discarded the last available rows")
	}
}

// Enter confirms the selected row and preserves its identity through refresh.
func TestJobSelectionUpdatesContext(t *testing.T) {
	for _, runID := range []domaintypes.RunID{"", "run-abc"} {
		t.Run("scope="+runID.String(), func(t *testing.T) {
			m := InitialModel(t.Context(), nil, nil)
			m.usersLoading = false
			m.screen = ScreenJobsList
			runEntry := listItem{title: "Run", description: "run-abc\n"}
			if !runID.IsZero() {
				m = makeRunDetailsModel(t)
				m.rootList.Select(jobsIndex)
				next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				m = next.(model)
				runEntry = listItem{title: "✓ java", description: "team/app\n@alice"}
			}
			jobs := []domainapi.JobListItem{
				{JobID: "job-1", DisplayName: "deploy", RunID: "run-abc", RepoURL: "https://example.com/team/app.git", Status: domaintypes.JobStatusSuccess},
				{JobID: "job-2", DisplayName: "verify", RunID: "run-abc", RepoURL: "https://example.com/team/app.git", Status: domaintypes.JobStatusSuccess},
			}
			next, _ := m.Update(jobsLoadedMsg{requestID: m.jobsRequest, runID: runID, jobs: jobs})
			m = next.(model)
			result, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd != nil {
				t.Fatal("job selection issued a command")
			}
			selected := result.(model)
			if selected.screen != ScreenJobsList || selected.selectedRunID != runID {
				t.Fatal("job selection changed the screen or scope")
			}
			if got := selected.jobList.ConfirmedJobID(); got != "job-1" {
				t.Errorf("confirmed job ID = %q, want job-1", got)
			}
			assertPloyItems(t, selected, []listItem{
				runEntry,
				{title: "✓ deploy", description: "team/app\n0s"},
			})
			selected.jobList = selected.jobList.Select(1)
			jobs[0].DurationMs = 6500
			result, _ = selected.Update(jobsLoadedMsg{requestID: m.jobsRequest, runID: runID, jobs: []domainapi.JobListItem{jobs[1], jobs[0]}})
			refreshed := result.(model)
			if refreshed.jobList.ConfirmedJobID() != "job-1" || refreshed.jobList.SelectedJobID() != "job-2" {
				t.Fatal("refresh changed highlighted or confirmed job identity")
			}
			assertPloyItems(t, refreshed, []listItem{
				runEntry,
				{title: "✓ deploy", description: "team/app\n6s"},
			})
		})
	}
}

// Scope changes clear prior selection and reject unrelated jobs.
func TestJobsScopeRejectsStaleResponses(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenJobsList
	m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "job-global", RunID: "run-global"}}).SetSelectedJobID("job-global")
	m.setRun("run-current")
	if len(m.jobList.Jobs()) != 0 || m.jobList.ConfirmedJobID() != "" {
		t.Fatal("scope switch retained old job state")
	}
	for _, scope := range []domaintypes.RunID{"", "run-other"} {
		next, _ := m.Update(jobsLoadedMsg{runID: scope, jobs: []domainapi.JobListItem{{JobID: "job-stale"}}})
		m = next.(model)
		if len(m.jobList.Jobs()) != 0 {
			t.Fatalf("accepted stale scope %q", scope)
		}
	}
	next, _ := m.Update(jobsLoadedMsg{runID: "run-current", jobs: []domainapi.JobListItem{{JobID: "job-current", RunID: "run-current"}}})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.jobList.ConfirmedJobID() != "job-current" {
		t.Fatal("current job selection was not confirmed")
	}
	m.setRun("")
	next, _ = m.Update(jobsLoadedMsg{runID: "run-current", jobs: []domainapi.JobListItem{{JobID: "job-stale"}}})
	m = next.(model)
	if len(m.jobList.Jobs()) != 0 {
		t.Fatal("global scope accepted scoped jobs")
	}
}

// Filtering a global list selects the visible job without changing its origin.
func TestFilteredGlobalJobSelectionKeepsGlobalScope(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	m.screen = ScreenJobsList
	m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "job-first", DisplayName: "alpha", RunID: "run-first"}, {JobID: "job-q", DisplayName: "query", RunID: "run-q"}})
	for _, msg := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 'q', Text: "q"}} {
		next, cmd := m.Update(msg)
		m = applyRunFilterCommands(next.(model), cmd)
	}
	if !m.jobList.SettingFilter() {
		t.Fatal("job filter is not active")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+C did not quit during job filtering")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = applyRunFilterCommands(next.(model), cmd)
	if m.jobList.ConfirmedJobID() != "" || !m.jobList.IsFiltered() {
		t.Fatal("filter Enter confirmed a job")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	if m.jobList.ConfirmedJobID() != "job-q" || !m.selectedRunID.IsZero() {
		t.Fatalf("job selection = %q, selected run %q", m.jobList.ConfirmedJobID(), m.selectedRunID)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = applyRunFilterCommands(next.(model), cmd)
	if m.screen != ScreenJobsList || m.jobList.IsFiltered() {
		t.Fatal("Esc failed to clear the job filter")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(model)
	if m.screen != ScreenPloyList {
		t.Fatal("global Jobs returned to run details")
	}
	assertPloyItems(t, m, []listItem{{title: "Runs", description: "select run\n"}, {title: "Jobs", description: "select job\n"}, {title: "Users", description: "cli-admin only\n"}})
}

// A response from an earlier root visit cannot reset the selected job.
func TestJobsLatestRequestPreservesSelection(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.usersLoading = false
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	previewRequest := m.jobsRequest
	next, _ = m.Update(jobsLoadedMsg{requestID: previewRequest, jobs: []domainapi.JobListItem{{JobID: "job-preview"}}})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(model)
	if m.jobsRequest <= previewRequest {
		t.Fatal("root Jobs revisit did not supersede its completed request")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)

	next, _ = m.Update(jobsLoadedMsg{requestID: m.jobsRequest, jobs: []domainapi.JobListItem{{JobID: "job-current", RunID: "run-current"}}})
	m = next.(model)
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(model)
	for _, late := range []jobsLoadedMsg{
		{requestID: previewRequest, jobs: []domainapi.JobListItem{{JobID: "job-old"}}},
		{requestID: previewRequest, err: errors.New("old failure")},
	} {
		next, _ = m.Update(late)
		m = next.(model)
	}
	if m.lastErr != nil {
		t.Fatal("late preview replaced the current error")
	}
	if m.jobList.ConfirmedJobID() != "job-current" || len(m.jobList.Jobs()) != 1 || m.jobList.Jobs()[0].JobID != "job-current" {
		t.Fatal("late preview reset the current selection")
	}
	assertPloyItems(t, m, []listItem{{title: "Run", description: "run-current\n"}, {title: "  -", description: "-\n0s"}})
}
