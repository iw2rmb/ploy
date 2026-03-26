package tui

import (
	"testing"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func makeS3Model(t *testing.T) model {
	t.Helper()
	m := InitialModel(nil, nil)
	m.screen = ScreenMigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-abc"), Name: "my-mig"},
	}})
	nm := next.(model)
	nm.secondary.Select(0)
	result, _ := nm.handleEnter()
	return result.(model)
}

func TestS3ScreenSetOnEnterFromS2(t *testing.T) {
	rm := makeS3Model(t)
	if rm.screen != ScreenMigrationDetails {
		t.Errorf("Enter(S2): got screen %v, want ScreenMigrationDetails", rm.screen)
	}
}

func TestS3PloyItemsPlaceholder(t *testing.T) {
	rm := makeS3Model(t)
	items := rm.ploy.Items()
	if len(items) != 3 {
		t.Fatalf("ploy items count: got %d, want 3", len(items))
	}

	tests := []struct {
		idx       int
		wantTitle string
		wantDesc  string
	}{
		{0, "my-mig", "mig-abc"},
		{1, "Runs", "total: —"},
		{2, "Jobs", "select job"},
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

func TestS3MigDetailsLoadedUpdatesRunsTotalInPloy(t *testing.T) {
	s3m := makeS3Model(t)
	afterLoad, _ := s3m.Update(migDetailsLoadedMsg{repoTotal: 5, runTotal: 3})
	lm := afterLoad.(model)

	items := lm.ploy.Items()
	if len(items) != 3 {
		t.Fatalf("ploy items count: got %d, want 3", len(items))
	}

	item, ok := items[1].(listItem)
	if !ok {
		t.Fatalf("item 1: unexpected type %T", items[1])
	}
	if item.title != "Runs" {
		t.Errorf("item 1 title: got %q, want %q", item.title, "Runs")
	}
	if item.description != "total: 3" {
		t.Errorf("item 1 description: got %q, want %q", item.description, "total: 3")
	}
}

func TestS3EscTransitionsToS2(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenMigrationDetails

	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != ScreenMigrationsList {
		t.Errorf("Esc(S3): got screen %v, want ScreenMigrationsList", nm.screen)
	}
}
