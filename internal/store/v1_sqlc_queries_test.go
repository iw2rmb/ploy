package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
)

// TestV1SQLCQueries_Mods verifies that the v1 mods queries are wired and match
// the expected filter semantics.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_Mods(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"

	modID1 := types.NewModID().String()
	modName1 := "v1-sqlc-mod-alpha-" + modID1
	mod1, err := db.CreateMod(ctx, CreateModParams{
		ID:        modID1,
		Name:      modName1,
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMod(mod1) failed: %v", err)
	}
	defer func() { _ = db.DeleteMod(ctx, mod1.ID) }()

	modID2 := types.NewModID().String()
	modName2 := "v1-sqlc-mod-beta-" + modID2
	mod2, err := db.CreateMod(ctx, CreateModParams{
		ID:        modID2,
		Name:      modName2,
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMod(mod2) failed: %v", err)
	}
	defer func() { _ = db.DeleteMod(ctx, mod2.ID) }()

	gotByName, err := db.GetModByName(ctx, modName1)
	if err != nil {
		t.Fatalf("GetModByName() failed: %v", err)
	}
	if gotByName.ID != mod1.ID {
		t.Fatalf("GetModByName() returned wrong mod id: got=%q want=%q", gotByName.ID, mod1.ID)
	}

	if err := db.ArchiveMod(ctx, mod2.ID); err != nil {
		t.Fatalf("ArchiveMod() failed: %v", err)
	}

	// List all: archived_only=nil.
	all, err := db.ListMods(ctx, ListModsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: nil,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMods(all) failed: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("ListMods(all) returned too few rows: got=%d want>=2", len(all))
	}

	// Archived only.
	archivedOnly := true
	archived, err := db.ListMods(ctx, ListModsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: &archivedOnly,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMods(archived) failed: %v", err)
	}
	foundArchived := false
	for _, m := range archived {
		if m.ID == mod2.ID {
			foundArchived = true
		}
		if !m.ArchivedAt.Valid {
			t.Fatalf("ListMods(archived) returned unarchived row: id=%q name=%q", m.ID, m.Name)
		}
	}
	if !foundArchived {
		t.Fatalf("ListMods(archived) did not return expected archived mod: id=%q", mod2.ID)
	}

	// Active only.
	activeOnly := false
	active, err := db.ListMods(ctx, ListModsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: &activeOnly,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMods(active) failed: %v", err)
	}
	for _, m := range active {
		if m.ArchivedAt.Valid {
			t.Fatalf("ListMods(active) returned archived row: id=%q name=%q", m.ID, m.Name)
		}
	}

	// Name filtering.
	nameFilter := "alpha"
	filtered, err := db.ListMods(ctx, ListModsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: nil,
		NameFilter:   &nameFilter,
	})
	if err != nil {
		t.Fatalf("ListMods(name_filter) failed: %v", err)
	}
	foundAlpha := false
	for _, m := range filtered {
		if m.ID == mod1.ID {
			foundAlpha = true
		}
		if m.ID == mod2.ID {
			t.Fatalf("ListMods(name_filter) unexpectedly included mod2: id=%q name=%q", m.ID, m.Name)
		}
	}
	if !foundAlpha {
		t.Fatalf("ListMods(name_filter) did not return expected mod1: id=%q", mod1.ID)
	}

	if err := db.UnarchiveMod(ctx, mod2.ID); err != nil {
		t.Fatalf("UnarchiveMod() failed: %v", err)
	}
}

