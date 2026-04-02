package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
)

// TestV1SQLCQueries_Mods verifies that the v1 migs queries are wired and match
// the expected filter semantics.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestV1SQLCQueries_Mods(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"

	modID1 := types.NewMigID()
	modName1 := "v1-sqlc-mig-alpha-" + modID1.String()
	mod1, err := db.CreateMig(ctx, CreateMigParams{
		ID:        modID1,
		Name:      modName1,
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig(mod1) failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mod1.ID) }()

	modID2 := types.NewMigID()
	modName2 := "v1-sqlc-mig-beta-" + modID2.String()
	mod2, err := db.CreateMig(ctx, CreateMigParams{
		ID:        modID2,
		Name:      modName2,
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig(mod2) failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mod2.ID) }()

	gotByName, err := db.GetMigByName(ctx, modName1)
	if err != nil {
		t.Fatalf("GetMigByName() failed: %v", err)
	}
	if gotByName.ID != mod1.ID {
		t.Fatalf("GetMigByName() returned wrong mig id: got=%q want=%q", gotByName.ID, mod1.ID)
	}

	if err := db.ArchiveMig(ctx, mod2.ID); err != nil {
		t.Fatalf("ArchiveMig() failed: %v", err)
	}

	// List all: archived_only=nil.
	all, err := db.ListMigs(ctx, ListMigsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: nil,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMigs(all) failed: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("ListMigs(all) returned too few rows: got=%d want>=2", len(all))
	}

	// Archived only.
	archivedOnly := true
	archived, err := db.ListMigs(ctx, ListMigsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: &archivedOnly,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMigs(archived) failed: %v", err)
	}
	foundArchived := false
	for _, m := range archived {
		if m.ID == mod2.ID {
			foundArchived = true
		}
		if !m.ArchivedAt.Valid {
			t.Fatalf("ListMigs(archived) returned unarchived row: id=%q name=%q", m.ID, m.Name)
		}
	}
	if !foundArchived {
		t.Fatalf("ListMigs(archived) did not return expected archived mig: id=%q", mod2.ID)
	}

	// Active only.
	activeOnly := false
	active, err := db.ListMigs(ctx, ListMigsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: &activeOnly,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMigs(active) failed: %v", err)
	}
	for _, m := range active {
		if m.ArchivedAt.Valid {
			t.Fatalf("ListMigs(active) returned archived row: id=%q name=%q", m.ID, m.Name)
		}
	}

	// Name filtering.
	nameFilter := "alpha"
	filtered, err := db.ListMigs(ctx, ListMigsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: nil,
		NameFilter:   &nameFilter,
	})
	if err != nil {
		t.Fatalf("ListMigs(name_filter) failed: %v", err)
	}
	foundAlpha := false
	for _, m := range filtered {
		if m.ID == mod1.ID {
			foundAlpha = true
		}
		if m.ID == mod2.ID {
			t.Fatalf("ListMigs(name_filter) unexpectedly included mod2: id=%q name=%q", m.ID, m.Name)
		}
	}
	if !foundAlpha {
		t.Fatalf("ListMigs(name_filter) did not return expected mod1: id=%q", mod1.ID)
	}

	if err := db.UnarchiveMig(ctx, mod2.ID); err != nil {
		t.Fatalf("UnarchiveMig() failed: %v", err)
	}
}

// TestV1SQLCQueries_ModRepos verifies v1 mig_repos lookup/upsert/delete behavior.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestV1SQLCQueries_ModRepos(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"
	modID := types.NewMigID()
	mig, err := db.CreateMig(ctx, CreateMigParams{
		ID:        modID,
		Name:      "v1-sqlc-mig-repos-" + modID.String(),
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mig.ID) }()

	repoURL := "https://github.com/iw2rmb/ploy-test-repo.git"

	repoID1 := types.NewMigRepoID()
	inserted, err := db.UpsertMigRepo(ctx, UpsertMigRepoParams{
		ID:        repoID1,
		MigID:     mig.ID,
		Url:       repoURL,
		BaseRef:   "main",
		TargetRef: "feature",
	})
	if err != nil {
		t.Fatalf("UpsertMigRepo(insert) failed: %v", err)
	}
	_, err = db.UpsertMigRepo(ctx, UpsertMigRepoParams{
		ID:        types.NewMigRepoID(),
		MigID:     mig.ID,
		Url:       repoURL,
		BaseRef:   "main",
		TargetRef: "feature",
	})
	if err != nil {
		t.Fatalf("UpsertMigRepo(unchanged) failed: %v", err)
	}

	// Conflict path: provide a different id but same (mig_id, repo_url). ID must remain stable.
	repoID2 := types.NewMigRepoID()
	updated, err := db.UpsertMigRepo(ctx, UpsertMigRepoParams{
		ID:        repoID2,
		MigID:     mig.ID,
		Url:       repoURL,
		BaseRef:   "trunk",
		TargetRef: "feature-2",
	})
	if err != nil {
		t.Fatalf("UpsertMigRepo(update) failed: %v", err)
	}
	if updated.ID != inserted.ID {
		t.Fatalf("UpsertMigRepo(update) changed id: got=%q want=%q", updated.ID, inserted.ID)
	}
	if updated.BaseRef != "trunk" || updated.TargetRef != "feature-2" {
		t.Fatalf("UpsertMigRepo(update) did not update refs: base=%q target=%q", updated.BaseRef, updated.TargetRef)
	}

	got, err := db.GetMigRepoByURL(ctx, GetMigRepoByURLParams{
		MigID: mig.ID,
		Url:   repoURL,
	})
	if err != nil {
		t.Fatalf("GetMigRepoByURL() failed: %v", err)
	}
	if got.ID != inserted.ID {
		t.Fatalf("GetMigRepoByURL() returned wrong id: got=%q want=%q", got.ID, inserted.ID)
	}

	if err := db.DeleteMigRepo(ctx, inserted.ID); err != nil {
		t.Fatalf("DeleteMigRepo() failed: %v", err)
	}

	_, err = db.GetMigRepoByURL(ctx, GetMigRepoByURLParams{
		MigID: mig.ID,
		Url:   repoURL,
	})
	if err == nil {
		t.Fatal("expected GetMigRepoByURL() after DeleteMigRepo to fail, but it succeeded")
	}
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows after DeleteMigRepo, got %T: %v", err, err)
	}
}

// TestV1SQLCQueries_Specs verifies ListSpecs ordering by created_at DESC.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestV1SQLCQueries_Specs(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"

	specID1 := types.NewSpecID()
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

	specID2 := types.NewSpecID()
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

	// Use recent timestamps (relative to now) so both specs appear in the top of DESC ordering.
	now := time.Now().UTC()
	older := now.Add(-2 * time.Second)
	newer := now.Add(-1 * time.Second)
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
