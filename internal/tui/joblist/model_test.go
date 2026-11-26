package joblist_test

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	cliruns "github.com/iw2rmb/ploy/internal/cli/runs"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/tui/joblist"
)

type defaultItem interface {
	Title() string
	Description() string
}

func mustDefaultItem(t *testing.T, value any) defaultItem {
	t.Helper()
	item, ok := value.(defaultItem)
	if !ok {
		t.Fatalf("item has type %T, want defaultItem", value)
	}
	return item
}

func TestNewHasCorrectTitleAndWidth(t *testing.T) {
	m := joblist.New("JOBS")
	if m.Title() != "JOBS" {
		t.Errorf("Title() = %q, want %q", m.Title(), "JOBS")
	}
	if m.Width() != joblist.ListWidth {
		t.Errorf("Width() = %d, want %d", m.Width(), joblist.ListWidth)
	}
}

// Each row shows the status and step name, repository path, and duration.
func TestSetJobsFormatsRows(t *testing.T) {
	m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{
		{JobID: "job-abc", Name: "internal-name", DisplayName: "  Deploy  ", RepoURL: "https://git.example.com/team/backend/app.git", Status: domaintypes.JobStatusSuccess, DurationMs: 2500},
		{JobID: "job-type", Name: "internal-name", JobType: domaintypes.JobType("PRE_GATE"), Status: domaintypes.JobStatusQueued},
		{JobID: "job-empty", Name: "internal-name", Status: domaintypes.JobStatusCreated},
	})
	wantTitles := []string{"✓ Deploy", "- pre_gate", "- -"}
	wantDescriptions := []string{"team/backend/app\n2s", "-\n0s", "-\n0s"}
	for i, item := range m.Items() {
		r := mustDefaultItem(t, item)
		if r.Title() != wantTitles[i] || r.Description() != wantDescriptions[i] {
			t.Fatalf("row %d = %q / %q", i, r.Title(), r.Description())
		}
		if strings.Contains(r.Title()+item.FilterValue(), "\x1b[") {
			t.Fatal("row/filter value contains ANSI")
		}
	}
}

// Started jobs sort newest first; pending jobs and equal starts retain their input order.
func TestSetJobsSortsStartedNewestFirst(t *testing.T) {
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	jobs := []domainapi.JobListItem{{JobID: "pending-1"}, {JobID: "old", StartedAt: &older}, {JobID: "new-1", StartedAt: &newer}, {JobID: "pending-2"}, {JobID: "new-2", StartedAt: &newer}}
	m := joblist.New("JOBS").SetJobs(jobs)
	for i, id := range []domaintypes.JobID{"new-1", "new-2", "old", "pending-1", "pending-2"} {
		if m.Jobs()[i].JobID != id {
			t.Fatalf("job %d = %q, want %q", i, m.Jobs()[i].JobID, id)
		}
	}
	if jobs[0].JobID != "pending-1" {
		t.Fatal("sorting mutated caller input")
	}
}

func TestHighlightedSelectionFollowsCursor(t *testing.T) {
	m := joblist.New("JOBS")
	if _, ok := m.SelectedJob(); ok || m.SelectedJobID() != "" {
		t.Fatal("empty model has a highlighted selection")
	}

	m = m.SetJobs([]domainapi.JobListItem{
		{JobID: domaintypes.JobID("job-1"), DisplayName: "first"},
		{JobID: domaintypes.JobID("job-2"), DisplayName: "second"},
	}).Select(1)
	job, ok := m.SelectedJob()
	if !ok || job.JobID != "job-2" || m.SelectedJobID() != "job-2" {
		t.Errorf("highlighted selection = (%+v, %v, %q), want job-2", job, ok, m.SelectedJobID())
	}
}

// Filtering must preserve the selected job's identity when its visible index changes.
func TestFilteredSelectionUsesVisibleJob(t *testing.T) {
	m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{
		{JobID: "job-1", DisplayName: "alpha"},
		{JobID: "job-2", DisplayName: "zulu"},
	})
	for _, key := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 'z', Text: "z"}} {
		var cmd tea.Cmd
		m, cmd = m.Update(key)
		m = applyFilterCommands(m, cmd)
	}
	if !m.SettingFilter() {
		t.Fatal("filter input is not active")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.SettingFilter() || !m.IsFiltered() {
		t.Fatal("Enter did not accept the filter")
	}
	job, ok := m.SelectedJob()
	if !ok || job.JobID != "job-2" || m.Index() != 0 {
		t.Fatalf("filtered selection = (%+v, %v), index %d; want job-2 at index 0", job, ok, m.Index())
	}
	if got := m.Items()[1].FilterValue(); !strings.Contains(got, "zulu") || !strings.HasSuffix(got, " job-2") {
		t.Fatalf("filter value = %q, want plain job name and ID", got)
	}
	m = m.SetSelectedJobID("job-2")
	m = m.SetJobs([]domainapi.JobListItem{{JobID: "job-3", DisplayName: "other"}})
	if m.IsFiltered() || m.SettingFilter() || m.ConfirmedJobID() != "" {
		t.Fatal("replacement jobs retained the filter or confirmed selection")
	}
	if job, ok := m.SelectedJob(); !ok || job.JobID != "job-3" {
		t.Fatalf("replacement selection = (%+v, %v), want job-3", job, ok)
	}
}

