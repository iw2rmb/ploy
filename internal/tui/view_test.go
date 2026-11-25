package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestViewEnablesAltScreen(t *testing.T) {
	tests := []struct {
		name   string
		screen Screen
	}{
		{name: "root", screen: ScreenPloyList},
		{name: "runs_list", screen: ScreenRunsList},
		{name: "run_details", screen: ScreenRunDetails},
		{name: "jobs_list", screen: ScreenJobsList},
		{name: "users_list", screen: ScreenUsersList},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(t.Context(), nil, nil)
			m.screen = tt.screen
			m.usersEnabled = true

			view := m.View()
			if !view.AltScreen {
				t.Fatalf("View().AltScreen = false for %s; want true", tt.name)
			}
		})
	}
}

func TestSplitScreensRenderColumns(t *testing.T) {
	tests := []struct {
		name       string
		screen     Screen
		rightTitle string
	}{
		{name: "runs", screen: ScreenRunsList, rightTitle: "RUNS"},
		{name: "jobs", screen: ScreenJobsList, rightTitle: "JOBS"},
		{name: "users", screen: ScreenUsersList, rightTitle: "USERS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(t.Context(), nil, nil)
			m.screen = tt.screen
			m.usersEnabled = true
			m.rightPaneList = newRunsList(tt.rightTitle, nil)

			rendered := m.View().Content
			ployLine := firstLineWith(rendered, "PLOY")
			rightLine := firstLineWith(rendered, tt.rightTitle)

			if ployLine < 0 {
				t.Fatalf("rendered view missing PLOY title")
			}
			if rightLine < 0 {
				t.Fatalf("rendered view missing %s title", tt.rightTitle)
			}
			if ployLine != rightLine {
				t.Fatalf("column titles misaligned: PLOY line=%d %s line=%d", ployLine, tt.rightTitle, rightLine)
			}
		})
	}
}

// RUN follows the highlighted run in each focus state.
func TestRunPreviewShowsSelectedFields(t *testing.T) {
	run := runItem{run: domaintypes.RunSummary{
		ID: "run-abc", Status: domaintypes.RunStatusSuccess, SpecName: "spec",
		SpecSourceDomain: "host", SpecSourceRepo: "group/specs",
		RepoURL: "https://github.com/group/repo.git", SourceCommitSHA: strings.Repeat("a", 40),
	}}
	for _, screen := range []Screen{ScreenPloyList, ScreenRunsList, ScreenRunDetails} {
		m := InitialModel(t.Context(), nil, nil)
		m.screen = screen
		m.rightPaneList.SetItems([]list.Item{run})
		if screen == ScreenRunDetails {
			m.selectedRunID = run.run.ID
			m.rootList.SetItems([]list.Item{run, m.runJobsEntry()})
		}
		next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
		m = next.(model)
		m.runDetails.nodeName = "node-alpha"
		view := ansi.Strip(m.View().Content)
		if got := strings.Contains(view, "RUNS"); got != (screen != ScreenRunDetails) {
			t.Fatalf("screen %d shows RUNS = %v", screen, got)
		}
		col := ployListWidth + 2
		if screen != ScreenRunDetails {
			col += runsListWidth + 2
		}
		var fields []string
		for _, line := range strings.Split(view, "\n") {
			if field := strings.TrimSpace(ansi.Cut(line, col, 120)); field != "" {
				fields = append(fields, field)
			}
		}
		want := "RUN|5 items|ID|run-abc|STATUS|Success|SPEC|host/group/specs:spec|REPO|group/repo:aaaaaaaa|NODE|node-alpha"
		if got := strings.Join(fields, "|"); got != want {
			t.Fatalf("screen %d: got %q, want %q", screen, got, want)
		}
	}
}

// Web links target the full commit and discard clone credentials.
func TestRunCommitURL(t *testing.T) {
	sha := strings.Repeat("a", 40)
	for _, tc := range []struct{ repo, sha, want string }{
		{"https://user:secret@github.com/team/repo.git", sha, "https://github.com/team/repo/commit/" + sha},
		{"ssh://git@gitlab.example:2222/team/sub/repo.git", sha, "https://gitlab.example/team/sub/repo/-/commit/" + sha},
		{"git@gitlab.example:team/repo.git", sha, "https://gitlab.example/team/repo/-/commit/" + sha},
		{"file:///tmp/repo", sha, ""}, {"https://github.com/team/repo", "", ""},
	} {
		if got := runCommitURL(tc.repo, tc.sha); got != tc.want {
			t.Errorf("commit URL = %q, want %q", got, tc.want)
		}
	}
}

func firstLineWith(content string, needle string) int {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}

// RUNS starts after the full PLOY column even when its labels are short.
func TestRunsPaneStartsAfterFixedPloyWidth(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.screen = ScreenRunsList
	rendered := m.View().Content
	lineIndex := firstLineWith(rendered, "RUNS")
	if lineIndex < 0 {
		t.Fatal("view has no RUNS title")
	}
	line := strings.Split(rendered, "\n")[lineIndex]
	runLineIndex := firstLineWith(m.rightPaneList.View(), "RUNS")
	runLine := strings.Split(m.rightPaneList.View(), "\n")[runLineIndex]
	offset := lipgloss.Width(line[:strings.Index(line, "RUNS")]) - lipgloss.Width(runLine[:strings.Index(runLine, "RUNS")])
	if offset != ployListWidth+2 {
		t.Fatalf("RUNS offset = %d, want %d", offset, ployListWidth+2)
	}
}

