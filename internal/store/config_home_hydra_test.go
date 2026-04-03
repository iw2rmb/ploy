package store

import (
	"context"
	"os"
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
