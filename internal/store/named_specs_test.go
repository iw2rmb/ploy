package store

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestGitSpecSnapshotIdentityIncludesSourcePath(t *testing.T) {
	ctx, db := newTestStore(t)
	sha := "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	create := func(path string) Spec {
		t.Helper()
		source := mustGitSpecSnapshotSourceJSON(t, path)
		created, err := db.CreateGitSpecSnapshot(ctx, CreateGitSpecSnapshotParams{
			ID: types.NewSpecID(), Name: "upgrade", Source: source, Sha: sha,
			SourceCommittedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			Spec:              []byte(`{"apiVersion":"ploy.mig/v1alpha1","name":"upgrade","steps":[{"image":"img"}]}`),
		})
		if err != nil {
			t.Fatalf("CreateGitSpecSnapshot(%s): %v", path, err)
		}
		return created
	}

	first := create("one.yaml")
	second := create("nested/two.yaml")
	if first.ID == second.ID {
		t.Fatal("different source paths reused one snapshot row")
	}
	fetched, err := db.GetGitSpecSnapshot(ctx, GetGitSpecSnapshotParams{
		Name: "upgrade", Domain: "git.example.com", Repo: "team/specs", Path: "nested/two.yaml", Sha: sha, Spec: second.Spec,
	})
	if err != nil {
		t.Fatalf("GetGitSpecSnapshot: %v", err)
	}
	if fetched.ID != second.ID {
		t.Fatalf("snapshot id = %s, want %s", fetched.ID, second.ID)
	}
}

func TestGitSpecSnapshotIdentityIncludesCanonicalSpec(t *testing.T) {
	ctx, db := newTestStore(t)
	sha := "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	source := mustGitSpecSnapshotSourceJSON(t, "upgrade.yaml")
	create := func(spec []byte) Spec {
		t.Helper()
		created, err := db.CreateGitSpecSnapshot(ctx, CreateGitSpecSnapshotParams{
			ID: types.NewSpecID(), Name: "upgrade", Source: source, Sha: sha,
			SourceCommittedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, Spec: spec,
		})
		if err != nil {
			t.Fatalf("CreateGitSpecSnapshot: %v", err)
		}
		return created
	}

	first := create([]byte(`{"apiVersion":"ploy.mig/v1alpha1","name":"upgrade","steps":[{"envs":{"MODE":"one"},"image":"img"}]}`))
	second := create([]byte(`{"apiVersion":"ploy.mig/v1alpha1","name":"upgrade","steps":[{"envs":{"MODE":"two"},"image":"img"}]}`))
	if first.ID == second.ID {
		t.Fatal("different canonical specs reused one snapshot row")
	}
	for _, want := range []Spec{first, second} {
		got, err := db.GetGitSpecSnapshot(ctx, GetGitSpecSnapshotParams{
			Name: "upgrade", Domain: "git.example.com", Repo: "team/specs", Path: "upgrade.yaml", Sha: sha, Spec: want.Spec,
		})
		if err != nil {
			t.Fatalf("GetGitSpecSnapshot: %v", err)
		}
		if got.ID != want.ID {
			t.Fatalf("snapshot id = %s, want %s", got.ID, want.ID)
		}
	}
}

func TestGitSpecSnapshotConstraints(t *testing.T) {
	ctx, db := newTestStore(t)
	tests := []struct {
		name string
		arg  CreateGitSpecSnapshotParams
	}{
		{name: "bad sha", arg: CreateGitSpecSnapshotParams{Name: "upgrade-java", Source: mustGitSpecSnapshotSourceJSON(t, "upgrade.yaml"), Sha: "ABC", SourceCommittedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, Spec: []byte(`{}`)}},
		{name: "missing source", arg: CreateGitSpecSnapshotParams{Name: "upgrade-java", Source: []byte(`{}`), Sha: "5555555555555555555555555555555555555555", SourceCommittedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true}, Spec: []byte(`{}`)}},
		{name: "missing committed at", arg: CreateGitSpecSnapshotParams{Name: "upgrade-java", Source: mustGitSpecSnapshotSourceJSON(t, "upgrade.yaml"), Sha: "6666666666666666666666666666666666666666", Spec: []byte(`{}`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.arg.ID = types.NewSpecID()
			_, err := db.CreateGitSpecSnapshot(ctx, tt.arg)
			if err == nil {
				t.Fatal("expected constraint violation")
			}
		})
	}

	_, err := db.CreateSpec(ctx, CreateSpecParams{ID: types.NewSpecID(), Name: "label-only", Spec: []byte(`{}`)})
	if err != nil {
		t.Fatalf("CreateSpec() with name and empty sha should remain valid: %v", err)
	}
	_, err = db.GetGitSpecSnapshot(ctx, GetGitSpecSnapshotParams{
		Name: "missing", Domain: "git.example.com", Repo: "team/specs", Path: "missing.yaml",
		Sha: "abcdefabcdefabcdefabcdefabcdefabcdefabcd", Spec: []byte(`{}`),
	})
	if err != pgx.ErrNoRows {
		t.Fatalf("missing snapshot lookup err = %v, want pgx.ErrNoRows", err)
	}
}

func mustGitSpecSnapshotSourceJSON(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]string{
		"domain": "git.example.com", "repo": "team/specs", "url": "https://git.example.com/team/specs", "path": path,
	})
	if err != nil {
		t.Fatalf("marshal source: %v", err)
	}
	return raw
}
