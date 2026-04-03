package store

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestConfigHome_HydraCRUD verifies CRUD operations for the config_home table
// using Hydra canonical entry semantics: dedup by (dst, section) composite key,
// upsert updates entry on conflict, and delete by normalized dst removes the row.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestConfigHome_HydraCRUD(t *testing.T) {
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

	// Clean up test entries to ensure isolation.
	cleanup := func() {
		_ = db.DeleteConfigHome(ctx, DeleteConfigHomeParams{Dst: ".config/app", Section: "mig"})
		_ = db.DeleteConfigHome(ctx, DeleteConfigHomeParams{Dst: ".local/bin", Section: "mig"})
		_ = db.DeleteConfigHome(ctx, DeleteConfigHomeParams{Dst: ".ssh", Section: "pre_gate"})
	}
	cleanup()
	t.Cleanup(cleanup)

	t.Run("upsert_insert", func(t *testing.T) {
		err := db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
			Entry:   "abcdef1:.config/app",
			Dst:     ".config/app",
			Section: "mig",
		})
		if err != nil {
			t.Fatalf("UpsertConfigHome() insert failed: %v", err)
		}

		rows, err := db.ListConfigHomeBySection(ctx, "mig")
		if err != nil {
			t.Fatalf("ListConfigHomeBySection() failed: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows, want 1", len(rows))
		}
		if rows[0].Entry != "abcdef1:.config/app" {
			t.Errorf("entry = %q, want abcdef1:.config/app", rows[0].Entry)
		}
		if rows[0].Dst != ".config/app" {
			t.Errorf("dst = %q, want .config/app", rows[0].Dst)
		}
	})

	t.Run("upsert_update_same_dst", func(t *testing.T) {
		// Upsert same dst with different hash — should update, not duplicate.
		err := db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
			Entry:   "bbbbbbb:.config/app",
			Dst:     ".config/app",
			Section: "mig",
		})
		if err != nil {
			t.Fatalf("UpsertConfigHome() update failed: %v", err)
		}

		rows, err := db.ListConfigHomeBySection(ctx, "mig")
		if err != nil {
			t.Fatalf("ListConfigHomeBySection() failed: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("got %d rows after upsert, want 1", len(rows))
		}
		if rows[0].Entry != "bbbbbbb:.config/app" {
			t.Errorf("entry = %q, want bbbbbbb:.config/app (updated)", rows[0].Entry)
		}
	})

	t.Run("different_section_separate_rows", func(t *testing.T) {
		err := db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
			Entry:   "1234567:.ssh",
			Dst:     ".ssh",
			Section: "pre_gate",
		})
		if err != nil {
			t.Fatalf("UpsertConfigHome() failed: %v", err)
		}

		all, err := db.ListConfigHome(ctx)
		if err != nil {
			t.Fatalf("ListConfigHome() failed: %v", err)
		}
		// Should have mig/.config/app and pre_gate/.ssh
		found := 0
		for _, row := range all {
			if (row.Dst == ".config/app" && row.Section == "mig") ||
				(row.Dst == ".ssh" && row.Section == "pre_gate") {
				found++
			}
		}
		if found != 2 {
			t.Errorf("expected 2 test entries, found %d", found)
		}
	})

	t.Run("delete_by_normalized_dst", func(t *testing.T) {
		err := db.DeleteConfigHome(ctx, DeleteConfigHomeParams{
			Dst:     ".config/app",
			Section: "mig",
		})
		if err != nil {
			t.Fatalf("DeleteConfigHome() failed: %v", err)
		}

		rows, err := db.ListConfigHomeBySection(ctx, "mig")
		if err != nil {
			t.Fatalf("ListConfigHomeBySection() failed: %v", err)
		}
		if len(rows) != 0 {
			t.Errorf("got %d rows after delete, want 0", len(rows))
		}
	})

	t.Run("list_ordering", func(t *testing.T) {
		_ = db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
			Entry: "abcdef1:.local/bin", Dst: ".local/bin", Section: "mig",
		})
		_ = db.UpsertConfigHome(ctx, UpsertConfigHomeParams{
			Entry: "abcdef1:.config/app", Dst: ".config/app", Section: "mig",
		})

		rows, err := db.ListConfigHomeBySection(ctx, "mig")
		if err != nil {
			t.Fatalf("ListConfigHomeBySection() failed: %v", err)
		}
		if len(rows) < 2 {
			t.Fatalf("got %d rows, want >= 2", len(rows))
		}
		// Should be ordered by dst ASC.
		if rows[0].Dst > rows[1].Dst {
			t.Errorf("ordering: %q should come before %q", rows[0].Dst, rows[1].Dst)
		}
	})
}

