package tui

import (
	"testing"
)

// TestModelInit verifies that InitialModel produces a well-formed starting state.
func TestModelInit(t *testing.T) {
	m := InitialModel(nil, nil)

	if m.screen != ScreenPloyList {
		t.Errorf("initial screen: got %v, want ScreenPloyList", m.screen)
	}

	// Init must return nil for the base shell (no async commands on start).
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init: expected nil cmd for base shell")
	}
}

// TestModelEscTransitions verifies Esc key transitions between screens.
func TestModelEscTransitions(t *testing.T) {
	tests := []struct {
		name     string
		from     Screen
		want     Screen
		wantQuit bool
	}{
		{"S1 quits", ScreenPloyList, ScreenPloyList, true},
		{"S2 -> S1", ScreenMigrationsList, ScreenPloyList, false},
		{"S3 -> S2", ScreenMigrationDetails, ScreenMigrationsList, false},
		{"S4 -> S1", ScreenRunsList, ScreenPloyList, false},
		{"S5 -> S4", ScreenRunDetails, ScreenRunsList, false},
		{"S6 -> S1", ScreenJobsList, ScreenPloyList, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(nil, nil)
			m.screen = tt.from
			next, cmd := m.handleEsc()
			nm, ok := next.(model)
			if !ok {
				t.Fatal("Update did not return model")
			}
			if nm.screen != tt.want {
				t.Errorf("screen after Esc: got %v, want %v", nm.screen, tt.want)
			}
			if gotQuit := cmd != nil; gotQuit != tt.wantQuit {
				t.Errorf("quit command after Esc: got %v, want %v", gotQuit, tt.wantQuit)
			}
		})
	}
}

// TestModelEnterTransitionsFromRoot verifies Enter from S1 transitions to the
// correct secondary screen based on selected root item index.
func TestModelEnterTransitionsFromRoot(t *testing.T) {
	tests := []struct {
		name  string
		index int
		want  Screen
	}{
		{"Migrations", 0, ScreenMigrationsList},
		{"Runs", 1, ScreenRunsList},
		{"Jobs", 2, ScreenJobsList},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(nil, nil)
			m.rootList.Select(tt.index)
			next, _ := m.handleEnter()
			nm, ok := next.(model)
			if !ok {
				t.Fatal("handleEnter did not return model")
			}
			if nm.screen != tt.want {
				t.Errorf("screen after Enter(%s): got %v, want %v", tt.name, nm.screen, tt.want)
			}
		})
	}
}

// TestNewListInvariants verifies that newList enforces width=24 and help=false.
func TestNewListInvariants(t *testing.T) {
	l := newList("TEST", nil)

	if l.Width() != listWidth {
		t.Errorf("list width: got %d, want %d", l.Width(), listWidth)
	}
	if l.Title != "TEST" {
		t.Errorf("list title: got %q, want %q", l.Title, "TEST")
	}
}

func TestNewRunsListInvariants(t *testing.T) {
	l := newRunsList("RUNS", nil)

	if l.Width() != runsListWidth {
		t.Errorf("runs list width: got %d, want %d", l.Width(), runsListWidth)
	}
	if l.Title != "RUNS" {
		t.Errorf("runs list title: got %q, want %q", l.Title, "RUNS")
	}
}

func assertPloyItems(t *testing.T, m model, want []listItem) {
	t.Helper()
	items := m.rootList.Items()
	if len(items) != len(want) {
		t.Fatalf("PLOY items count = %d, want %d", len(items), len(want))
	}
	for i, expected := range want {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("PLOY item %d has type %T, want listItem", i, items[i])
		}
		if item.title != expected.title || item.description != expected.description {
			t.Errorf("PLOY item %d = %q/%q, want %q/%q", i, item.title, item.description, expected.title, expected.description)
		}
	}
}

func TestNewPloyListInvariants(t *testing.T) {
	l := newPloyList("PLOY", nil)

	if l.Width() != ployListWidth {
		t.Errorf("ploy list width: got %d, want %d", l.Width(), ployListWidth)
	}
	if l.Title != "PLOY" {
		t.Errorf("ploy list title: got %q, want %q", l.Title, "PLOY")
	}
}