func applyFilterCommands(m joblist.Model, cmd tea.Cmd) joblist.Model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, child := range msg {
			m = applyFilterCommands(m, child)
		}
	case list.FilterMatchesMsg:
		m, _ = m.Update(msg)
	}
	return m
}

func TestConfirmedSelectionCanBeSetAndCleared(t *testing.T) {
	m := joblist.New("JOBS")
	if got := m.ConfirmedJobID(); got != "" {
		t.Fatalf("initial ConfirmedJobID() = %q, want empty", got)
	}
	m = m.SetSelectedJobID(domaintypes.JobID("job-42"))
	if got := m.ConfirmedJobID(); got != "job-42" {
		t.Errorf("confirmed selection after set = %q, want %q", got, "job-42")
	}
	m = m.SetSelectedJobID("")
	if got := m.ConfirmedJobID(); got != "" {
		t.Errorf("confirmed selection after clear = %q, want empty", got)
	}
}

// Terminal glyphs match stdout; running rows use the supplied frame and clock.
func TestViewUsesStdoutGlyphsAndLiveDuration(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, status := range []domaintypes.JobStatus{domaintypes.JobStatusSuccess, domaintypes.JobStatusFail, domaintypes.JobStatusError, domaintypes.JobStatusCancelled} {
		m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{{JobID: "job-terminal", DisplayName: "deploy", Status: status, DurationMs: 2500}})
		text := ansi.Strip(m.View(true, start, 0))
		want := cliruns.StatusGlyph(status.String(), 0) + " deploy"
		if !strings.Contains(text, want) {
			t.Fatalf("terminal %s: missing %q in %q", status, want, text)
		}
	}
	m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{{JobID: "job-running", DisplayName: "healing", Status: domaintypes.JobStatusRunning, StartedAt: &start}})
	for _, frame := range []int{0, 1} {
		text := ansi.Strip(m.View(true, start.Add(time.Duration(frame+1)*time.Second), frame))
		want := cliruns.StatusGlyph("running", frame) + " healing"
		if !strings.Contains(text, want) || !strings.Contains(text, "│ "+string(rune('1'+frame))+"s") {
			t.Fatalf("frame %d: missing %q in %q", frame, want, text)
		}
	}
}

// Durations floor fractional seconds and retain elapsed hours beyond one day.
func TestJobDurations(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	finish := start.Add(25*time.Hour + 2*time.Minute + 3900*time.Millisecond)
	for _, tt := range []struct {
		name string
		job  domainapi.JobListItem
		now  time.Time
		want string
	}{
		{"recorded terminal", domainapi.JobListItem{Status: domaintypes.JobStatusSuccess, DurationMs: 3661999, StartedAt: &start, FinishedAt: &finish}, finish, " 1h  1m  1s"},
		{"timestamp terminal", domainapi.JobListItem{Status: domaintypes.JobStatusFail, StartedAt: &start, FinishedAt: &finish}, finish.Add(time.Hour), "25h  2m  3s"},
		{"running elapsed", domainapi.JobListItem{Status: domaintypes.JobStatusRunning, DurationMs: 1, StartedAt: &start}, start.Add(100*time.Hour + 2*time.Minute + 3*time.Second), "100h  2m  3s"},
		{"minutes only", domainapi.JobListItem{Status: domaintypes.JobStatusSuccess, DurationMs: 65000}, finish, "     1m  5s"},
		{"exact hour", domainapi.JobListItem{Status: domaintypes.JobStatusSuccess, DurationMs: 3600000}, finish, " 1h      0s"},
		{"future start", domainapi.JobListItem{Status: domaintypes.JobStatusRunning, StartedAt: &finish}, start, "         0s"},
		{"missing start", domainapi.JobListItem{Status: domaintypes.JobStatusRunning}, finish, "--h --m --s"},
		{"pending", domainapi.JobListItem{Status: domaintypes.JobStatusQueued, DurationMs: 1234}, finish, "         0s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.job.JobID = "job-duration"
			tt.job.DisplayName = "healing"
			m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{tt.job})
			if view := ansi.Strip(m.View(true, tt.now, 0)); !strings.Contains(view, "│ "+strings.TrimSpace(tt.want)) {
				t.Fatalf("missing duration %q in %q", tt.want, view)
			}
		})
	}
}

