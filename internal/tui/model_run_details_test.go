package tui

import (
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func makeS5Model(t *testing.T) model {
	t.Helper()
	m := InitialModel(nil, nil)
	m.screen = S4RunsList
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{ID: domaintypes.RunID("run-abc"), MigName: "my-mig", CreatedAt: time.Now()},
	}})
	nm := next.(model)
	nm.secondary.Select(0)
	result, _ := nm.handleEnter()
	return result.(model)
}

// TestS5ScreenSetOnEnterFromS4 verifies the screen transitions to S5 on Enter from S4.
func TestS5ScreenSetOnEnterFromS4(t *testing.T) {
	rm := makeS5Model(t)
	if rm.screen != S5RunDetails {
		t.Errorf("Enter(S4): got screen %v, want S5RunDetails", rm.screen)
	}
}

// TestS5DetailListTitle verifies the detail list title is "RUN" after entering S5.
func TestS5DetailListTitle(t *testing.T) {
	rm := makeS5Model(t)
	if rm.detail.Title != "RUN" {
		t.Errorf("detail title: got %q, want %q", rm.detail.Title, "RUN")
	}
}

// TestS5DetailItemsPlaceholder verifies placeholder items before data loads.
func TestS5DetailItemsPlaceholder(t *testing.T) {
	rm := makeS5Model(t)
	items := rm.detail.Items()
	if len(items) != 2 {
		t.Fatalf("detail items count: got %d, want 2", len(items))
	}
	wantTitles := []string{"Repositories", "Jobs"}
	for i, want := range wantTitles {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", i, items[i])
		}
		if item.title != want {
			t.Errorf("item %d title: got %q, want %q", i, item.title, want)
		}
		if item.description != "total: —" {
			t.Errorf("item %d description: got %q, want %q", i, item.description, "total: —")
		}
	}
}

// TestS5RunDetailsLoadedUpdatesItems verifies runDetailsLoadedMsg updates totals.
func TestS5RunDetailsLoadedUpdatesItems(t *testing.T) {
	s5m := makeS5Model(t)
	afterLoad, _ := s5m.Update(runDetailsLoadedMsg{repoTotal: 4, jobTotal: 7})
	lm := afterLoad.(model)

	items := lm.detail.Items()
	if len(items) != 2 {
		t.Fatalf("detail items count: got %d, want 2", len(items))
	}

	tests := []struct {
		idx       int
		wantTitle string
		wantDesc  string
	}{
		{0, "Repositories", "total: 4"},
		{1, "Jobs", "total: 7"},
	}
	for _, tt := range tests {
		item, ok := items[tt.idx].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", tt.idx, items[tt.idx])
		}
		if item.title != tt.wantTitle {
			t.Errorf("item %d title: got %q, want %q", tt.idx, item.title, tt.wantTitle)
		}
		if item.description != tt.wantDesc {
			t.Errorf("item %d description: got %q, want %q", tt.idx, item.description, tt.wantDesc)
		}
	}
}

// TestS5EscTransitionsToS4 verifies Esc from S5 returns to S4.
func TestS5EscTransitionsToS4(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S5RunDetails

	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != S4RunsList {
		t.Errorf("Esc(S5): got screen %v, want S4RunsList", nm.screen)
	}
}
