package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitexec"
	"github.com/iw2rmb/ploy/internal/server/snapshot"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type replayStore struct {
	*handlerStore
	diffs  []store.Diff
	source string
}

func (s *replayStore) ListDiffsByRun(context.Context, types.RunID) ([]store.Diff, error) {
	return s.diffs, nil
}
func (s *replayStore) GetRunSnapshotMetadata(context.Context, types.RunID) (store.GetRunSnapshotMetadataRow, error) {
	return store.GetRunSnapshotMetadataRow{SourceCommitSha: s.source}, nil
}

type replaySnapshot struct{ workspace string }

func (s replaySnapshot) WriteTarGz(context.Context, snapshot.Metadata, io.Writer) error {
	panic("unused")
}
func (s replaySnapshot) WithWorkspace(_ context.Context, _ snapshot.Metadata, use func(string) error) error {
	return use(s.workspace)
}

// Replay preserves earlier-attempt commits, replaces failed uploads, and emits
// an applicable source-to-result patch including binaries and reversions.
func TestRunDiffs_ReplayRetainedAttempt(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		name := "verified result applies to source"
		if mismatch {
			name = "wrong result is rejected"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			workspace := t.TempDir()
			gitrepo.Init(t, workspace)
			gitrepo.WriteFile(t, filepath.Join(workspace, "pom.xml"), "original\n")
			gitrepo.WriteFile(t, filepath.Join(workspace, "remove.txt"), "delete me\n")
			gitrepo.CommitAll(t, workspace, "source")
			source := strings.TrimSpace(string(gitrepo.Run(t, workspace, "rev-parse", "HEAD")))
			runID := types.NewRunID()
			firstID := types.NewJobID()
			targetID := types.NewJobID()
			bs := bsmock.New()
			var diffs []store.Diff
			addDiff := func(job types.JobID, code int) store.Diff {
				patch := gitrepo.Run(t, workspace, "diff", "--binary", "HEAD")
				id := uuid.New()
				key := id.String()
				d := store.Diff{ID: pgtype.UUID{Bytes: id, Valid: true}, JobID: &job, RunID: runID, ObjectKey: &key, Summary: types.NewDiffSummaryBuilder().ExitCode(code).MustBuild()}
				_, err := bs.Put(ctx, key, "application/gzip", gzipTestBytes(t, patch))
				if err != nil {
					t.Fatal(err)
				}
				diffs = append(diffs, d)
				return d
			}
			gitrepo.WriteFile(t, filepath.Join(workspace, "pom.xml"), "intermediate\n")
			first := addDiff(firstID, 0)
			gitrepo.CommitAll(t, workspace, "first attempt")
			firstTree := strings.TrimSpace(string(gitrepo.Run(t, workspace, "rev-parse", "HEAD^{tree}")))
			firstSHA := gitexec.RepositorySHA(firstTree, source)
			gitrepo.WriteFile(t, filepath.Join(workspace, "pom.xml"), "failed edit\n")
			addDiff(targetID, 1)
			gitrepo.WriteFile(t, filepath.Join(workspace, "pom.xml"), "original\n")
			gitrepo.WriteFile(t, filepath.Join(workspace, "binary.dat"), "\x00\x01binary\xff")
			if err := os.Remove(filepath.Join(workspace, "remove.txt")); err != nil {
				t.Fatal(err)
			}
			gitrepo.Run(t, workspace, "add", "-A")
			targetDiff := addDiff(targetID, 0)
			tree := strings.TrimSpace(string(gitrepo.Run(t, workspace, "write-tree")))
			out := gitexec.RepositorySHA(tree, firstSHA)
			if mismatch {
				out = strings.Repeat("f", 40)
			}
			target := store.Job{ID: targetID, RunID: runID, Attempt: 2, Status: types.JobStatusSuccess, RepoShaIn: firstSHA, RepoShaOut: out}
			st := &replayStore{handlerStore: &handlerStore{}, diffs: diffs, source: source}
			st.getRun.val = store.Run{ID: runID, Attempt: 2}
			st.listJobsByRun.val = []store.Job{{ID: firstID, Attempt: 1, Status: types.JobStatusSuccess, RepoShaIn: source, RepoShaOut: firstSHA}, target}
			st.getLatestDiffByJobByID = map[types.JobID]store.Diff{firstID: first, targetID: targetDiff}
			gitrepo.Run(t, workspace, "reset", "--hard", source)
			gitrepo.Run(t, workspace, "clean", "-fd")
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/v1/runs/"+runID.String()+"/diffs?download=true&accumulated=true&diff_id="+uuid.UUID(targetDiff.ID.Bytes).String(), nil)
			req.SetPathValue("run_id", runID.String())
			listRunDiffsHandler(st, bs, replaySnapshot{workspace}).ServeHTTP(rr, req)
			if mismatch {
				assertStatus(t, rr, http.StatusConflict)
				return
			}
			assertStatus(t, rr, http.StatusOK)
			patch := gunzipTestBytes(t, rr.Body.Bytes())
			if bytes.Contains(patch, []byte("pom.xml")) {
				t.Fatalf("reverted change remains in patch: %s", patch)
			}
			gitrepo.Run(t, workspace, "read-tree", source)
			result, err := gitexec.Execute(ctx, gitexec.Request{Dir: workspace, Args: []string{"apply", "--cached", "-"}, Stdin: patch})
			if err != nil {
				t.Fatalf("apply: %v %s", err, result.Stderr)
			}
			got := strings.TrimSpace(string(gitrepo.Run(t, workspace, "write-tree")))
			if got != tree {
				t.Fatalf("tree=%s want %s", got, tree)
			}
		})
	}
}

