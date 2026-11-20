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

func TestRunDetailsRendersOnlyPloy(t *testing.T) {
	m := InitialModel(t.Context(), nil, nil)
	m.screen = ScreenRunDetails
	m.rootList.SetItems([]list.Item{
		listItem{title: "Run", description: "run-abc"},
		listItem{title: "Jobs", description: "total: 8"},
	})

	rendered := m.View().Content
	if !strings.Contains(rendered, "PLOY") {
		t.Fatalf("rendered view missing PLOY title")
	}
	if strings.Contains(rendered, "RUN\n") {
		t.Fatalf("rendered view unexpectedly contains RUN pane title")
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

// Only the active column draws a border; inactive selections retain the same text position.
func TestSelectionBorderFollowsColumnFocus(t *testing.T) {
	for _, tc := range []struct {
		name         string
		screen       Screen
		rootIndex    int
		leftBorders  int
		rightBorders int
	}{
		{"runs preview", ScreenPloyList, runsIndex, 2, 0},
		{"jobs preview", ScreenPloyList, jobsIndex, 2, 0},
		{"users preview", ScreenPloyList, usersIndex, 2, 0},
		{"runs", ScreenRunsList, runsIndex, 0, 3},
		{"jobs", ScreenJobsList, jobsIndex, 0, 2},
		{"users", ScreenUsersList, usersIndex, 0, 2},
		{"run details", ScreenRunDetails, jobsIndex, 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := InitialModel(t.Context(), nil, nil)
			m.screen = tc.screen
			m.usersEnabled = true
			m.usersList.SetItems([]list.Item{userItem{}})
			m.rootList.Select(tc.rootIndex)
			m.jobList = m.jobList.SetJobs([]domainapi.JobListItem{{JobID: "job-1", DisplayName: "deploy│step"}})
			m.rightPaneList = newRunsList("RUNS", []list.Item{runItem{run: domaintypes.RunSummary{ID: "run-1"}}})
			next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
			m = next.(model)
			left, right := 0, 0
			for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
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
				if !strings.Contains(view, "RUNS") || !strings.Contains(view, "run-1") {
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
			if !strings.Contains(focusedRun, "│   run-1") {
				t.Fatalf("Runs selection border was not restored: %q", focusedRun)
			}
			m.screen = ScreenJobsList
			focused := ansi.Strip(m.View().Content)
			if !strings.Contains(focused, "│ job-1") {
				t.Fatal("Jobs selection border was not restored")
			}
		})
	}
}
