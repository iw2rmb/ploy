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
	migs := []clitui.MigItem{
		{ID: domaintypes.MigID("mig-aaa"), Name: "alpha"},
		{ID: domaintypes.MigID("mig-bbb"), Name: "beta"},
	}
	next, _ := m.Update(migsLoadedMsg{migs: migs})
	nm := next.(model)

	items := nm.secondary.Items()
	if len(items) != 2 {
		t.Fatalf("secondary items: got %d, want 2", len(items))
	}

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

// TestS2MigrationsOrderingPreserved verifies that items appear in the order received
// from the API (newest-to-oldest).
func TestS2MigrationsOrderingPreserved(t *testing.T) {
	m := InitialModel(nil, nil)
	migs := []clitui.MigItem{
		{ID: domaintypes.MigID("mig-newest"), Name: "newest"},
		{ID: domaintypes.MigID("mig-middle"), Name: "middle"},
		{ID: domaintypes.MigID("mig-oldest"), Name: "oldest"},
	}
	next, _ := m.Update(migsLoadedMsg{migs: migs})
	nm := next.(model)

	items := nm.secondary.Items()
	if len(items) != 3 {
		t.Fatalf("items count: got %d, want 3", len(items))
	}

	for i, want := range migs {
		item, ok := items[i].(listItem)
		if !ok {
			t.Fatalf("item %d: unexpected type %T", i, items[i])
		}
		if item.title != want.Name {
			t.Errorf("ordering: item %d title: got %q, want %q", i, item.title, want.Name)
		}
	}
}

// TestS2EnterTransitionsToS3 verifies Enter on a selected migration transitions to S3.
func TestS2EnterTransitionsToS3(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S2MigrationsList
	next, _ := m.Update(migsLoadedMsg{migs: []clitui.MigItem{
		{ID: domaintypes.MigID("mig-xyz"), Name: "my-migration"},
	}})
	nm := next.(model)
	nm.secondary.Select(0)

	result, _ := nm.handleEnter()
	rm := result.(model)
	if rm.screen != S3MigrationDetails {
		t.Errorf("Enter(S2): got screen %v, want S3MigrationDetails", rm.screen)
	}
}

// TestS2EnterSetsSelectedMigID verifies selectedMigID is set from the chosen migration's ID.
func TestS2EnterSetsSelectedMigID(t *testing.T) {
	const wantID = "mig-xyz"
	m := InitialModel(nil, nil)
	m.screen = S2MigrationsList
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

// TestS2EscTransitionsToS1 verifies Esc from S2 returns to S1.
func TestS2EscTransitionsToS1(t *testing.T) {
	m := InitialModel(nil, nil)
	m.screen = S2MigrationsList
	next, _ := m.handleEsc()
	nm := next.(model)
	if nm.screen != S1Root {
		t.Errorf("Esc(S2): got screen %v, want S1Root", nm.screen)
	}
}
