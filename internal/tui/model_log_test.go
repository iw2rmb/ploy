package tui

import (
	"net/http"
	"net/url"
	"testing"

	tea "charm.land/bubbletea/v2"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/stream"
)

func TestLogFollowsHighlightedJobAcrossRefreshAndNavigation(t *testing.T) {
	for _, runID := range []domaintypes.RunID{"", "run"} {
		t.Run("scope="+runID.String(), func(t *testing.T) {
			m := InitialModel(t.Context(), http.DefaultClient, &url.URL{})
			m.selectedRunID = runID
			m.rootList.Select(jobsIndex)
			if !runID.IsZero() {
				m.screen = ScreenJobsList
			}
			jobs := []domainapi.JobListItem{{JobID: "first", JobType: domaintypes.JobTypePostGate}, {JobID: "second", JobType: domaintypes.JobTypePostGate}, {JobID: "mig", JobType: domaintypes.JobTypeMig}}
			update := func(msg tea.Msg) { next, _ := m.Update(msg); m = next.(model) }
			loaded := jobsLoadedMsg{runID: runID, jobs: jobs}
			update(loaded)
			first := m.jobLog.sub
			if first == nil || first.jobID != "first" {
				t.Fatal("missing first job stream")
			}
			update(jobLogMsg{sub: first, records: []logstream.LogRecord{{Line: "first output"}}})
			update(loaded)
			if m.jobLog.sub != first || len(m.jobLog.lines) != 1 {
				t.Fatal("refresh replaced selected log")
			}
			if runID.IsZero() {
				update(tea.KeyPressMsg{Code: tea.KeyEnter})
			}
			update(tea.KeyPressMsg{Code: tea.KeyDown})
			second := m.jobLog.sub
			if second == nil || second.jobID != "second" || len(m.jobLog.lines) != 0 {
				t.Fatal("cursor did not replace the visible log")
			}
			update(tea.KeyPressMsg{Code: tea.KeyUp})
			update(jobLogMsg{sub: first, records: []logstream.LogRecord{{Line: "stale"}}, done: true})
			if m.jobLog.sub == first || m.jobLog.done || len(m.jobLog.lines) != 0 {
				t.Fatal("revisited job accepted a stale stream")
			}
			update(tea.KeyPressMsg{Code: tea.KeyDown})
			update(tea.KeyPressMsg{Code: tea.KeyDown})
			if m.jobLog.sub != nil {
				t.Fatal("mig retained post_gate log")
			}
		})
	}
}

func TestLogRefreshReopensResumedJob(t *testing.T) {
	m := InitialModel(t.Context(), http.DefaultClient, &url.URL{})
	m.screen = ScreenJobsList
	job := domainapi.JobListItem{JobID: "resumed", JobType: domaintypes.JobTypePostGate, Status: domaintypes.JobStatusCancelled}
	next, _ := m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{job}})
	m = next.(model)
	old := m.jobLog.sub
	job.Status = domaintypes.JobStatusRunning
	next, _ = m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{job}})
	m = next.(model)
	if m.jobLog.sub == nil || m.jobLog.sub == old {
		t.Fatal("resumed job retained its previous stream")
	}
}

func TestFilterWithNoMatchingJobHidesLog(t *testing.T) {
	m := InitialModel(t.Context(), http.DefaultClient, &url.URL{})
	m.screen = ScreenJobsList
	next, _ := m.Update(jobsLoadedMsg{jobs: []domainapi.JobListItem{{JobID: "first", JobType: domaintypes.JobTypePostGate}}})
	m = next.(model)
	if m.jobLog.sub == nil {
		t.Fatal("missing initial log")
	}
	for _, key := range []rune{'/', 'z'} {
		next, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		m = applyRunFilterCommands(next.(model), cmd)
	}
	if m.jobLog.sub != nil || !m.highlightedLogJob().IsZero() {
		t.Fatal("filtered list retained hidden log")
	}
}