// TestV1SQLCQueries_ModRepos verifies v1 mod_repos lookup/upsert/delete behavior.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_ModRepos(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"
	modID := types.NewModID().String()
	mod, err := db.CreateMod(ctx, CreateModParams{
		ID:        modID,
		Name:      "v1-sqlc-mod-repos-" + modID,
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMod() failed: %v", err)
	}
	defer func() { _ = db.DeleteMod(ctx, mod.ID) }()

	repoURL := "https://github.com/iw2rmb/ploy-test-repo.git"

	repoID1 := types.NewModRepoID().String()
	inserted, err := db.UpsertModRepo(ctx, UpsertModRepoParams{
		ID:        repoID1,
		ModID:     mod.ID,
		RepoUrl:   repoURL,
		BaseRef:   "main",
		TargetRef: "feature",
	})
	if err != nil {
		t.Fatalf("UpsertModRepo(insert) failed: %v", err)
	}

	// Conflict path: provide a different id but same (mod_id, repo_url). ID must remain stable.
	repoID2 := types.NewModRepoID().String()
	updated, err := db.UpsertModRepo(ctx, UpsertModRepoParams{
		ID:        repoID2,
		ModID:     mod.ID,
		RepoUrl:   repoURL,
		BaseRef:   "trunk",
		TargetRef: "feature-2",
	})
	if err != nil {
		t.Fatalf("UpsertModRepo(update) failed: %v", err)
	}
	if updated.ID != inserted.ID {
		t.Fatalf("UpsertModRepo(update) changed id: got=%q want=%q", updated.ID, inserted.ID)
	}
	if updated.BaseRef != "trunk" || updated.TargetRef != "feature-2" {
		t.Fatalf("UpsertModRepo(update) did not update refs: base=%q target=%q", updated.BaseRef, updated.TargetRef)
	}

	got, err := db.GetModRepoByURL(ctx, GetModRepoByURLParams{
		ModID:   mod.ID,
		RepoUrl: repoURL,
	})
	if err != nil {
		t.Fatalf("GetModRepoByURL() failed: %v", err)
	}
	if got.ID != inserted.ID {
		t.Fatalf("GetModRepoByURL() returned wrong id: got=%q want=%q", got.ID, inserted.ID)
	}

	if err := db.DeleteModRepo(ctx, inserted.ID); err != nil {
		t.Fatalf("DeleteModRepo() failed: %v", err)
	}

	_, err = db.GetModRepoByURL(ctx, GetModRepoByURLParams{
		ModID:   mod.ID,
		RepoUrl: repoURL,
	})
	if err == nil {
		t.Fatal("expected GetModRepoByURL() after DeleteModRepo to fail, but it succeeded")
	}
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows after DeleteModRepo, got %T: %v", err, err)
	}
}

// TestV1SQLCQueries_Specs verifies ListSpecs ordering by created_at DESC.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_Specs(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"

	specID1 := types.NewSpecID().String()
	spec1, err := db.CreateSpec(ctx, CreateSpecParams{
		ID:        specID1,
		Name:      "v1-sqlc-spec-1",
		Spec:      []byte(`{"steps":[]}`),
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec(spec1) failed: %v", err)
	}
	defer func() { _, _ = db.Pool().Exec(ctx, "DELETE FROM specs WHERE id=$1", spec1.ID) }()

	specID2 := types.NewSpecID().String()
	spec2, err := db.CreateSpec(ctx, CreateSpecParams{
		ID:        specID2,
		Name:      "v1-sqlc-spec-2",
		Spec:      []byte(`{"steps":[]}`),
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec(spec2) failed: %v", err)
	}
	defer func() { _, _ = db.Pool().Exec(ctx, "DELETE FROM specs WHERE id=$1", spec2.ID) }()

	older := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.Pool().Exec(ctx, "UPDATE specs SET created_at=$2 WHERE id=$1", spec1.ID, older); err != nil {
		t.Fatalf("set spec1 created_at failed: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE specs SET created_at=$2 WHERE id=$1", spec2.ID, newer); err != nil {
		t.Fatalf("set spec2 created_at failed: %v", err)
	}

	got, err := db.ListSpecs(ctx, ListSpecsParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("ListSpecs() failed: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("ListSpecs() returned too few rows: got=%d want>=2", len(got))
	}

	// Find our two specs and assert relative ordering.
	idx1, idx2 := -1, -1
	for i, s := range got {
		switch s.ID {
		case spec1.ID:
			idx1 = i
		case spec2.ID:
			idx2 = i
		}
	}
	if idx1 == -1 || idx2 == -1 {
		t.Fatalf("ListSpecs() did not include expected specs: idx1=%d idx2=%d", idx1, idx2)
	}
	if idx2 >= idx1 {
		t.Fatalf("expected spec2 (newer) to appear before spec1 (older): idx2=%d idx1=%d", idx2, idx1)
	}
}
