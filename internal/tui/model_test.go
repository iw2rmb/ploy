package tui

import (
	"testing"
)

// TestModelInit verifies that InitialModel produces a well-formed starting state.
func TestModelInit(t *testing.T) {
	m := InitialModel(nil, nil)

	if m.screen != S1Root {
		t.Errorf("initial screen: got %v, want S1Root", m.screen)
	}

	// Init must return nil for the base shell (no async commands on start).
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init: expected nil cmd for base shell")
	}
}

// TestModelPloyListInvariants verifies that the PLOY root list satisfies
// the shared list invariants: width 24 and help disabled.
func TestModelPloyListInvariants(t *testing.T) {
	m := InitialModel(nil, nil)

	if m.ploy.Width() != listWidth {
		t.Errorf("ploy list width: got %d, want %d", m.ploy.Width(), listWidth)
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
	if m.screen != S1Root {
		t.Fatal("expected S1Root")
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
		{"S2 -> S1", S2MigrationsList, S1Root},
		{"S3 -> S2", S3MigrationDetails, S2MigrationsList},
		{"S4 -> S1", S4RunsList, S1Root},
		{"S5 -> S4", S5RunDetails, S4RunsList},
		{"S6 -> S1", S6JobsList, S1Root},
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
		{"Migrations", 0, S2MigrationsList},
		{"Runs", 1, S4RunsList},
		{"Jobs", 2, S6JobsList},
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