// Fresh attempts exclude previous commits; a selected failed patch is applied
// once after the committed prefix, while earlier failed uploads are omitted.
func TestSelectReplayDiffs(t *testing.T) {
	source := strings.Repeat("a", 40)
	oldID, headID, newID := types.NewJobID(), types.NewJobID(), types.NewJobID()
	diff := func(job types.JobID, code int) store.Diff {
		return store.Diff{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, JobID: &job, Summary: types.NewDiffSummaryBuilder().ExitCode(code).MustBuild()}
	}
	old, failed, target := diff(oldID, 0), diff(newID, 1), diff(newID, 1)
	for _, fresh := range []bool{false, true} {
		name := "retained attempt includes prefix"
		if fresh {
			name = "fresh attempt excludes prefix"
		}
		t.Run(name, func(t *testing.T) {
			headOut := strings.Repeat("b", 40)
			if fresh {
				headOut = source
			}
			jobs := []store.Job{{ID: oldID, Attempt: 1, Status: types.JobStatusSuccess}, {ID: headID, Attempt: 2, JobType: types.JobTypePreGate, Status: types.JobStatusSuccess, RepoShaOut: headOut}, {ID: newID, Attempt: 2, Status: types.JobStatusFail}}
			got, _, err := selectReplayDiffs(jobs, []store.Diff{old, failed, target}, target, source)
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if fresh {
				want = 1
			}
			if len(got) != want || got[len(got)-1].ID != target.ID {
				t.Fatalf("selected %v", got)
			}
		})
	}
}

// A no-op attempt still exports inherited changes; a fresh source discards them.
func TestRunDiffs_NoOpAttempt(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		name := "retained workspace"
		if fresh {
			name = "fresh workspace"
		}
		t.Run(name, func(t *testing.T) {
			source := strings.Repeat("a", 40)
			runID := types.NewRunID()
			oldID, headID := types.NewJobID(), types.NewJobID()
			headOut := strings.Repeat("b", 40)
			if fresh {
				headOut = source
			}
			diff := store.Diff{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, JobID: &oldID, Summary: types.NewDiffSummaryBuilder().ExitCode(0).MustBuild()}
			st := &handlerStore{}
			st.listJobsByRun.val = []store.Job{{ID: oldID, Attempt: 1, Status: types.JobStatusSuccess}, {ID: headID, Attempt: 2, JobType: types.JobTypePreGate, Status: types.JobStatusSuccess, RepoShaOut: headOut}}
			st.getLatestDiffByJobByID = map[types.JobID]store.Diff{oldID: diff}
			st.getLatestDiffByJobError = pgx.ErrNoRows
			rows, err := listEffectiveRunDiffs(context.Background(), st, store.Run{ID: runID, Attempt: 2, SourceCommitSha: source})
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if fresh {
				want = 0
			}
			if len(rows) != want {
				t.Fatalf("diff count=%d want %d", len(rows), want)
			}
		})
	}
}
