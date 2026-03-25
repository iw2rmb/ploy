package tui

import "testing"

func TestViewEnablesAltScreen(t *testing.T) {
	tests := []struct {
		name   string
		screen Screen
	}{
		{name: "root", screen: S1Root},
		{name: "migrations_list", screen: S2MigrationsList},
		{name: "migration_details", screen: S3MigrationDetails},
		{name: "runs_list", screen: S4RunsList},
		{name: "run_details", screen: S5RunDetails},
		{name: "jobs_list", screen: S6JobsList},
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
