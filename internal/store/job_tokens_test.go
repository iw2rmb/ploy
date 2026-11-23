package store

import (
	"errors"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestJobTokenFollowsClaim(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/test/job-tokens", "main", []byte("{}"))
	createTestJob(t, ctx, db, fx, "first")
	createTestJob(t, ctx, db, fx, "second")
	node := createTestNode(t, ctx, db)
	claim := func() Job {
		t.Helper()
		job, err := db.ClaimJob(ctx, node.ID)
		if err != nil {
			t.Fatal(err)
		}
		return job
	}
	params := func(job Job, tokenID string) UpsertJobTokenParams {
		return UpsertJobTokenParams{
			JobID: job.ID, StartedAt: job.StartedAt, TokenID: tokenID, TokenHash: tokenID + "-hash",
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		}
	}
	issue := func(job Job, tokenID string) {
		t.Helper()
		if _, err := db.UpsertJobToken(ctx, params(job, tokenID)); err != nil {
			t.Fatal(err)
		}
	}
	check := func(tokenID string, want bool) {
		t.Helper()
		active, err := db.IsJobTokenActive(ctx, tokenID)
		if err != nil || active != want {
			t.Fatalf("active(%s) = %t, %v; want %t", tokenID, active, err, want)
		}
	}

	first, second := claim(), claim()
	issue(first, "first")
	issue(second, "second")
	if tokens, err := db.ListAPITokens(ctx); err != nil || len(tokens) != 0 {
		t.Fatalf("managed API token inventory contains %d tokens, %v", len(tokens), err)
	}
	check("first", true)
	check("second", true)
	unclaim := UnclaimJobParams{ID: first.ID, NodeID: node.ID, StartedAt: first.StartedAt}
	if err := db.UnclaimJob(ctx, unclaim); err != nil {
		t.Fatal(err)
	}
	check("first", false)
	current := claim()
	check("first", false)
	if err := db.UnclaimJob(ctx, unclaim); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertJobToken(ctx, params(first, "stale")); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale claim returned %v", err)
	}
	issue(current, "replacement")
	check("replacement", true)
	check("first", false)
	check("second", true)
	if err := db.RevokeAPIToken(ctx, "second"); err != nil {
		t.Fatal(err)
	}
	check("second", false)
	if err := db.UpdateJobStatus(ctx, UpdateJobStatusParams{ID: current.ID, Status: types.JobStatusCancelled}); err != nil {
		t.Fatal(err)
	}
	check("replacement", false)
}
