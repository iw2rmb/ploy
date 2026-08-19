package tui

import (
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestS2MigrationsListTitle verifies the MIGRATIONS secondary list title.
func TestS2MigrationsListTitle(t *testing.T) {
	m := InitialModel(nil, nil)
	next, _ := m.Update(migsLoadedMsg{migs: []domainapi.MigSummary{
		{ID: domaintypes.MigID("mig-1"), Name: "alpha"},
	}})
	nm := next.(model)
	if nm.rightPaneList.Title != "MIGRATIONS" {
		t.Errorf("secondary list title: got %q, want %q", nm.rightPaneList.Title, "MIGRATIONS")
	}
}

// TestS2MigrationsItemsPopulated verifies migration rows use name as title and ID as description.
func TestS2MigrationsItemsPopulated(t *testing.T) {
	m := InitialModel(nil, nil)
	// Provide distinct CreatedAt values so sort order is deterministic.
	migs := []domainapi.MigSummary{
		{ID: domaintypes.MigID("mig-aaa"), Name: "alpha", CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ID: domaintypes.MigID("mig-bbb"), Name: "beta", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	next, _ := m.Update(migsLoadedMsg{migs: migs})
	nm := next.(model)

	items := nm.rightPaneList.Items()
	if len(items) != 2 {
		t.Fatalf("secondary items: got %d, want 2", len(items))
	}

	// After sorting newest-to-oldest, migs[0] (newer) stays first.
	for i, mig := range migs {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", i, items[i])
		}
		if item.title != mig.Name {
			t.Errorf("item %d title: got %q, want %q", i, item.title, mig.Name)
		}
		if item.description != mig.ID.String() {
			t.Errorf("item %d description: got %q, want %q", i, item.description, mig.ID.String())
		}
	}
}

// TestS2MigrationsOrderingEnforced verifies that items are sorted newest-to-oldest by
// CreatedAt regardless of the order received from the API.
func TestS2MigrationsOrderingEnforced(t *testing.T) {
	m := InitialModel(nil, nil)
	// Provide items intentionally out of order (oldest first).
	migs := []domainapi.MigSummary{
		{ID: domaintypes.MigID("mig-oldest"), Name: "oldest", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: domaintypes.MigID("mig-middle"), Name: "middle", CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{ID: domaintypes.MigID("mig-newest"), Name: "newest", CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)},
	}
	next, _ := m.Update(migsLoadedMsg{migs: migs})
	nm := next.(model)

	items := nm.rightPaneList.Items()
	if len(items) != 3 {
		t.Fatalf("items count: got %d, want 3", len(items))
	}

	wantOrder := []string{"newest", "middle", "oldest"}
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

func TestMigrationSelectionUpdatesContext(t *testing.T) {
	rm := makeS3Model(t)
	if rm.screen != ScreenMigrationDetails {
		t.Errorf("Enter(S2): got screen %v, want ScreenMigrationDetails", rm.screen)
	}
	if rm.selectedMigID != "mig-abc" {
		t.Errorf("selectedMigID: got %q, want %q", rm.selectedMigID, "mig-abc")
	}
	assertPloyItems(t, rm, []listItem{
		{title: "my-mig", description: "mig-abc"},
		{title: "Runs", description: "total: —"},
		{title: "Jobs", description: "select job"},
	})
}
