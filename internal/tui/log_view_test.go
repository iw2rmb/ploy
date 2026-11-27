package tui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/stream"
)

// The timestamp appears once and only stderr timestamps have color.
func TestLogRecordWrapsUnderTimestamp(t *testing.T) {
	rec := logstream.LogRecord{Timestamp: "2026-11-27T12:30:00+03:00", Stream: "stderr", Line: "red\x1b[31m!\x1b[m\n  尾123\a"}
	want := []string{stderrTimeStyle.Render("09:30:00") + " red!", "           尾", "         123"}
	if got := renderLogRecord(rec, 13); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	rec.Stream = "stdout"
	if got := renderLogRecord(rec, 13)[0]; got != "09:30:00 red!" {
		t.Fatalf("stdout = %q", got)
	}
}

// Equal timestamps keep separate records and the panel keeps the latest rows.
func TestLogPanelShowsLatestOutputAndTerminalState(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.windowHeight = 4
	for _, line := range []string{"old", "first", "second"} {
		m.jobLog.lines = append(m.jobLog.lines, logstream.LogRecord{Timestamp: "2026-11-27T09:30:00Z", Line: line})
	}
	rendered := ansi.Strip(m.renderJobLog(32))
	if strings.Contains(rendered, "old") || !strings.HasSuffix(rendered, "09:30:00 first\n09:30:00 second") {
		t.Fatalf("tail = %q", rendered)
	}
	m.jobLog.lines = nil
	m.jobLog.done = true
	if !strings.Contains(m.renderJobLog(32), "No output") {
		t.Fatal("missing empty result")
	}
	m.jobLog.err = errors.New("stream failed")
	if !strings.Contains(m.renderJobLog(32), "stream failed") {
		t.Fatal("missing stream error")
	}
}

// JOBS renders the selected post_gate output in a third column.
func TestJobsShowsLogBesideHighlightedPostGate(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.windowWidth, m.windowHeight = 115, 20
	m.applyWindowSize()
	m.rootList.Select(jobsIndex)
	m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "gate", JobType: domaintypes.JobTypePostGate}, {JobID: "mig", JobType: domaintypes.JobTypeMig}})
	m.jobLog.lines = []logstream.LogRecord{{Timestamp: "2026-11-27T09:30:00Z", Line: "build output"}}
	for _, screen := range []Screen{ScreenPloyList, ScreenJobsList} {
		m.screen = screen
		rendered := ansi.Strip(m.View().Content)
		if firstLineWith(rendered, "JOBS") != firstLineWith(rendered, "LOG") || !strings.Contains(rendered, "09:30:00 build output") {
			t.Fatalf("screen %d missing log: %s", screen, rendered)
		}
	}
	m.jobList = m.jobList.Select(1)
	if strings.Contains(m.View().Content, "LOG") {
		t.Fatal("mig shows LOG")
	}
}
