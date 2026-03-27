package tui

import (
	"testing"

	"github.com/iw2rmb/ploy/internal/tui/joblist"
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

// TestModelPloyListInvariants verifies that the PLOY root list satisfies
// the shared list invariants.
func TestModelPloyListInvariants(t *testing.T) {
	m := InitialModel(nil, nil)

	if m.ploy.Width() != ployListWidth {
		t.Errorf("ploy list width: got %d, want %d", m.ploy.Width(), ployListWidth)
	}

	// Filtering must be disabled on the root PLOY list.
	if m.ploy.FilteringEnabled() {
		t.Error("ploy list: filtering must be disabled")
	}
}

// TestModelPloyListItems verifies the root list has the three required items.
func TestModelPloyListItems(t *testing.T) {
	m := InitialModel(nil, nil)

	items := m.ploy.Items()
	if len(items) != 3 {
		t.Fatalf("ploy list items: got %d, want 3", len(items))
	}

	wantTitles := []string{"Migrations", "Runs", "Jobs"}
	for i, want := range wantTitles {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", i, items[i])
		}
		if item.title != want {
			t.Errorf("item %d title: got %q, want %q", i, item.title, want)
		}
	}
}

// TestModelEscFromS1Quits verifies that pressing Esc from S1 (root) quits.
func TestModelEscFromS1Quits(t *testing.T) {
	m := InitialModel(nil, nil)
	if m.screen != ScreenPloyList {
		t.Fatal("expected ScreenPloyList")
	}

	_, cmd := m.handleEsc()
	if cmd == nil {
		t.Error("expected quit cmd from S1 Esc, got nil")
	}
}

// TestModelEscTransitions verifies Esc key transitions between screens.
func TestModelEscTransitions(t *testing.T) {
	tests := []struct {
		name string
		from Screen
		want Screen
	}{
		{"S2 -> S1", ScreenMigrationsList, ScreenPloyList},
		{"S3 -> S2", ScreenMigrationDetails, ScreenMigrationsList},
		{"S4 -> S1", ScreenRunsList, ScreenPloyList},
		{"S5 -> S4", ScreenRunDetails, ScreenRunsList},
		{"S6 -> S1", ScreenJobsList, ScreenPloyList},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := InitialModel(nil, nil)
			m.screen = tt.from
			next, _ := m.handleEsc()
			nm, ok := next.(model)
			if !ok {
				t.Fatal("Update did not return model")
			}
			if nm.screen != tt.want {
				t.Errorf("screen after Esc: got %v, want %v", nm.screen, tt.want)
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
			m.ploy.Select(tt.index)
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

func TestJobListInvariants(t *testing.T) {
	jl := joblist.New("JOBS")

	if jl.Width() != joblist.ListWidth {
		t.Errorf("jobList width: got %d, want %d", jl.Width(), joblist.ListWidth)
	}
	if jl.Title() != "JOBS" {
		t.Errorf("jobList title: got %q, want %q", jl.Title(), "JOBS")
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

func TestNewPloyListInvariants(t *testing.T) {
	l := newPloyList("PLOY", nil)

	if l.Width() != ployListWidth {
		t.Errorf("ploy list width: got %d, want %d", l.Width(), ployListWidth)
	}
	if l.Title != "PLOY" {
		t.Errorf("ploy list title: got %q, want %q", l.Title, "PLOY")
	}
}
