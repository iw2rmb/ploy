package tui

import (
	"strings"
	"testing"
)

func TestViewEnablesAltScreen(t *testing.T) {
	tests := []struct {
		name   string
		screen Screen
	}{
		{name: "root", screen: ScreenRoot},
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
		{name: "migration_details", screen: ScreenMigrationDetails, rightTitle: "MIGRATION selected", detailPane: true},
		{name: "runs", screen: ScreenRunsList, rightTitle: "RUNS"},
		{name: "run_details", screen: ScreenRunDetails, rightTitle: "RUN", detailPane: true},
		{name: "jobs", screen: ScreenJobsList, rightTitle: "JOBS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(nil, nil)
			m.screen = tt.screen
			if tt.detailPane {
				m.detail = newList(tt.rightTitle, nil)
			} else {
				m.secondary = newList(tt.rightTitle, nil)
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

func firstLineWith(content string, needle string) int {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	return -1
}