// The title background and selection border identify the focused column.
func TestSelectionBorderAndTitleFollowColumnFocus(t *testing.T) {
	for _, tc := range []struct {
		name         string
		screen       Screen
		rootIndex    int
		leftBorders  int
		rightBorders int
	}{
		{"runs preview", ScreenPloyList, runsIndex, 3, 0},
		{"jobs preview", ScreenPloyList, jobsIndex, 3, 0},
		{"users preview", ScreenPloyList, usersIndex, 3, 0},
		{"runs", ScreenRunsList, runsIndex, 0, 3},
		{"jobs", ScreenJobsList, jobsIndex, 0, 3},
		{"users", ScreenUsersList, usersIndex, 0, 2},
		{"run details", ScreenRunDetails, jobsIndex, 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := InitialModel(t.Context(), nil, nil)
			m.screen = tc.screen
			m.usersEnabled = true
			m.usersList.SetItems([]list.Item{userItem{}})
			m.rootList.Select(tc.rootIndex)
			m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "job-1", DisplayName: "deploy│step", Status: domaintypes.JobStatusSuccess}})
			m.rightPaneList = newRunsList("RUNS", []list.Item{runItem{run: domaintypes.RunSummary{ID: "run-1", SpecName: "spec-1"}}})
			next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
			m = next.(model)
			rendered := m.View().Content
			titleStyle := list.DefaultStyles(true).Title
			focusedTitle := "PLOY"
			if tc.rightBorders > 0 {
				focusedTitle = []string{"RUNS", "JOBS", "USERS"}[tc.rootIndex]
			}
			for _, title := range []string{"PLOY", "RUNS", "JOBS", "USERS"} {
				focused := title == focusedTitle
				if got := strings.Contains(rendered, titleStyle.Render(title)); got != focused {
					t.Errorf("%s title has focused style = %v, want %v", title, got, focused)
				}
			}
			left, right := 0, 0
			for _, line := range strings.Split(ansi.Strip(rendered), "\n") {
				if ansi.Cut(line, 0, 1) == "│" {
					left++
				}
				if ansi.Cut(line, ployListWidth+2, ployListWidth+3) == "│" {
					right++
				}
			}
			if left != tc.leftBorders || right != tc.rightBorders {
				t.Fatalf("border cells = %d/%d, want %d/%d", left, right, tc.leftBorders, tc.rightBorders)
			}
			if tc.screen == ScreenRunsList || tc.name == "runs preview" {
				view := ansi.Strip(m.View().Content)
				if !strings.Contains(view, "RUNS") || !strings.Contains(view, "spec-1") {
					t.Fatalf("Runs pane missing: %q", view)
				}
			}
			if tc.screen == ScreenJobsList || tc.name == "jobs preview" {
				view := ansi.Strip(m.View().Content)
				if !strings.Contains(view, "deploy│step") {
					t.Fatalf("job text changed: %q", view)
				}
			}
			// Rendering inactive columns must not alter their later focused appearance.
			m.screen = ScreenPloyList
			if !strings.Contains(ansi.Strip(m.View().Content), "│ "+m.rootList.SelectedItem().(listItem).Title()) {
				t.Fatal("PLOY selection border was not restored")
			}
			m.screen = ScreenRunsList
			focusedRun := ansi.Strip(m.View().Content)
			if !strings.Contains(focusedRun, "│   spec-1") {
				t.Fatalf("Runs selection border was not restored: %q", focusedRun)
			}
			m.screen = ScreenJobsList
			focused := ansi.Strip(m.View().Content)
			if !strings.Contains(focused, "│ ✓ deploy│step") {
				t.Fatal("Jobs selection border was not restored")
			}
		})
	}
}

// The list keeps the commit target when it truncates the repository label.
func TestRunListTruncatesLabelWithFullCommitTarget(t *testing.T) {
	sha := strings.Repeat("b", 40)
	run := domaintypes.RunSummary{RepoURL: "https://github.com/group/repository-name", SourceCommitSHA: sha}
	details := InitialModel(t.Context(), nil, nil).runDetailsList
	details.SetSize(15, 30)
	details.SetItems(runDetailItems(run, "", true))
	rendered := renderList(details, false)
	if !strings.Contains(rendered, "https://github.com/group/repository-name/commit/"+sha) || !strings.Contains(ansi.Strip(rendered), "group/repo") {
		t.Fatalf("list link: %q", rendered)
	}
}

// A filter with no matching run leaves the RUNS list visible.
func TestRunPreviewHiddenWhenFilterHasNoMatches(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.rightPaneList.SetItems([]list.Item{runItem{run: domaintypes.RunSummary{ID: "run-abc"}}})
	m.rightPaneList.SetFilterText("unmatched")
	for _, screen := range []Screen{ScreenPloyList, ScreenRunsList} {
		m.screen = screen
		rendered := ansi.Strip(m.View().Content)
		if !strings.Contains(rendered, "RUNS") || strings.Contains(rendered, "STATUS") {
			t.Fatalf("screen %d shows unexpected panes: %q", screen, rendered)
		}
	}
}
