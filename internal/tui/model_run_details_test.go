package tui

import (
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func makeS5Model(t *testing.T) model {
	t.Helper()
	m := InitialModel(nil, nil)
	m.screen = ScreenRunsList
	next, _ := m.Update(runsLoadedMsg{runs: []runSummary{
		{
			ID:        domaintypes.RunID("run-abc"),
			MigID:     domaintypes.MigID("mig-abc"),
			MigName:   "my-mig",
			CreatedAt: time.Now(),
		},
	}})
	nm := next.(model)
	nm.secondary.Select(0)
	result, _ := nm.handleEnter()
	return result.(model)
}

func TestS5ScreenSetOnEnterFromS4(t *testing.T) {
	rm := makeS5Model(t)
	if rm.screen != ScreenRunDetails {
		t.Errorf("Enter(S4): got screen %v, want ScreenRunDetails", rm.screen)
	}
}

func TestS5PloyItemsPlaceholder(t *testing.T) {
	rm := makeS5Model(t)
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
		{1, "Run", "run-abc"},
		{2, "Jobs", "total: —"},
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

func TestS5RunDetailsLoadedUpdatesJobsTotalInPloy(t *testing.T) {
	s5m := makeS5Model(t)
	afterLoad, _ := s5m.Update(runDetailsLoadedMsg{repoTotal: 4, jobTotal: 7})
	lm := afterLoad.(model)

	items := lm.ploy.Items()
	if len(items) != 3 {
		t.Fatalf("ploy items count: got %d, want 3", len(items))
	}

	item, ok := items[2].(listItem)
	if !ok {
		t.Fatalf("item 2: unexpected type %T", items[2])
	}
	if item.title != "Jobs" {
		t.Errorf("item 2 title: got %q, want %q", item.title, "Jobs")
	}
	if item.description != "total: 7" {
		t.Errorf("item 2 description: got %q, want %q", item.description, "total: 7")
	}
}

func TestS5EscTransitionsToS4(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenRunDetails

	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != ScreenRunsList {
		t.Errorf("Esc(S5): got screen %v, want ScreenRunsList", nm.screen)
	}
}