// Polling stays active while any created, queued, or running job remains.
func TestHasActiveJobs(t *testing.T) {
	for _, tt := range []struct {
		status domaintypes.JobStatus
		want   bool
	}{
		{domaintypes.JobStatusCreated, true}, {domaintypes.JobStatusQueued, true}, {domaintypes.JobStatusRunning, true},
		{domaintypes.JobStatusSuccess, false}, {domaintypes.JobStatusFail, false}, {domaintypes.JobStatusCancelled, false}, {domaintypes.JobStatusError, false},
	} {
		m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{{Status: domaintypes.JobStatusSuccess}, {Status: tt.status}})
		if m.HasActiveJobs() != tt.want {
			t.Fatalf("status %s: active %v, want %v", tt.status, m.HasActiveJobs(), tt.want)
		}
	}
	if joblist.New("JOBS").HasActiveJobs() {
		t.Fatal("empty jobs are active")
	}
}

// Refresh preserves highlighted and confirmed job identities across reordering.
func TestRefreshJobsPreservesSelectionByIdentity(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newStart := start.Add(time.Hour)
	m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{{JobID: "job-1", StartedAt: &start}, {JobID: "job-2"}}).Select(1)
	m = m.SetSelectedJobID("job-1")
	m = m.RefreshJobs([]domainapi.JobListItem{{JobID: "job-1", StartedAt: &start}, {JobID: "job-2", StartedAt: &newStart}})
	if m.SelectedJobID() != "job-2" || m.Index() != 0 || m.ConfirmedJobID() != "job-1" {
		t.Fatalf("refreshed selection: highlighted %q index %d, confirmed %q", m.SelectedJobID(), m.Index(), m.ConfirmedJobID())
	}
	m = m.RefreshJobs([]domainapi.JobListItem{{JobID: "job-2", StartedAt: &newStart}})
	if m.SelectedJobID() != "job-2" || m.ConfirmedJobID() != "" {
		t.Fatal("removed confirmed selection was retained")
	}
	m = m.RefreshJobs(nil)
	if _, ok := m.SelectedJob(); ok || m.ConfirmedJobID() != "" {
		t.Fatal("empty refresh retained selection")
	}
}

// Refresh retains an applied filter and its selected row when a job starts.
func TestRefreshJobsPreservesAppliedFilter(t *testing.T) {
	m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{{JobID: "job-1", DisplayName: "zulu"}, {JobID: "job-2", DisplayName: "zebra"}, {JobID: "job-3", DisplayName: "other"}})
	for _, key := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 'z', Text: "z"}, {Code: tea.KeyEnter}, {Code: tea.KeyDown}} {
		var cmd tea.Cmd
		m, cmd = m.Update(key)
		m = applyFilterCommands(m, cmd)
	}
	if m.SelectedJobID() != "job-2" {
		t.Fatalf("initial selected job = %q", m.SelectedJobID())
	}
	start := time.Now()
	m = m.RefreshJobs([]domainapi.JobListItem{{JobID: "job-1", DisplayName: "zulu"}, {JobID: "job-2", DisplayName: "zebra", StartedAt: &start}, {JobID: "job-3", DisplayName: "other"}})
	if !m.IsFiltered() || m.SettingFilter() || m.SelectedJobID() != "job-2" || m.Index() != 0 {
		t.Fatal("refresh lost the applied filter or its selection")
	}
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = applyFilterCommands(m, cmd)
	if m.SelectedJobID() != "job-1" {
		t.Fatalf("filtered next row = %q, want job-1", m.SelectedJobID())
	}
}

// Refresh preserves the editing position, so typing continues at the same character.
func TestRefreshJobsPreservesFilterEditingCursor(t *testing.T) {
	jobs := []domainapi.JobListItem{{JobID: "job-middle", DisplayName: "azb"}, {JobID: "job-end", DisplayName: "abz"}}
	m := joblist.New("JOBS").SetJobs(jobs)
	for _, key := range []tea.KeyPressMsg{{Code: '/', Text: "/"}, {Code: 'a', Text: "a"}, {Code: 'b', Text: "b"}, {Code: tea.KeyLeft}} {
		var cmd tea.Cmd
		m, cmd = m.Update(key)
		m = applyFilterCommands(m, cmd)
	}
	m = m.RefreshJobs(jobs)
	if !m.SettingFilter() {
		t.Fatal("refresh ended filter editing")
	}
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m = applyFilterCommands(m, cmd)
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = applyFilterCommands(m, cmd)
	if m.SelectedJobID() != "job-middle" {
		t.Fatalf("typing after refresh selected %q, want job-middle", m.SelectedJobID())
	}
}

