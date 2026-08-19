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
	if nm.rightPaneList.Title != "RUNS" {
		t.Errorf("secondary list title: got %q, want %q", nm.rightPaneList.Title, "RUNS")
	}
	if nm.rightPaneList.Width() != runsListWidth {
		t.Errorf("secondary list width: got %d, want %d", nm.rightPaneList.Width(), runsListWidth)
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

	items := nm.rightPaneList.Items()
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

	items := nm.rightPaneList.Items()
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

func TestRunSelectionUpdatesContext(t *testing.T) {
	rm := makeS5Model(t)
	if rm.screen != ScreenRunDetails {
		t.Errorf("Enter(S4): got screen %v, want ScreenRunDetails", rm.screen)
	}
	if rm.selectedRunID != "run-abc" {
		t.Errorf("selectedRunID: got %q, want %q", rm.selectedRunID, "run-abc")
	}
	assertPloyItems(t, rm, []listItem{
		{title: "my-mig", description: "mig-abc"},
		{title: "Run", description: "run-abc"},
		{title: "Jobs", description: "total: —"},
	})
}
