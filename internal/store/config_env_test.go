package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestConfigEnv_CRUD verifies the CRUD operations for the config_env table.
// See docs/envs/README.md#Global Env Configuration for user-facing semantics.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestConfigEnv_CRUD(t *testing.T) {
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

	// Clean up any existing test keys to ensure isolation.
	testKeys := []string{
		"TEST_CA_CERTS_PEM_BUNDLE",
		"TEST_CODEX_AUTH_JSON",
		"TEST_OPENAI_API_KEY",
	}
	for _, key := range testKeys {
		_ = db.DeleteGlobalEnv(ctx, key)
	}

	// Subtest: UpsertGlobalEnv creates a new entry (insert path).
	t.Run("upsert_insert", func(t *testing.T) {
		err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key:    "TEST_CA_CERTS_PEM_BUNDLE",
			Value:  "-----BEGIN CERTIFICATE-----\nMIIC...\n-----END CERTIFICATE-----",
			Scope:  "all",
			Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv() insert failed: %v", err)
		}

		// Verify via GetGlobalEnv.
		env, err := db.GetGlobalEnv(ctx, "TEST_CA_CERTS_PEM_BUNDLE")
		if err != nil {
			t.Fatalf("GetGlobalEnv() failed: %v", err)
		}
		if env.Key != "TEST_CA_CERTS_PEM_BUNDLE" {
			t.Errorf("expected key=%q, got %q", "TEST_CA_CERTS_PEM_BUNDLE", env.Key)
		}
		if env.Scope != "all" {
			t.Errorf("expected scope=%q, got %q", "all", env.Scope)
		}
		if !env.Secret {
			t.Error("expected secret=true, got false")
		}
		if !env.UpdatedAt.Valid {
			t.Error("expected updated_at to be set")
		}
	})

	// Subtest: UpsertGlobalEnv updates an existing entry (update path).
	t.Run("upsert_update", func(t *testing.T) {
		// First, get the current updated_at.
		before, err := db.GetGlobalEnv(ctx, "TEST_CA_CERTS_PEM_BUNDLE")
		if err != nil {
			t.Fatalf("GetGlobalEnv() before update failed: %v", err)
		}
		beforeTime := before.UpdatedAt.Time

		// Small delay to ensure updated_at changes.
		time.Sleep(10 * time.Millisecond)

		// Upsert with new value and scope.
		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key:    "TEST_CA_CERTS_PEM_BUNDLE",
			Value:  "-----BEGIN CERTIFICATE-----\nUPDATED...\n-----END CERTIFICATE-----",
			Scope:  "migs",
			Secret: false,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv() update failed: %v", err)
		}

		// Verify update.
		after, err := db.GetGlobalEnv(ctx, "TEST_CA_CERTS_PEM_BUNDLE")
		if err != nil {
			t.Fatalf("GetGlobalEnv() after update failed: %v", err)
		}
		if after.Value != "-----BEGIN CERTIFICATE-----\nUPDATED...\n-----END CERTIFICATE-----" {
			t.Error("value was not updated")
		}
		if after.Scope != "migs" {
			t.Errorf("expected scope=%q after update, got %q", "migs", after.Scope)
		}
		if after.Secret {
			t.Error("expected secret=false after update, got true")
		}
		// updated_at should be newer.
		if !after.UpdatedAt.Time.After(beforeTime) {
			t.Error("updated_at was not refreshed on update")
		}
	})

	// Subtest: ListGlobalEnv returns all entries ordered by key.
	t.Run("list_global_env", func(t *testing.T) {
		// Insert additional entries.
		err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key:    "TEST_CODEX_AUTH_JSON",
			Value:  `{"token": "secret-token"}`,
			Scope:  "migs",
			Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv() for CODEX failed: %v", err)
		}

		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key:    "TEST_OPENAI_API_KEY",
			Value:  "sk-test-key-12345",
			Scope:  "all",
			Secret: true,
		})
		if err != nil {
			t.Fatalf("UpsertGlobalEnv() for OPENAI failed: %v", err)
		}

		// List all entries.
		envs, err := db.ListGlobalEnv(ctx)
		if err != nil {
			t.Fatalf("ListGlobalEnv() failed: %v", err)
		}

		// Find our test entries (there may be others from previous runs).
		foundKeys := make(map[string]bool)
		for _, e := range envs {
			foundKeys[e.Key] = true
		}

		for _, key := range testKeys {
			if !foundKeys[key] {
				t.Errorf("expected to find key %q in ListGlobalEnv output", key)
			}
		}

		// Verify ordering: TEST_CA < TEST_CODEX < TEST_OPENAI (alphabetical).
		var indices []int
		for i, e := range envs {
			for j, key := range testKeys {
				if e.Key == key {
					indices = append(indices, j)
					_ = i // suppress unused warning
				}
			}
		}
		// indices should be in ascending order if sorted by key.
		for i := 1; i < len(indices); i++ {
			if indices[i] < indices[i-1] {
				t.Error("ListGlobalEnv results are not ordered by key")
				break
			}
		}
	})

	// Subtest: GetGlobalEnv returns error for non-existent key.
	t.Run("get_nonexistent", func(t *testing.T) {
		_, err := db.GetGlobalEnv(ctx, "NONEXISTENT_KEY_12345")
		if err == nil {
			t.Error("expected GetGlobalEnv() to return error for non-existent key")
		}
		if err != pgx.ErrNoRows {
			t.Errorf("expected pgx.ErrNoRows, got %v", err)
		}
	})

	// Subtest: DeleteGlobalEnv removes an entry.
	t.Run("delete_global_env", func(t *testing.T) {
		// Verify key exists before delete.
		_, err := db.GetGlobalEnv(ctx, "TEST_OPENAI_API_KEY")
		if err != nil {
			t.Fatalf("GetGlobalEnv() before delete failed: %v", err)
		}

		// Delete.
		err = db.DeleteGlobalEnv(ctx, "TEST_OPENAI_API_KEY")
		if err != nil {
			t.Fatalf("DeleteGlobalEnv() failed: %v", err)
		}

		// Verify key is gone.
		_, err = db.GetGlobalEnv(ctx, "TEST_OPENAI_API_KEY")
		if err == nil {
			t.Error("expected GetGlobalEnv() to return error after delete")
		}
		if err != pgx.ErrNoRows {
			t.Errorf("expected pgx.ErrNoRows after delete, got %v", err)
		}
	})

	// Subtest: DeleteGlobalEnv is a no-op for non-existent key (no error).
	t.Run("delete_nonexistent", func(t *testing.T) {
		err := db.DeleteGlobalEnv(ctx, "NONEXISTENT_KEY_TO_DELETE")
		if err != nil {
			t.Errorf("DeleteGlobalEnv() on non-existent key should not error, got %v", err)
		}
	})

	// Subtest: Primary key enforcement — upsert on same key updates, not duplicates.
	t.Run("primary_key_enforcement", func(t *testing.T) {
		// Upsert same key twice with different values.
		err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key:    "TEST_PK_KEY",
			Value:  "first-value",
			Scope:  "all",
			Secret: true,
		})
		if err != nil {
			t.Fatalf("first UpsertGlobalEnv() failed: %v", err)
		}

		err = db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
			Key:    "TEST_PK_KEY",
			Value:  "second-value",
			Scope:  "gate",
			Secret: false,
		})
		if err != nil {
			t.Fatalf("second UpsertGlobalEnv() failed: %v", err)
		}

		// Should only have one entry with the latest value.
		env, err := db.GetGlobalEnv(ctx, "TEST_PK_KEY")
		if err != nil {
			t.Fatalf("GetGlobalEnv() failed: %v", err)
		}
		if env.Value != "second-value" {
			t.Errorf("expected value=%q, got %q", "second-value", env.Value)
		}
		if env.Scope != "gate" {
			t.Errorf("expected scope=%q, got %q", "gate", env.Scope)
		}

		// Clean up.
		_ = db.DeleteGlobalEnv(ctx, "TEST_PK_KEY")
	})

	// Subtest: Scope values are stored correctly.
	t.Run("scope_values", func(t *testing.T) {
		scopes := []string{"migs", "heal", "gate", "all"}
		for _, scope := range scopes {
			key := "TEST_SCOPE_" + scope
			err := db.UpsertGlobalEnv(ctx, UpsertGlobalEnvParams{
				Key:    key,
				Value:  "test-value",
				Scope:  scope,
				Secret: false,
			})
			if err != nil {
				t.Fatalf("UpsertGlobalEnv() with scope=%q failed: %v", scope, err)
			}

			env, err := db.GetGlobalEnv(ctx, key)
			if err != nil {
				t.Fatalf("GetGlobalEnv() with scope=%q failed: %v", scope, err)
			}
			if env.Scope != scope {
				t.Errorf("expected scope=%q, got %q", scope, env.Scope)
			}

			// Clean up.
			_ = db.DeleteGlobalEnv(ctx, key)
		}
	})

	// Clean up remaining test keys.
	for _, key := range testKeys {
		_ = db.DeleteGlobalEnv(ctx, key)
	}
}