// The supplied width bounds long rows and the focus flag owns the selection bar.
func TestViewUsesWidthAndFocus(t *testing.T) {
	m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{{JobID: "job-long-long-long-long-long", DisplayName: "long long long long long long", Status: domaintypes.JobStatusSuccess}})
	m.SetWidth(24)
	focused := ansi.Strip(m.View(true, time.Now(), 0))
	unfocused := ansi.Strip(m.View(false, time.Now(), 0))
	if !strings.Contains(focused, "│") || strings.Contains(unfocused, "│") {
		t.Fatalf("focus marker mismatch: %q / %q", focused, unfocused)
	}
	for _, line := range strings.Split(focused, "\n") {
		if ansi.StringWidth(line) > 24 {
			t.Fatalf("line exceeds requested width: %q", line)
		}
	}
}

// Late filter results cannot restore rows removed by a newer jobs response.
func TestRefreshRejectsOldFilterRows(t *testing.T) {
	m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{{JobID: "job-removed", DisplayName: "azb"}, {JobID: "job-kept", DisplayName: "azb"}})
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = applyFilterCommands(m, cmd)
	m, cmd = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	old := captureFilterMatches(t, cmd)
	m = m.RefreshJobs([]domainapi.JobListItem{{JobID: "job-kept", DisplayName: "azb", Status: domaintypes.JobStatusRunning}})
	m, _ = m.Update(old)
	if !m.SettingFilter() {
		t.Fatal("late result ended filter editing")
	}
	if job, ok := m.SelectedJob(); !ok || job.JobID != "job-kept" || job.Status != domaintypes.JobStatusRunning {
		t.Fatalf("late filter selected stale job: %+v, %v", job, ok)
	}
	for _, key := range []tea.KeyPressMsg{{Code: 'b', Text: "b"}, {Code: tea.KeyLeft}} {
		m, cmd = m.Update(key)
		m = applyFilterCommands(m, cmd)
	}
	m, _ = m.Update(old)
	m, cmd = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m = applyFilterCommands(m, cmd)
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = applyFilterCommands(m, cmd)
	if m.SelectedJobID() != "job-kept" || !m.IsFiltered() {
		t.Fatal("late filter response moved the input cursor")
	}
	m, _ = m.Update(old)
	if m.SettingFilter() || !m.IsFiltered() || m.SelectedJobID() != "job-kept" {
		t.Fatal("late response changed the applied filter")
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = applyFilterCommands(m, cmd)
	m, _ = m.Update(old)
	if m.SettingFilter() || m.IsFiltered() || m.SelectedJobID() != "job-kept" {
		t.Fatal("late response restored a cleared filter")
	}
}

func captureFilterMatches(t *testing.T, cmd tea.Cmd) list.FilterMatchesMsg {
	t.Helper()
	var find func(tea.Cmd) (list.FilterMatchesMsg, bool)
	find = func(cmd tea.Cmd) (list.FilterMatchesMsg, bool) {
		if cmd == nil {
			return nil, false
		}
		switch msg := cmd().(type) {
		case list.FilterMatchesMsg:
			return msg, true
		case tea.BatchMsg:
			for _, child := range msg {
				if matches, ok := find(child); ok {
					return matches, true
				}
			}
		}
		return nil, false
	}
	if matches, ok := find(cmd); ok {
		return matches
	}
	t.Fatal("filter command returned no FilterMatchesMsg")
	return nil
}

// Only unselected success and failure glyphs use semantic colors in either focus state.
func TestJobStatusGlyphColorsFollowSelection(t *testing.T) {
	for _, tc := range []struct {
		status domaintypes.JobStatus
		color  string
	}{
		{domaintypes.JobStatusSuccess, "\x1b[92m"},
		{domaintypes.JobStatusFail, "\x1b[91m"},
		{domaintypes.JobStatusError, "\x1b[91m"},
	} {
		m := joblist.New("JOBS").SetJobs([]domainapi.JobListItem{
			{JobID: "1", DisplayName: "first", Status: tc.status},
			{JobID: "2", DisplayName: "second", Status: tc.status},
		})
		m.SetHeight(24)
		for _, focused := range []bool{false, true} {
			output := m.View(focused, time.Time{}, 0)
			glyph := cliruns.StatusGlyph(tc.status.String(), 0)
			colored := "\x1b[1m" + tc.color + glyph
			if strings.Count(output, colored) != 1 {
				t.Fatalf("status %s focus %t: expected one colored glyph in %q", tc.status, focused, output)
			}
			_, after, _ := strings.Cut(output, colored)
			title, _, _ := strings.Cut(after, "\n")
			if !strings.Contains(ansi.Strip(title), "second") {
				t.Fatalf("status %s focus %t: colored the wrong row: %q", tc.status, focused, title)
			}
		}
	}
}
