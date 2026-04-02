package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestConfigEnv_CRUD verifies the CRUD operations for the config_env table
// using key+target composite primary key semantics.
// See docs/envs/README.md#Global Env Configuration for user-facing semantics.
//
// This test is skipped if PLOY_TEST_DB_DSN is not set.
func TestConfigEnv_CRUD(t *testing.T) {
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

	// Clean up any existing test keys to ensure isolation.
	testPairs := []DeleteGlobalEnvParams{
		{Key: "TEST_CA_CERTS", Target: "gates"},
		{Key: "TEST_CA_CERTS", Target: "steps"},
		{Key: "TEST_CODEX_AUTH_JSON", Target: "steps"},
		{Key: "TEST_OPENAI_API_KEY", Target: "server"},
		{Key: "TEST_PK_KEY", Target: "gates"},
		{Key: "TEST_PK_KEY", Target: "steps"},
		{Key: "TEST_MULTI_TARGET", Target: "gates"},
		{Key: "TEST_MULTI_TARGET", Target: "steps"},
		{Key: "TEST_MULTI_TARGET", Target: "server"},
	}
	for _, p := range testPairs {
		_ = db.DeleteGlobalEnv(ctx, p)
	}

	// Subtest: UpsertGlobalEnv creates a new entry (insert path).
	t.Run("upsert_insert", func(t *testing.T) {
		err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key:    "TEST_CA_CERTS",
			Target: "gates",
			Value:  "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----",
			Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv() insert failed: %v", err)
		}

		// Verify via GetGlobalEnv.
		env, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_CA_CERTS", Target: "gates"})
		if err != nil {
			t.Fatalf("GetGlobalEnv() failed: %v", err)
		}
		if env.Key != "TEST_CA_CERTS" {
			t.Errorf("expected key=%q, got %q", "TEST_CA_CERTS", env.Key)
		}
		if env.Target != "gates" {
			t.Errorf("expected target=%q, got %q", "gates", env.Target)
		}
		if !env.Secret {
			t.Error("expected secret=true, got false")
		}
		if !env.UpdatedAt.Valid {
			t.Error("expected updated_at to be set")
		}
	})

	// Subtest: UpsertGlobalEnv updates an existing entry (update path on same key+target).
	t.Run("upsert_update", func(t *testing.T) {
		before, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_CA_CERTS", Target: "gates"})
		if err != nil {
			t.Fatalf("GetGlobalEnv() before update failed: %v", err)
		}
		beforeTime := before.UpdatedAt.Time

		time.Sleep(10 * time.Millisecond)

		// Upsert same key+target with new value and secret flag.
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key:    "TEST_CA_CERTS",
			Target: "gates",
			Value:  "-----BEGIN CERTIFICATE-----\nUPDATED...\n-----END CERTIFICATE-----",
			Secret: false,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv() update failed: %v", err)
		}

		after, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_CA_CERTS", Target: "gates"})
		if err != nil {
			t.Fatalf("GetGlobalEnv() after update failed: %v", err)
		}
		if after.Value != "-----BEGIN CERTIFICATE-----\nUPDATED...\n-----END CERTIFICATE-----" {
			t.Error("value was not updated")
		}
		if after.Target != "gates" {
			t.Errorf("expected target=%q after update, got %q", "gates", after.Target)
		}
		if after.Secret {
			t.Error("expected secret=false after update, got true")
		}
		if !after.UpdatedAt.Time.After(beforeTime) {
			t.Error("updated_at was not refreshed on update")
		}
	})

	// Subtest: Multi-target same-key creates separate rows (not overwrite).
	t.Run("multi_target_same_key", func(t *testing.T) {
		// Insert same key with two different targets.
		err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_MULTI_TARGET", Target: "gates",
			Value: "gates-value", Secret: false,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv(gates) failed: %v", err)
		}
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_MULTI_TARGET", Target: "steps",
			Value: "steps-value", Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv(steps) failed: %v", err)
		}

		// Both rows must exist with distinct values.
		g, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_MULTI_TARGET", Target: "gates"})
		if err != nil {
			t.Fatalf("GetGlobalEnv(gates) failed: %v", err)
		}
		if g.Value != "gates-value" {
			t.Errorf("gates value = %q, want %q", g.Value, "gates-value")
		}
		if g.Secret {
			t.Error("gates secret = true, want false")
		}

		s, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_MULTI_TARGET", Target: "steps"})
		if err != nil {
			t.Fatalf("GetGlobalEnv(steps) failed: %v", err)
		}
		if s.Value != "steps-value" {
			t.Errorf("steps value = %q, want %q", s.Value, "steps-value")
		}
		if !s.Secret {
			t.Error("steps secret = false, want true")
		}

		// Upsert one target does not affect the other.
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_MULTI_TARGET", Target: "gates",
			Value: "gates-updated", Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv(gates update) failed: %v", err)
		}
		g2, _ := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_MULTI_TARGET", Target: "gates"})
		if g2.Value != "gates-updated" {
			t.Errorf("gates value after update = %q, want %q", g2.Value, "gates-updated")
		}
		s2, _ := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_MULTI_TARGET", Target: "steps"})
		if s2.Value != "steps-value" {
			t.Errorf("steps value should be unchanged, got %q", s2.Value)
		}

		// Delete one target leaves the other intact.
		_ = db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: "TEST_MULTI_TARGET", Target: "gates"})
		_, err = db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_MULTI_TARGET", Target: "gates"})
		if err != pgx.ErrNoRows {
			t.Errorf("expected pgx.ErrNoRows after deleting gates target, got %v", err)
		}
		s3, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_MULTI_TARGET", Target: "steps"})
		if err != nil {
			t.Fatalf("steps target should still exist: %v", err)
		}
		if s3.Value != "steps-value" {
			t.Errorf("steps value = %q, want %q", s3.Value, "steps-value")
		}

		// Clean up remaining.
		_ = db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: "TEST_MULTI_TARGET", Target: "steps"})
	})

	// Subtest: ListGlobalEnv returns all entries ordered by key then target.
	t.Run("list_global_env", func(t *testing.T) {
		// Insert additional entries with distinct keys.
		err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_CODEX_AUTH_JSON", Target: "steps",
			Value: `{"token": "secret-token"}`, Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv() for CODEX failed: %v", err)
		}

		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_OPENAI_API_KEY", Target: "server",
			Value: "sk-test-key-12345", Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv() for OPENAI failed: %v", err)
		}

		// Insert same key with multiple targets to verify list returns
		// both rows without collapse.
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_MULTI_TARGET", Target: "gates",
			Value: "gates-list-value", Secret: false,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv(TEST_MULTI_TARGET/gates) failed: %v", err)
		}
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_MULTI_TARGET", Target: "server",
			Value: "server-list-value", Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv(TEST_MULTI_TARGET/server) failed: %v", err)
		}
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_MULTI_TARGET", Target: "steps",
			Value: "steps-list-value", Secret: false,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv(TEST_MULTI_TARGET/steps) failed: %v", err)
		}

		envs, err := db.ListGlobalEnv(ctx)
		if err != nil {
			t.Fatalf("ListGlobalEnv() failed: %v", err)
		}

		// Find our test entries (distinct keys).
		testKeys := []string{"TEST_CA_CERTS", "TEST_CODEX_AUTH_JSON", "TEST_OPENAI_API_KEY"}
		foundKeys := make(map[string]bool)
		for _, e := range envs {
			foundKeys[e.Key] = true
		}
		for _, key := range testKeys {
			if !foundKeys[key] {
				t.Errorf("expected to find key %q in ListGlobalEnv output", key)
			}
		}

		// Assert same-key multi-target rows are returned without collapse.
		type keyTarget struct{ key, target string }
		wantPairs := []keyTarget{
			{"TEST_MULTI_TARGET", "gates"},
			{"TEST_MULTI_TARGET", "server"},
			{"TEST_MULTI_TARGET", "steps"},
		}
		foundPairs := make(map[keyTarget]bool)
		for _, e := range envs {
			foundPairs[keyTarget{e.Key, e.Target}] = true
		}
		for _, p := range wantPairs {
			if !foundPairs[p] {
				t.Errorf("ListGlobalEnv missing row for key=%q target=%q; same-key rows may have been collapsed", p.key, p.target)
			}
		}

		// Verify ordering: entries must be ordered by key ASC, target ASC.
		for i := 1; i < len(envs); i++ {
			prev := envs[i-1].Key + ":" + envs[i-1].Target
			curr := envs[i].Key + ":" + envs[i].Target
			if prev > curr {
				t.Errorf("ListGlobalEnv not ordered: %q > %q", prev, curr)
				break
			}
		}
	})

	// Subtest: GetGlobalEnv returns error for non-existent key+target.
	t.Run("get_nonexistent", func(t *testing.T) {
		_, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "NONEXISTENT_KEY", Target: "server"})
		if err == nil {
			t.Error("expected GetGlobalEnv() to return error for non-existent key+target")
		}
		if err != pgx.ErrNoRows {
			t.Errorf("expected pgx.ErrNoRows, got %v", err)
		}
	})

	// Subtest: DeleteGlobalEnv removes an entry by key+target.
	t.Run("delete_global_env", func(t *testing.T) {
		_, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_OPENAI_API_KEY", Target: "server"})
		if err != nil {
			t.Fatalf("GetGlobalEnv() before delete failed: %v", err)
		}

		err = db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: "TEST_OPENAI_API_KEY", Target: "server"})
		if err != nil {
			t.Fatalf("DeleteGlobalEnv() failed: %v", err)
		}

		_, err = db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_OPENAI_API_KEY", Target: "server"})
		if err == nil {
			t.Error("expected GetGlobalEnv() to return error after delete")
		}
		if err != pgx.ErrNoRows {
			t.Errorf("expected pgx.ErrNoRows after delete, got %v", err)
		}
	})

	// Subtest: DeleteGlobalEnv is a no-op for non-existent key+target (no error).
	t.Run("delete_nonexistent", func(t *testing.T) {
		err := db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: "NONEXISTENT_KEY", Target: "server"})
		if err != nil {
			t.Errorf("DeleteGlobalEnv() on non-existent key+target should not error, got %v", err)
		}
	})

	// Subtest: Composite primary key enforcement — same key+target upserts, not duplicates.
	t.Run("composite_pk_enforcement", func(t *testing.T) {
		err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_PK_KEY", Target: "gates",
			Value: "first-value", Secret: true,
		})
		if err != nil {
			t.Fatalf("first UpsertGlobalEnv() failed: %v", err)
		}

		// Same key+target: should update, not duplicate.
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_PK_KEY", Target: "gates",
			Value: "second-value", Secret: false,
		})
		if err != nil {
			t.Fatalf("second UpsertGlobalEnv() failed: %v", err)
		}

		env, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_PK_KEY", Target: "gates"})
		if err != nil {
			t.Fatalf("GetGlobalEnv() failed: %v", err)
		}
		if env.Value != "second-value" {
			t.Errorf("expected value=%q, got %q", "second-value", env.Value)
		}

		// Same key, different target: should create a new row.
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key: "TEST_PK_KEY", Target: "steps",
			Value: "steps-value", Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv(steps) failed: %v", err)
		}

		// Both (key, gates) and (key, steps) must exist.
		g, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_PK_KEY", Target: "gates"})
		if err != nil {
			t.Fatalf("GetGlobalEnv(gates) failed: %v", err)
		}
		if g.Value != "second-value" {
			t.Errorf("gates value = %q, want %q", g.Value, "second-value")
		}
		s, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: "TEST_PK_KEY", Target: "steps"})
		if err != nil {
			t.Fatalf("GetGlobalEnv(steps) failed: %v", err)
		}
		if s.Value != "steps-value" {
			t.Errorf("steps value = %q, want %q", s.Value, "steps-value")
		}

		_ = db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: "TEST_PK_KEY", Target: "gates"})
		_ = db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: "TEST_PK_KEY", Target: "steps"})
	})

	// Subtest: Target values are stored correctly.
	t.Run("target_values", func(t *testing.T) {
		targets := []string{"server", "nodes", "gates", "steps"}
		for _, target := range targets {
			key := "TEST_TARGET_" + target
			err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
				Key: key, Target: target,
				Value: "test-value", Secret: false,
			})
			if err != nil {
				t.Fatalf("UpsertGlobalEnv() with target=%q failed: %v", target, err)
			}

			env, err := db.GetGlobalEnv(ctx, GetGlobalEnvParams{Key: key, Target: target})
			if err != nil {
				t.Fatalf("GetGlobalEnv() with target=%q failed: %v", target, err)
			}
			if env.Target != target {
				t.Errorf("expected target=%q, got %q", target, env.Target)
			}

			_ = db.DeleteGlobalEnv(ctx, DeleteGlobalEnvParams{Key: key, Target: target})
		}
	})

	// Clean up remaining test keys.
	for _, p := range testPairs {
		_ = db.DeleteGlobalEnv(ctx, p)
	}
}
