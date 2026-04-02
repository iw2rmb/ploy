package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func openStoreForSpecBundleTests(t *testing.T) (context.Context, Store) {
	t.Helper()
	dsn := os.Getenv("PLOY_TEST_DB_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_DB_DSN not set; skipping store integration test")
	}
	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	if err := RunMigrations(ctx, db.Pool()); err != nil {
		t.Fatalf("RunMigrations() failed: %v", err)
	}
	t.Cleanup(db.Close)
	return ctx, db
}

func TestSpecBundle_CreateAndGet(t *testing.T) {
	ctx, db := openStoreForSpecBundleTests(t)

	createdBy := "test-user"
	id := types.NewSpecBundleID()

	bundle, err := db.CreateSpecBundle(ctx, CreateSpecBundleParams{
		ID:        id,
		Cid:       "bafybeiczsscdsbs7ffqz55asqdf3smv6klcw3gofszvwlyarci47bgf354",
		Digest:    "sha256:abc123def456",
		Size:      1024,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpecBundle() failed: %v", err)
	}

	if bundle.ID != id {
		t.Fatalf("bundle.ID=%q, want %q", bundle.ID, id)
	}
	if bundle.Cid != "bafybeiczsscdsbs7ffqz55asqdf3smv6klcw3gofszvwlyarci47bgf354" {
		t.Fatalf("bundle.Cid=%q unexpected", bundle.Cid)
	}
	if bundle.Digest != "sha256:abc123def456" {
		t.Fatalf("bundle.Digest=%q unexpected", bundle.Digest)
	}
	if bundle.Size != 1024 {
		t.Fatalf("bundle.Size=%d, want 1024", bundle.Size)
	}
	if bundle.ObjectKey == nil || *bundle.ObjectKey == "" {
		t.Fatal("bundle.ObjectKey should be set by generated column")
	}
	want := "spec_bundles/" + id.String() + "/bundle.tar"
	if *bundle.ObjectKey != want {
		t.Fatalf("bundle.ObjectKey=%q, want %q", *bundle.ObjectKey, want)
	}
	if bundle.CreatedBy == nil || *bundle.CreatedBy != createdBy {
		t.Fatalf("bundle.CreatedBy=%v, want %q", bundle.CreatedBy, createdBy)
	}
	if !bundle.CreatedAt.Valid {
		t.Fatal("bundle.CreatedAt should be set")
	}
	if !bundle.LastRefAt.Valid {
		t.Fatal("bundle.LastRefAt should be set")
	}

	// GetSpecBundle round-trip.
	fetched, err := db.GetSpecBundle(ctx, id)
	if err != nil {
		t.Fatalf("GetSpecBundle() failed: %v", err)
	}
	if fetched.ID != bundle.ID {
		t.Fatalf("fetched.ID=%q, want %q", fetched.ID, bundle.ID)
	}
	if fetched.Cid != bundle.Cid {
		t.Fatalf("fetched.Cid=%q, want %q", fetched.Cid, bundle.Cid)
	}
}

func TestSpecBundle_GetByID_NotFound(t *testing.T) {
	ctx, db := openStoreForSpecBundleTests(t)

	_, err := db.GetSpecBundle(ctx, types.NewSpecBundleID())
	if err != pgx.ErrNoRows {
		t.Fatalf("GetSpecBundle() for missing id: got %v, want pgx.ErrNoRows", err)
	}
}

func TestSpecBundle_GetByCID(t *testing.T) {
	ctx, db := openStoreForSpecBundleTests(t)

	cid := "bafybeid-test-cid-" + types.NewSpecBundleID().String()

	id := types.NewSpecBundleID()
	_, err := db.CreateSpecBundle(ctx, CreateSpecBundleParams{
		ID:     id,
		Cid:    cid,
		Digest: "sha256:aabbcc",
		Size:   512,
	})
	if err != nil {
		t.Fatalf("CreateSpecBundle() failed: %v", err)
	}

	found, err := db.GetSpecBundleByCID(ctx, cid)
	if err != nil {
		t.Fatalf("GetSpecBundleByCID() failed: %v", err)
	}
	if found.ID != id {
		t.Fatalf("GetSpecBundleByCID() ID=%q, want %q", found.ID, id)
	}
}

func TestSpecBundle_GetByCID_NotFound(t *testing.T) {
	ctx, db := openStoreForSpecBundleTests(t)

	_, err := db.GetSpecBundleByCID(ctx, "nonexistent-cid-"+types.NewSpecBundleID().String())
	if err != pgx.ErrNoRows {
		t.Fatalf("GetSpecBundleByCID() for missing cid: got %v, want pgx.ErrNoRows", err)
	}
}

func TestSpecBundle_List(t *testing.T) {
	ctx, db := openStoreForSpecBundleTests(t)

	suffix := types.NewSpecBundleID().String()
	for i := range 3 {
		id := types.NewSpecBundleID()
		_, err := db.CreateSpecBundle(ctx, CreateSpecBundleParams{
			ID:     id,
			Cid:    "cid-list-" + suffix + "-" + string(rune('a'+i)),
			Digest: "sha256:list",
			Size:   int64(100 + i),
		})
		if err != nil {
			t.Fatalf("CreateSpecBundle(%d) failed: %v", i, err)
		}
	}

	bundles, err := db.ListSpecBundles(ctx, ListSpecBundlesParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("ListSpecBundles() failed: %v", err)
	}
	if len(bundles) < 3 {
		t.Fatalf("expected at least 3 bundles, got %d", len(bundles))
	}

	// Verify descending order by created_at.
	for i := 1; i < len(bundles); i++ {
		prev := bundles[i-1]
		curr := bundles[i]
		if prev.CreatedAt.Time.Before(curr.CreatedAt.Time) {
			t.Fatalf("bundles not ordered by created_at DESC at index %d", i)
		}
	}
}

func TestSpecBundle_UpdateLastRefAt(t *testing.T) {
	ctx, db := openStoreForSpecBundleTests(t)

	id := types.NewSpecBundleID()
	original, err := db.CreateSpecBundle(ctx, CreateSpecBundleParams{
		ID:     id,
		Cid:    "cid-lastref-" + id.String(),
		Digest: "sha256:lastref",
		Size:   256,
	})
	if err != nil {
		t.Fatalf("CreateSpecBundle() failed: %v", err)
	}

	// Sleep briefly to ensure last_ref_at will advance.
	time.Sleep(10 * time.Millisecond)

	if err := db.UpdateSpecBundleLastRefAt(ctx, id); err != nil {
		t.Fatalf("UpdateSpecBundleLastRefAt() failed: %v", err)
	}

	updated, err := db.GetSpecBundle(ctx, id)
	if err != nil {
		t.Fatalf("GetSpecBundle() after update failed: %v", err)
	}
	if !updated.LastRefAt.Time.After(original.LastRefAt.Time) {
		t.Fatalf("expected last_ref_at to advance: original=%v, updated=%v",
			original.LastRefAt.Time, updated.LastRefAt.Time)
	}
}

func TestSpecBundle_ListUnreferencedBefore(t *testing.T) {
	ctx, db := openStoreForSpecBundleTests(t)

	id := types.NewSpecBundleID()
	_, err := db.CreateSpecBundle(ctx, CreateSpecBundleParams{
		ID:     id,
		Cid:    "cid-gc-" + id.String(),
		Digest: "sha256:gc",
		Size:   128,
	})
	if err != nil {
		t.Fatalf("CreateSpecBundle() failed: %v", err)
	}

	// Query with a threshold well in the future: should include our new bundle.
	future := pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true}
	stale, err := db.ListSpecBundlesUnreferencedBefore(ctx, future)
	if err != nil {
		t.Fatalf("ListSpecBundlesUnreferencedBefore() failed: %v", err)
	}
	found := false
	for _, b := range stale {
		if b.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected new bundle to appear in unreferenced list with future threshold")
	}

	// After updating last_ref_at, a past threshold must exclude it.
	if err := db.UpdateSpecBundleLastRefAt(ctx, id); err != nil {
		t.Fatalf("UpdateSpecBundleLastRefAt() failed: %v", err)
	}
	past := pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true}
	staleAfter, err := db.ListSpecBundlesUnreferencedBefore(ctx, past)
	if err != nil {
		t.Fatalf("ListSpecBundlesUnreferencedBefore() after update failed: %v", err)
	}
	for _, b := range staleAfter {
		if b.ID == id {
			t.Fatal("expected bundle to be excluded from unreferenced list after update")
		}
	}
}

func TestSpecBundle_SizeConstraint(t *testing.T) {
	ctx, db := openStoreForSpecBundleTests(t)

	// Zero size violates CHECK (size > 0).
	_, err := db.CreateSpecBundle(ctx, CreateSpecBundleParams{
		ID:     types.NewSpecBundleID(),
		Cid:    "cid-zero-size",
		Digest: "sha256:zero",
		Size:   0,
	})
	if err == nil {
		t.Fatal("expected error for size=0 (violates CHECK constraint), got nil")
	}
}
