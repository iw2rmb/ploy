package tui

import (
	"testing"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func makeS3Model(t *testing.T) model {
	t.Helper()
	m := InitialModel(nil, nil)
	m.screen = ScreenMigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []domainapi.MigSummary{
		{ID: domaintypes.MigID("mig-abc"), Name: "my-mig"},
	}})
	nm := next.(model)
	nm.rightPaneList.Select(0)
	result, _ := nm.handleEnter()
	return result.(model)
}

func TestS3MigDetailsLoadedUpdatesRunsTotalInPloy(t *testing.T) {
	s3m := makeS3Model(t)
	afterLoad, _ := s3m.Update(migDetailsLoadedMsg{repoTotal: 5, runTotal: 3})
	lm := afterLoad.(model)

	items := lm.rootList.Items()
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
