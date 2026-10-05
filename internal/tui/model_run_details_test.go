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
	nm.rightPaneList.Select(0)
	result, _ := nm.handleEnter()
	return result.(model)
}

func TestS5RunDetailsLoadedUpdatesJobsTotalInPloy(t *testing.T) {
	s5m := makeS5Model(t)
	afterLoad, _ := s5m.Update(runDetailsLoadedMsg{jobTotal: 7})
	lm := afterLoad.(model)

	items := lm.rootList.Items()
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
