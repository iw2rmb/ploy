package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Drain and claim must serialize before selecting work, in either order.
func TestDrainSerializesWithClaim(t *testing.T) {
	for _, drainFirst := range []bool{true, false} {
		name := "claim_first"
		if drainFirst {
			name = "drain_first"
		}
		t.Run(name, func(t *testing.T) {
			ctx, db := newTestStore(t)
			fx := newV1Fixture(t, ctx, db, "https://example.com/drain", "main", []byte(`{}`))
			createTestJob(t, ctx, db, fx, "job")
			node := createTestNode(t, ctx, db)
			first, err := db.Pool().Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Rollback(ctx)
			second, err := db.Pool().Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Rollback(ctx)
			var pid int32
			if err := second.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				t.Fatal(err)
			}
			if drainFirst {
				_, err = New(first).UpdateNodeDrained(ctx, UpdateNodeDrainedParams{ID: node.ID, Drained: true})
			} else {
				_, err = New(first).ClaimJob(ctx, node.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				if drainFirst {
					_, err := New(second).ClaimJob(ctx, node.ID)
					done <- err
				} else {
					_, err := New(second).UpdateNodeDrained(ctx, UpdateNodeDrainedParams{ID: node.ID, Drained: true})
					done <- err
				}
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				var blocked bool
				if err := db.Pool().QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock')", pid).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked {
					break
				}
				select {
				case err := <-done:
					t.Fatalf("operation did not wait for node row: %v", err)
				default:
				}
				if time.Now().After(deadline) {
					t.Fatal("operation did not reach row lock")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := first.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-done
			if drainFirst && !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("claim after drain: %v", err)
			}
			if !drainFirst && err != nil {
				t.Fatal(err)
			}
			if err := second.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ClaimJob(ctx, node.ID); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("drained node claimed: %v", err)
			}
		})
	}
}

// Operator reassertions invalidate maintenance ownership; stale undrain fails.
func TestDrainRevisionPreservesOperatorDecision(t *testing.T) {
	ctx, db := newTestStore(t)
	node := createTestNode(t, ctx, db)
	acquired, err := db.UpdateNodeDrained(ctx, UpdateNodeDrainedParams{ID: node.ID, Drained: true, ExpectedRevision: &node.DrainRevision})
	if err != nil {
		t.Fatal(err)
	}
	reasserted, err := db.UpdateNodeDrained(ctx, UpdateNodeDrainedParams{ID: node.ID, Drained: true})
	if err != nil {
		t.Fatal(err)
	}
	if reasserted.DrainRevision != acquired.DrainRevision+1 {
		t.Fatal("revision did not advance")
	}
	if _, err := db.UpdateNodeDrained(ctx, UpdateNodeDrainedParams{ID: node.ID, Drained: false, ExpectedRevision: &acquired.DrainRevision}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale undrain: %v", err)
	}
	released, err := db.UpdateNodeDrained(ctx, UpdateNodeDrainedParams{ID: node.ID, Drained: false, ExpectedRevision: &reasserted.DrainRevision})
	if err != nil || released.Drained {
		t.Fatalf("owned undrain: %v", err)
	}
}

// Maintenance on one node must not serialize claims on another node.
func TestDrainingNodeDoesNotBlockOtherNodes(t *testing.T) {
	ctx, db := newTestStore(t)
	first, second := createTestNode(t, ctx, db), createTestNode(t, ctx, db)
	fx := newV1Fixture(t, ctx, db, "https://example.com/other", "main", []byte(`{}`))
	createTestJob(t, ctx, db, fx, "job")
	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := New(tx).UpdateNodeDrained(ctx, UpdateNodeDrainedParams{ID: first.ID, Drained: true}); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := db.ClaimJob(bounded, second.ID); err != nil {
		t.Fatal(err)
	}
}
