package store

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestRejectsInvalidJSONBPayloads verifies that invalid JSON does not reach the store.
// This test covers all JSONB columns: jobs.meta, runs.stats, specs.spec, diffs.summary.
func TestRejectsInvalidJSONBPayloads(t *testing.T) {
	invalidJSON := []byte(`{invalid json`)
	validJSON := []byte(`{"valid": true}`)
	emptyJSON := []byte{}

	// Tests run against the validateJSONB helper directly since we don't need
	// a database connection to verify the validation logic. The PgStore wrappers
	// call validateJSONB before delegating to the underlying Queries methods.

	t.Run("validateJSONB/invalid", func(t *testing.T) {
		err := validateJSONB(invalidJSON)
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
		if !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("expected ErrInvalidJSON, got %v", err)
		}
	})

	t.Run("validateJSONB/valid", func(t *testing.T) {
		err := validateJSONB(validJSON)
		if err != nil {
			t.Fatalf("unexpected error for valid JSON: %v", err)
		}
	})

	t.Run("validateJSONB/empty", func(t *testing.T) {
		err := validateJSONB(emptyJSON)
		if err != nil {
			t.Fatalf("unexpected error for empty JSON: %v", err)
		}
	})

	t.Run("validateJSONB/nil", func(t *testing.T) {
		err := validateJSONB(nil)
		if err != nil {
			t.Fatalf("unexpected error for nil JSON: %v", err)
		}
	})

	// Test wrapper methods using a nil-pool PgStore.
	// The validation runs before the database call, so we can verify
	// validation behavior without a real connection.
	store := &PgStore{pool: nil, Queries: nil}
	ctx := context.Background()

	t.Run("CreateJob/invalid_meta", func(t *testing.T) {
		_, err := store.CreateJob(ctx, CreateJobParams{
			ID:     types.NewJobID(),
			RunID:  types.NewRunID(),
			RepoID: types.NewRepoID(),
			NextID: nil,
			Meta:   invalidJSON,
		})
		if err == nil {
			t.Fatal("expected error for invalid jobs.meta, got nil")
		}
		if !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("expected ErrInvalidJSON, got %v", err)
		}
	})

	t.Run("CreateSpec/invalid_spec", func(t *testing.T) {
		_, err := store.CreateSpec(ctx, CreateSpecParams{
			ID:   types.NewSpecID(),
			Spec: invalidJSON,
		})
		if err == nil {
			t.Fatal("expected error for invalid specs.spec, got nil")
		}
		if !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("expected ErrInvalidJSON, got %v", err)
		}
	})

	t.Run("CreateDiff/invalid_summary", func(t *testing.T) {
		_, err := store.CreateDiff(ctx, CreateDiffParams{
			RunID:   types.NewRunID(),
			Summary: invalidJSON,
		})
		if err == nil {
			t.Fatal("expected error for invalid diffs.summary, got nil")
		}
		if !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("expected ErrInvalidJSON, got %v", err)
		}
	})

	t.Run("UpdateJobMeta/invalid_meta", func(t *testing.T) {
		err := store.UpdateJobMeta(ctx, UpdateJobMetaParams{
			ID:   types.NewJobID(),
			Meta: invalidJSON,
		})
		if err == nil {
			t.Fatal("expected error for invalid jobs.meta, got nil")
		}
		if !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("expected ErrInvalidJSON, got %v", err)
		}
	})

	t.Run("UpdateJobCompletionWithMeta/invalid_meta", func(t *testing.T) {
		err := store.UpdateJobCompletionWithMeta(ctx, UpdateJobCompletionWithMetaParams{
			ID:     types.NewJobID(),
			Status: JobStatusSuccess,
			Meta:   invalidJSON,
		})
		if err == nil {
			t.Fatal("expected error for invalid jobs.meta, got nil")
		}
		if !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("expected ErrInvalidJSON, got %v", err)
		}
	})

	t.Run("UpdateRunCompletion/invalid_stats", func(t *testing.T) {
		err := store.UpdateRunCompletion(ctx, UpdateRunCompletionParams{
			ID:     types.NewRunID(),
			Status: RunStatusFinished,
			Stats:  invalidJSON,
		})
		if err == nil {
			t.Fatal("expected error for invalid runs.stats, got nil")
		}
		if !errors.Is(err, ErrInvalidJSON) {
			t.Fatalf("expected ErrInvalidJSON, got %v", err)
		}
	})
}
