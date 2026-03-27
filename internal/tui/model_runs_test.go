package tui

import (
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestS4RunsListTitle verifies the RUNS secondary list title.
func TestS4RunsListTitle(t *testing.T) {
	m := InitialModel(nil, nil)
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{ID: domaintypes.RunID("run-1"), MigName: "alpha", CreatedAt: time.Now()},
	}})
	nm := next.(model)
	if nm.secondary.Title != "RUNS" {
		t.Errorf("secondary list title: got %q, want %q", nm.secondary.Title, "RUNS")
	}
	if nm.secondary.Width() != runsListWidth {
		t.Errorf("secondary list width: got %d, want %d", nm.secondary.Width(), runsListWidth)
	}
}

// TestS4RunsItemsPopulated verifies run rows use run ID as title and include migration name and timestamp in description.
func TestS4RunsItemsPopulated(t *testing.T) {
	ts := time.Date(2024, 3, 15, 9, 5, 0, 0, time.UTC)
	m := InitialModel(nil, nil)
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{ID: domaintypes.RunID("run-abc"), MigName: "my-mig", CreatedAt: ts},
	}})
	nm := next.(model)

	items := nm.secondary.Items()
	if len(items) != 1 {
		t.Fatalf("secondary items: got %d, want 1", len(items))
	}

	item, ok := items[0].(listItem)
	if !ok {
		t.Fatalf("item 0: unexpected type %T", items[0])
	}
	if item.title != "run-abc" {
		t.Errorf("item title: got %q, want %q", item.title, "run-abc")
	}
	if !strings.Contains(item.description, "my-mig") {
		t.Errorf("item description %q: missing migration name %q", item.description, "my-mig")
	}
	wantTS := "15 03 09:05"
	if !strings.Contains(item.description, wantTS) {
		t.Errorf("item description %q: missing timestamp %q", item.description, wantTS)
	}
}

// TestS4RunsOrderingEnforced verifies items are sorted newest-to-oldest by CreatedAt.
func TestS4RunsOrderingEnforced(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	m := InitialModel(nil, nil)
	// Provide items intentionally out of order (oldest first).
	runs := []runSummary{
		{ID: domaintypes.RunID("run-oldest"), MigName: "m", CreatedAt: base},
		{ID: domaintypes.RunID("run-middle"), MigName: "m", CreatedAt: base.Add(24 * time.Hour)},
		{ID: domaintypes.RunID("run-newest"), MigName: "m", CreatedAt: base.Add(48 * time.Hour)},
	}
	next, _ := m.Update(runsLoadedMsg{runs: runs})
	nm := next.(model)

	items := nm.secondary.Items()
	if len(items) != 3 {
		t.Fatalf("items count: got %d, want 3", len(items))
	}

	wantOrder := []string{"run-newest", "run-middle", "run-oldest"}
	for i, want := range wantOrder {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", i, items[i])
		}
		if item.title != want {
			t.Errorf("ordering: item %d title: got %q, want %q", i, item.title, want)
		}
	}
}

// TestS4EnterTransitionsToS5 verifies Enter on a selected run transitions to S5.
func TestS4EnterTransitionsToS5(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenRunsList
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{ID: domaintypes.RunID("run-xyz"), MigName: "mig", CreatedAt: time.Now()},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	if rm.screen != ScreenRunDetails {
		t.Errorf("Enter(S4): got screen %v, want ScreenRunDetails", rm.screen)
	}
}

// TestS4EnterSetsSelectedRunID verifies selectedRunID is set from the chosen run's ID.
func TestS4EnterSetsSelectedRunID(t *testing.T) {
	const wantID = "run-xyz"
	m := InitialModel(nil, nil)
	m.screen = ScreenRunsList
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{ID: domaintypes.RunID(wantID), MigName: "mig", CreatedAt: time.Now()},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	if rm.selectedRunID.String() != wantID {
		t.Errorf("selectedRunID: got %q, want %q", rm.selectedRunID.String(), wantID)
	}
}

func TestS4EnterDefinesMigrationAndRunInPloy(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenRunsList
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{ID: domaintypes.RunID("run-xyz"), MigID: domaintypes.MigID("mig-123"), MigName: "mig", CreatedAt: time.Now()},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	items := rm.ploy.Items()
	if len(items) != 3 {
		t.Fatalf("ploy items: got %d, want 3", len(items))
	}

	item0, ok := items[0].(listItem)
	if !ok {
		t.Fatalf("item 0: unexpected type %T", items[0])
	}
	item1, ok := items[1].(listItem)
	if !ok {
		t.Fatalf("item 1: unexpected type %T", items[1])
	}
	item2, ok := items[2].(listItem)
	if !ok {
		t.Fatalf("item 2: unexpected type %T", items[2])
	}

	if item0.title != "mig" {
		t.Errorf("item 0 title: got %q, want %q", item0.title, "mig")
	}
	if item0.description != "mig-123" {
		t.Errorf("item 0 description: got %q, want %q", item0.description, "mig-123")
	}
	if item1.title != "Run" {
		t.Errorf("item 1 title: got %q, want %q", item1.title, "Run")
	}
	if item1.description != "run-xyz" {
		t.Errorf("item 1 description: got %q, want %q", item1.description, "run-xyz")
	}
	if item2.title != "Jobs" {
		t.Errorf("item 2 title: got %q, want %q", item2.title, "Jobs")
	}
	if item2.description != "total: —" {
		t.Errorf("item 2 description: got %q, want %q", item2.description, "total: —")
	}
}

// TestS4EscTransitionsToS1 verifies Esc from S4 returns to S1.
func TestS4EscTransitionsToS1(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenRunsList
	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != ScreenPloyList {
		t.Errorf("Esc(S4): got screen %v, want ScreenPloyList", nm.screen)
	}
}
