package tui

import (
	"testing"

	clitui "github.com/iw2rmb/ploy/internal/cli/tui"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestS2MigrationsListTitle verifies the MIGRATIONS secondary list title.
func TestS2MigrationsListTitle(t *testing.T) {
	m := InitialModel(nil, nil)
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-1"), Name: "alpha"},
	}})
	nm := next.(model)
	if nm.secondary.Title != "MIGRATIONS" {
		t.Errorf("secondary list title: got %q, want %q", nm.secondary.Title, "MIGRATIONS")
	}
}

// TestS2MigrationsItemsPopulated verifies migration rows use name as title and ID as description.
func TestS2MigrationsItemsPopulated(t *testing.T) {
	m := InitialModel(nil, nil)
	// Provide distinct CreatedAt values so sort order is deterministic.
	migs := []clitui.MigItem{
		{ID: domaintypes.MigID("mig-aaa"), Name: "alpha", CreatedAt: "2024-01-02T00:00:00Z"},
		{ID: domaintypes.MigID("mig-bbb"), Name: "beta", CreatedAt: "2024-01-01T00:00:00Z"},
	}
	next, _ := m.Update(migsLoadedMsg{migs: migs})
	nm := next.(model)

	items := nm.secondary.Items()
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
	migs := []clitui.MigItem{
		{ID: domaintypes.MigID("mig-oldest"), Name: "oldest", CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: domaintypes.MigID("mig-middle"), Name: "middle", CreatedAt: "2024-01-02T00:00:00Z"},
		{ID: domaintypes.MigID("mig-newest"), Name: "newest", CreatedAt: "2024-01-03T00:00:00Z"},
	}
	next, _ := m.Update(migsLoadedMsg{migs: migs})
	nm := next.(model)

	items := nm.secondary.Items()
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

// TestS2EnterTransitionsToS3 verifies Enter on a selected migration transitions to S3.
func TestS2EnterTransitionsToS3(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenMigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-xyz"), Name: "my-migration"},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	if rm.screen != ScreenMigrationDetails {
		t.Errorf("Enter(S2): got screen %v, want ScreenMigrationDetails", rm.screen)
	}
}

// TestS2EnterSetsSelectedMigID verifies selectedMigID is set from the chosen migration's ID.
func TestS2EnterSetsSelectedMigID(t *testing.T) {
	const wantID = "mig-xyz"
	m := InitialModel(nil, nil)
	m.screen = ScreenMigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID(wantID), Name: "my-migration"},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	if rm.selectedMigID.String() != wantID {
		t.Errorf("selectedMigID: got %q, want %q", rm.selectedMigID.String(), wantID)
	}
}

func TestS2EnterDefinesSelectedMigrationInPloy(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenMigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-xyz"), Name: "my-migration"},
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

	if item0.title != "my-migration" {
		t.Errorf("item 0 title: got %q, want %q", item0.title, "my-migration")
	}
	if item0.description != "mig-xyz" {
		t.Errorf("item 0 description: got %q, want %q", item0.description, "mig-xyz")
	}
	if item1.title != "Runs" {
		t.Errorf("item 1 title: got %q, want %q", item1.title, "Runs")
	}
	if item1.description != "total: —" {
		t.Errorf("item 1 description: got %q, want %q", item1.description, "total: —")
	}
	if item2.title != "Jobs" {
		t.Errorf("item 2 title: got %q, want %q", item2.title, "Jobs")
	}
	if item2.description != "select job" {
		t.Errorf("item 2 description: got %q, want %q", item2.description, "select job")
	}
}

// TestS2EscTransitionsToS1 verifies Esc from S2 returns to S1.
func TestS2EscTransitionsToS1(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = ScreenMigrationsList
	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != ScreenRoot {
		t.Errorf("Esc(S2): got screen %v, want ScreenRoot", nm.screen)
	}
}
