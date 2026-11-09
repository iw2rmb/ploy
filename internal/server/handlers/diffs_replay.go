package handlers

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/gitexec"
	"github.com/iw2rmb/ploy/internal/gitlabtoken"
	"github.com/iw2rmb/ploy/internal/server/snapshot"
	"github.com/iw2rmb/ploy/internal/store"
)

func downloadAccumulatedRunDiff(w http.ResponseWriter, r *http.Request, st store.Store, bs blobstore.Store, snapshots repoSnapshotWriter, run store.Run, diffID types.DiffID) bool {
	current, err := listEffectiveRunDiffs(r.Context(), st, run)
	if err != nil {
		serverError(w, "download accumulated diff", "list diffs", err)
		return false
	}
	var target *store.Diff
	for _, row := range current {
		if row.Diff.ID.Bytes == uuid.MustParse(diffID.String()) {
			d := row.Diff
			target = &d
			break
		}
	}
	if target == nil {
		writeHTTPError(w, http.StatusNotFound, "diff not found")
		return false
	}
	if snapshots == nil {
		writeHTTPError(w, http.StatusServiceUnavailable, "snapshot service unavailable")
		return false
	}
	jobs, err := st.ListJobsByRun(r.Context(), run.ID)
	if err != nil {
		serverError(w, "download accumulated diff", "list jobs", err)
		return false
	}
	diffs, err := st.ListDiffsByRun(r.Context(), run.ID)
	if err != nil {
		serverError(w, "download accumulated diff", "list history", err)
		return false
	}
	meta, err := st.GetRunSnapshotMetadata(r.Context(), run.ID)
	if err != nil {
		serverError(w, "download accumulated diff", "snapshot metadata", err)
		return false
	}
	sequence, err := selectReplayDiffs(jobs, diffs, *target, meta.SourceCommitSha)
	if err != nil {
		writeHTTPError(w, http.StatusConflict, "cannot reconstruct run patch: %v", err)
		return false
	}
	var patch []byte
	err = snapshots.WithWorkspace(r.Context(), snapshot.Metadata{
		RepoURL: types.NormalizeRepoURL(meta.RepoUrl), BaseRef: meta.RepoBaseRef,
		SourceCommitSHA: meta.SourceCommitSha, GitLabTokenHash: gitlabtoken.HashFromRunStats(meta.Stats),
	}, func(workspace string) error {
		var replayErr error
		patch, replayErr = replayRunDiffs(r.Context(), bs, workspace, meta.SourceCommitSha, sequence)
		return replayErr
	})
	if err != nil {
		writeHTTPError(w, http.StatusConflict, "cannot reconstruct run patch: %v", err)
		return false
	}
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err = zw.Write(patch); err == nil {
		err = zw.Close()
	}
	if err != nil {
		serverError(w, "download accumulated diff", "compress patch", err)
		return false
	}
	streamBlob(w, bytes.NewReader(compressed.Bytes()), int64(compressed.Len()), fmt.Sprintf("diff-%s-accumulated.patch.gz", diffID), "application/gzip")
	return true
}

// Failed executions do not advance HEAD. Their edits are included in the next
// successful patch, so replaying both would apply the same changes twice.
func selectReplayDiffs(jobs []store.Job, diffs []store.Diff, target store.Diff, source string) ([]store.Diff, error) {
	byID := make(map[types.JobID]store.Job, len(jobs))
	for _, job := range jobs {
		byID[job.ID] = job
	}
	if target.JobID == nil {
		return nil, fmt.Errorf("target diff has no job")
	}
	targetJob, ok := byID[*target.JobID]
	if !ok {
		return nil, fmt.Errorf("target job is missing")
	}
	firstAttempt := replayFirstAttempt(jobs, targetJob.Attempt, source)
	latest := make(map[types.JobID]store.Diff)
	found := false
	for _, diff := range diffs {
		if diff.ID == target.ID {
			found = true
			break
		}
		if diff.JobID == nil {
			continue
		}
		job, ok := byID[*diff.JobID]
		if !ok || job.Attempt < firstAttempt || job.Attempt > targetJob.Attempt {
			continue
		}
		code, hasCode := types.DiffSummary(diff.Summary).ExitCode()
		if job.Status == types.JobStatusSuccess && hasCode && code == 0 {
			latest[job.ID] = diff
		}
	}
	if !found {
		return nil, fmt.Errorf("target diff is missing from history")
	}
	// The target replaces earlier uploads for its job, including failed attempts.
	delete(latest, targetJob.ID)
	sequence := make([]store.Diff, 0, len(latest)+1)
	for _, diff := range diffs {
		if diff.ID == target.ID {
			break
		}
		if diff.JobID != nil && latest[*diff.JobID].ID == diff.ID {
			sequence = append(sequence, diff)
		}
	}
	return append(sequence, target), nil
}

func replayRunDiffs(ctx context.Context, bs blobstore.Store, workspace, source string, sequence []store.Diff) ([]byte, error) {
	git := func(input []byte, args ...string) ([]byte, error) {
		result, err := gitexec.Execute(ctx, gitexec.Request{Dir: workspace, Args: args, Stdin: input})
		if err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", args[0], err, bytes.TrimSpace(result.Stderr))
		}
		return bytes.TrimSpace(result.Stdout), nil
	}
	if _, err := git(nil, "read-tree", source); err != nil {
		return nil, err
	}
	remaining := maxAccumulatedDiffPlainBytes
	for _, diff := range sequence {
		if diff.ObjectKey == nil {
			return nil, fmt.Errorf("diff %s has no blob", uuid.UUID(diff.ID.Bytes))
		}
		rc, _, err := bs.Get(ctx, *diff.ObjectKey)
		if err != nil {
			return nil, fmt.Errorf("read diff %s: %w", uuid.UUID(diff.ID.Bytes), err)
		}
		zr, err := gzip.NewReader(rc)
		if err != nil {
			_ = rc.Close()
			return nil, err
		}
		patch, err := io.ReadAll(io.LimitReader(zr, remaining+1))
		_ = zr.Close()
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		remaining -= int64(len(patch))
		if remaining < 0 {
			return nil, fmt.Errorf("diff history exceeds %d bytes", maxAccumulatedDiffPlainBytes)
		}
		if len(bytes.TrimSpace(patch)) == 0 {
			continue
		}
		if _, err := git(patch, "apply", "--cached", "--whitespace=nowarn", "-"); err != nil {
			return nil, fmt.Errorf("diff %s: %w", uuid.UUID(diff.ID.Bytes), err)
		}
	}
	// Emit one patch; concatenated incremental patches are not a source-to-result diff.
	result, err := gitexec.Execute(ctx, gitexec.Request{Dir: workspace, Args: []string{"diff", "--cached", "--binary", "--full-index", source, "--"}})
	if err != nil {
		return nil, err
	}
	if int64(len(result.Stdout)) > maxAccumulatedDiffPlainBytes {
		return nil, fmt.Errorf("result patch exceeds size limit")
	}
	return result.Stdout, nil
}

// A fresh chain head reports the source SHA. Retained workspaces require the
// committed prefix from older attempts, even if the current attempt is a no-op.
func replayFirstAttempt(jobs []store.Job, attempt int32, source string) int32 {
	first := int32(1)
	if !types.IsCanonicalFullCommitSHA(source) {
		return first
	}
	for _, job := range jobs {
		if job.Attempt <= attempt && job.JobType == types.JobTypePreGate && job.Status == types.JobStatusSuccess && job.RepoShaOut == source && job.Attempt > first {
			first = job.Attempt
		}
	}
	return first
}
