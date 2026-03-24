package tui

import (
	"testing"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestS3DetailListTitle verifies the detail list title is "MIGRATION <name>" after entering S3.
func TestS3DetailListTitle(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S2MigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-abc"), Name: "my-mig"},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	want := "MIGRATION my-mig"
	if rm.detail.Title != want {
		t.Errorf("detail title: got %q, want %q", rm.detail.Title, want)
	}
}

// TestS3DetailListTitleFallbackToID verifies that when migration name is empty, the title falls back to "MIGRATION <id>".
func TestS3DetailListTitleFallbackToID(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S2MigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-xyz"), Name: ""},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	want := "MIGRATION mig-xyz"
	if rm.detail.Title != want {
		t.Errorf("detail title: got %q, want %q", rm.detail.Title, want)
	}
}

// TestS3DetailItemsPlaceholder verifies that detail items use placeholder totals before data loads.
func TestS3DetailItemsPlaceholder(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S2MigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-abc"), Name: "my-mig"},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)

	items := rm.detail.Items()
	if len(items) != 2 {
		t.Fatalf("detail items count: got %d, want 2", len(items))
	}
	wantTitles := []string{"repositories", "runs"}
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

// TestS3MigDetailsLoadedUpdatesItems verifies migDetailsLoadedMsg updates totals.
func TestS3MigDetailsLoadedUpdatesItems(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S2MigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-abc"), Name: "my-mig"},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	afterEnter, _ := nm.handleEnter()
	s3m := afterEnter.(model)

	afterLoad, _ := s3m.Update(migDetailsLoadedMsg{repoTotal: 5, runTotal: 3})
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
		{0, "repositories", "total: 5"},
		{1, "runs", "total: 3"},
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

// TestS3EscTransitionsToS2 verifies Esc from S3 returns to S2.
func TestS3EscTransitionsToS2(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S3MigrationDetails

	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != S2MigrationsList {
		t.Errorf("Esc(S3): got screen %v, want S2MigrationsList", nm.screen)
	}
}

// TestS3ScreenSetOnEnterFromS2 verifies the screen transitions to S3 on Enter from S2.
func TestS3ScreenSetOnEnterFromS2(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S2MigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-abc"), Name: "my-mig"},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	if rm.screen != S3MigrationDetails {
		t.Errorf("Enter(S2): got screen %v, want S3MigrationDetails", rm.screen)
	}
}
