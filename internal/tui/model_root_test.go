package tui

import (
	"testing"
)

// TestS1RootListTitle verifies the PLOY root list title.
func TestS1RootListTitle(t *testing.T) {
	m := InitialModel(nil, nil)
	if m.ploy.Title != "PLOY" {
		t.Errorf("root list title: got %q, want %q", m.ploy.Title, "PLOY")
	}
}

// TestS1RootItemDescriptions verifies the required detail lines for each root item.
func TestS1RootItemDescriptions(t *testing.T) {
	m := InitialModel(nil, nil)
	items := m.ploy.Items()
	if len(items) != 3 {
		t.Fatalf("root items: got %d, want 3", len(items))
	}

	want := []struct{ title, desc string }{
		{"Migrations", "select migration"},
		{"Runs", "select run"},
		{"Jobs", "select job"},
	}
	for i, w := range want {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", i, items[i])
		}
		if item.title != w.title {
			t.Errorf("item %d title: got %q, want %q", i, item.title, w.title)
		}
		if item.description != w.desc {
			t.Errorf("item %d description: got %q, want %q", i, item.description, w.desc)
		}
	}
}

// TestS1FilteringDisabled verifies that filtering/search is disabled on the root list.
func TestS1FilteringDisabled(t *testing.T) {
	m := InitialModel(nil, nil)
	if m.ploy.FilteringEnabled() {
		t.Error("root list: filtering must be disabled")
	}
}

// TestS1EnterSelectsMigrations verifies Enter on index 0 transitions to S2.
func TestS1EnterSelectsMigrations(t *testing.T) {
	m := InitialModel(nil, nil)
	m.ploy.Select(0)
	next, _ := m.handleEnter()
	nm := next.(model)
	if nm.screen != S2MigrationsList {
		t.Errorf("Enter(Migrations): got screen %v, want S2MigrationsList", nm.screen)
	}
}

// TestS1EnterSelectsRuns verifies Enter on index 1 transitions to S4.
func TestS1EnterSelectsRuns(t *testing.T) {
	m := InitialModel(nil, nil)
	m.ploy.Select(1)
	next, _ := m.handleEnter()
	nm := next.(model)
	if nm.screen != S4RunsList {
		t.Errorf("Enter(Runs): got screen %v, want S4RunsList", nm.screen)
	}
}

// TestS1EnterSelectsJobs verifies Enter on index 2 transitions to S6.
func TestS1EnterSelectsJobs(t *testing.T) {
	m := InitialModel(nil, nil)
	m.ploy.Select(2)
	next, _ := m.handleEnter()
	nm := next.(model)
	if nm.screen != S6JobsList {
		t.Errorf("Enter(Jobs): got screen %v, want S6JobsList", nm.screen)
	}
}

// TestS1EscQuits verifies Esc from root issues a quit command.
func TestS1EscQuits(t *testing.T) {
	m := InitialModel(nil, nil)
	_, cmd := m.handleEsc()
	if cmd == nil {
		t.Error("Esc from S1: expected quit cmd, got nil")
	}
}