// TestConfigHydra_QueryContractSemantics verifies the Hydra config CRUD contract
// at the query and interface level without requiring a database connection.
// Validates:
//   - Querier interface carries typed CA and Home CRUD methods with correct signatures
//   - SQL queries use composite-key conflict handling for upsert dedup
//   - SQL queries enforce deterministic ordering for list operations
//   - Param and model structs carry expected fields for Hydra contract semantics
func TestConfigHydra_QueryContractSemantics(t *testing.T) {
	// Verify Querier interface exposes typed Hydra config CRUD with correct signatures.
	type hydraConfigQuerier interface {
		ListConfigCA(ctx context.Context) ([]ConfigCa, error)
		ListConfigCABySection(ctx context.Context, section string) ([]ConfigCa, error)
		UpsertConfigCA(ctx context.Context, arg UpsertConfigCAParams) error
		DeleteConfigCA(ctx context.Context, arg DeleteConfigCAParams) error
		DeleteConfigCABySection(ctx context.Context, section string) error
		ListConfigHome(ctx context.Context) ([]ConfigHome, error)
		ListConfigHomeBySection(ctx context.Context, section string) ([]ConfigHome, error)
		UpsertConfigHome(ctx context.Context, arg UpsertConfigHomeParams) error
		DeleteConfigHome(ctx context.Context, arg DeleteConfigHomeParams) error
		DeleteConfigHomeBySection(ctx context.Context, section string) error
	}
	var _ hydraConfigQuerier = (Querier)(nil)

	// Verify ConfigCa model fields.
	var ca ConfigCa
	assertType[string](ca.Hash)
	assertType[string](ca.Section)

	// Verify ConfigHome model fields.
	var home ConfigHome
	assertType[string](home.Entry)
	assertType[string](home.Dst)
	assertType[string](home.Section)

	// Verify CA upsert uses composite-key (hash, section) conflict handling.
	t.Run("ca_upsert_composite_key_conflict", func(t *testing.T) {
		if !strings.Contains(upsertConfigCA, "ON CONFLICT (hash, section)") {
			t.Errorf("upsertConfigCA missing composite-key conflict clause:\n%s", upsertConfigCA)
		}
	})

	// Verify Home upsert uses composite-key (dst, section) conflict handling
	// and updates entry on conflict (entry changes when hash or ro flag changes).
	t.Run("home_upsert_composite_key_conflict", func(t *testing.T) {
		if !strings.Contains(upsertConfigHome, "ON CONFLICT (dst, section)") {
			t.Errorf("upsertConfigHome missing composite-key conflict clause:\n%s", upsertConfigHome)
		}
		if !strings.Contains(upsertConfigHome, "entry") || !strings.Contains(upsertConfigHome, "EXCLUDED.entry") {
			t.Errorf("upsertConfigHome must update entry on conflict:\n%s", upsertConfigHome)
		}
	})

	// Verify list queries enforce deterministic ordering for overlay iteration.
	t.Run("ca_list_deterministic_ordering", func(t *testing.T) {
		if !strings.Contains(listConfigCA, "ORDER BY section ASC, hash ASC") {
			t.Errorf("listConfigCA missing deterministic ordering:\n%s", listConfigCA)
		}
		if !strings.Contains(listConfigCABySection, "ORDER BY hash ASC") {
			t.Errorf("listConfigCABySection missing hash ordering:\n%s", listConfigCABySection)
		}
	})

	t.Run("home_list_deterministic_ordering", func(t *testing.T) {
		if !strings.Contains(listConfigHome, "ORDER BY section ASC, dst ASC") {
			t.Errorf("listConfigHome missing deterministic ordering:\n%s", listConfigHome)
		}
		if !strings.Contains(listConfigHomeBySection, "ORDER BY dst ASC") {
			t.Errorf("listConfigHomeBySection missing dst ordering:\n%s", listConfigHomeBySection)
		}
	})

	// Verify delete queries use correct composite-key matching.
	t.Run("ca_delete_by_composite_key", func(t *testing.T) {
		var p DeleteConfigCAParams
		assertType[string](p.Hash)
		assertType[string](p.Section)
		if !strings.Contains(deleteConfigCA, "hash = $1 AND section = $2") {
			t.Errorf("deleteConfigCA must delete by composite key (hash, section):\n%s", deleteConfigCA)
		}
	})

	t.Run("home_delete_by_composite_key", func(t *testing.T) {
		var p DeleteConfigHomeParams
		assertType[string](p.Dst)
		assertType[string](p.Section)
		if !strings.Contains(deleteConfigHome, "dst = $1 AND section = $2") {
			t.Errorf("deleteConfigHome must delete by composite key (dst, section):\n%s", deleteConfigHome)
		}
	})
}
