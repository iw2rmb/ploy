package nodeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestDiffFetcher_ListRunRepoDiffs(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method=%s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(diffListResponse{Diffs: []diffListItem{{ID: "d1", JobID: types.NewJobID()}}})
	}))
	defer server.Close()

	fetcher, err := NewDiffFetcher(Config{ServerURL: server.URL, NodeID: "aB3xY9"})
	if err != nil {
		t.Fatalf("NewDiffFetcher: %v", err)
	}

	diffs, err := fetcher.ListRunRepoDiffs(context.Background(), runID, repoID)
	if err != nil {
		t.Fatalf("ListRunRepoDiffs: %v", err)
	}
	if len(diffs) != 1 || diffs[0].ID != "d1" {
		t.Fatalf("unexpected diffs: %+v", diffs)
	}
}

func TestDiffFetcher_FetchRunRepoDiffPatch(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	payload := gzipBytes(t, []byte("patch"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("download") != "true" {
			t.Fatalf("download query missing")
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	fetcher, err := NewDiffFetcher(Config{ServerURL: server.URL, NodeID: "aB3xY9"})
	if err != nil {
		t.Fatalf("NewDiffFetcher: %v", err)
	}

	got, err := fetcher.FetchRunRepoDiffPatch(context.Background(), runID, repoID, "d1")
	if err != nil {
		t.Fatalf("FetchRunRepoDiffPatch: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("patch mismatch")
	}
}

func TestDiffFetcher_FetchDiffsForJobRepo_FilterAndOrder(t *testing.T) {
	t.Parallel()

	runID := types.NewRunID()
	repoID := types.NewMigRepoID()
	currentJobID := types.NewJobID()
	jobA := types.NewJobID()
	jobB := types.NewJobID()
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Minute)

	diffs := []diffListItem{
		{ID: "healing", JobID: jobA, CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().JobType("healing").MustBuild()},
		{ID: "later", JobID: jobB, CreatedAt: t1, Summary: types.NewDiffSummaryBuilder().JobType(types.DiffJobTypeMod.String()).MustBuild()},
		{ID: "self", JobID: currentJobID, CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().JobType(types.DiffJobTypeMod.String()).MustBuild()},
		{ID: "earlier", JobID: jobA, CreatedAt: t0, Summary: types.NewDiffSummaryBuilder().JobType(types.DiffJobTypeMod.String()).MustBuild()},
	}
	patches := map[string][]byte{
		"earlier": gzipBytes(t, []byte("p-earlier")),
		"healing": gzipBytes(t, []byte("p-healing")),
		"later":   gzipBytes(t, []byte("p-later")),
	}

	var fetched []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("download") == "true" {
			diffID := r.URL.Query().Get("diff_id")
			fetched = append(fetched, diffID)
			_, _ = w.Write(patches[diffID])
			return
		}
		_ = json.NewEncoder(w).Encode(diffListResponse{Diffs: diffs})
	}))
	defer server.Close()

	fetcher, err := NewDiffFetcher(Config{ServerURL: server.URL, NodeID: "aB3xY9"})
	if err != nil {
		t.Fatalf("NewDiffFetcher: %v", err)
	}

	got, err := fetcher.FetchDiffsForJobRepo(context.Background(), runID, repoID, currentJobID)
	if err != nil {
		t.Fatalf("FetchDiffsForJobRepo: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("patch count=%d want 3", len(got))
	}
	if len(fetched) != 3 || fetched[0] != "earlier" || fetched[1] != "healing" || fetched[2] != "later" {
		t.Fatalf("fetched order=%v", fetched)
	}
}
