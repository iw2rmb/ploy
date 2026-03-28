package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
)

func TestViewEnablesAltScreen(t *testing.T) {
	tests := []struct {
		name   string
		screen Screen
	}{
		{name: "root", screen: ScreenPloyList},
		{name: "migrations_list", screen: ScreenMigrationsList},
		{name: "migration_details", screen: ScreenMigrationDetails},
		{name: "runs_list", screen: ScreenRunsList},
		{name: "run_details", screen: ScreenRunDetails},
		{name: "jobs_list", screen: ScreenJobsList},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(nil, nil)
			m.screen = tt.screen

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
		detailPane bool
	}{
		{name: "migrations", screen: ScreenMigrationsList, rightTitle: "MIGRATIONS"},
		{name: "runs", screen: ScreenRunsList, rightTitle: "RUNS"},
		{name: "jobs", screen: ScreenJobsList, rightTitle: "JOBS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(nil, nil)
			m.screen = tt.screen
			if tt.detailPane {
				m.detailsList = newList(tt.rightTitle, nil)
			} else {
				m.rightPaneList = newList(tt.rightTitle, nil)
			}

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

func TestMigrationDetailsRendersOnlyPloy(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenMigrationDetails
	m.rootList.SetItems([]list.Item{
		listItem{title: "my-mig", description: "mig-abc"},
		listItem{title: "Runs", description: "total: 1"},
		listItem{title: "Jobs", description: "select job"},
	})

	rendered := m.View().Content
	if !strings.Contains(rendered, "PLOY") {
		t.Fatalf("rendered view missing PLOY title")
	}
	if strings.Contains(rendered, "MIGRATION ") {
		t.Fatalf("rendered view unexpectedly contains MIGRATION pane title")
	}
}

func TestRunDetailsRendersOnlyPloy(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenRunDetails
	m.rootList.SetItems([]list.Item{
		listItem{title: "my-mig", description: "mig-abc"},
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
