package follow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/runs"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

func TestEngine_refreshRepos_DecodesRunRepoResponseShape(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method=%s, want GET", r.Method)
		}
		if want := "/v1/runs/" + runID.String() + "/repos"; r.URL.Path != want {
			t.Fatalf("path=%s, want %s", r.URL.Path, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "repos": [
    {
      "run_id": "` + runID.String() + `",
      "repo_id": "` + repoID.String() + `",
      "repo_url": "https://example.com/org/repo.git",
      "base_ref": "main",
      "target_ref": "feature",
      "status": "Queued",
      "attempt": 1,
      "last_error": null,
      "created_at": "2026-01-15T00:00:00Z",
      "started_at": null,
      "finished_at": null
    }
  ]
}`))
	}))
	t.Cleanup(srv.Close)

	baseURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	e := NewEngine(srv.Client(), baseURL, runID, Config{MaxRetries: 1})
	if err := e.refreshRepos(context.Background()); err != nil {
		t.Fatalf("refreshRepos error: %v", err)
	}

	if len(e.repoOrder) != 1 || e.repoOrder[0] != repoID {
		t.Fatalf("repoOrder=%v, want [%s]", e.repoOrder, repoID.String())
	}
	if got := e.repoURLs[repoID]; !strings.Contains(got, "example.com") {
		t.Fatalf("repoURLs[%s]=%q, want to contain example.com", repoID.String(), got)
	}
}

func TestEngine_render_UsesStepAndNodeColumns(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())

	var out strings.Builder
	e := NewEngine(nil, &url.URL{}, runID, Config{Output: &out})
	e.repoOrder = []domaintypes.MigRepoID{repoID}
	e.repoURLs[repoID] = "example.com/org/repo"
	started := time.Now().Add(-1500 * time.Millisecond).UTC()
	e.repoJobs[repoID] = []runs.RepoJobEntry{{
		JobID:       jobID,
		Name:        "mod-0",
		JobType:     "mod",
		JobImage:    "ubuntu:latest",
		NodeID:      &nodeID,
		Status:      store.JobStatusRunning,
		StartedAt:   &started,
		DisplayName: "human-label-should-not-render",
	}}

	e.render()

	s := stripANSI(out.String())
	if strings.Contains(s, "human-label-should-not-render") {
		t.Fatalf("render output included DisplayName, output=%q", s)
	}
	if strings.Contains(s, "mod-0") {
		t.Fatalf("render output included job name, output=%q", s)
	}
	if !strings.Contains(s, "Repos: 1") {
		t.Fatalf("render output missing repo count, output=%q", s)
	}
	if !strings.Contains(s, "Repo 1/1: example.com/org/repo") {
		t.Fatalf("render output missing repo block header, output=%q", s)
	}
	if !strings.Contains(s, "Step") || !strings.Contains(s, "Image") || !strings.Contains(s, "Node") {
		t.Fatalf("render output missing expected header, output=%q", s)
	}
	if strings.Contains(s, "Index") || strings.Contains(s, "NodeID") || strings.Contains(s, "Status") {
		t.Fatalf("render output included legacy header columns, output=%q", s)
	}
	want := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta("⣾") + `\s+mod\s+` + regexp.QuoteMeta(jobID.String()) + `\s+` + regexp.QuoteMeta(nodeID.String()) + `\s+ubuntu:latest\s+`)
	if !want.MatchString(s) {
		t.Fatalf("render output missing expected job row, output=%q", s)
	}
}

func TestStatusGlyph_RunningUsesConfiguredSpinnerFrames(t *testing.T) {
	t.Parallel()

	got := statusGlyph("running", 0)
	if got != "⣾ " {
		t.Fatalf("statusGlyph(running,0)=%q, want %q", got, "⣾ ")
	}
}

func TestEngine_render_DisplaysRepoLastError(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()

	var out strings.Builder
	e := NewEngine(nil, &url.URL{}, runID, Config{Output: &out})
	e.repoOrder = []domaintypes.MigRepoID{repoID}
	e.repoURLs[repoID] = "example.com/org/repo"
	e.repoJobs[repoID] = []runs.RepoJobEntry{{
		JobID:   jobID,
		Name:    "pre-gate",
		JobType: "pre_gate",
		Status:  store.JobStatusFail,
	}}

	// Set Stack Gate failure message
	errMsg := `Stack Gate [inbound]: mismatch
  Expected: {language: java, tool: maven, release: "17"}
  Detected: {language: java, tool: maven, release: "11"}
  Evidence:
    - pom.xml: maven.compiler.release=11`
	e.repoErrors[repoID] = &errMsg

	e.render()

	raw := out.String()
	s := stripANSI(raw)

	// Verify output contains one-line error details directly under a failed row.
	if !strings.Contains(s, "└ Stack Gate [inbound]: mismatch Expected: {language: java, tool: maven, release: \"17\"} Detected: {language: java, tool: maven, release: \"11\"} Evidence: - pom.xml: maven.compiler.release=11") {
		t.Errorf("expected output to contain Stack Gate failure, got: %q", s)
	}

	// Verify ANSI red color is applied to the error one-liner.
	if !strings.Contains(raw, "\x1b[31m") || !strings.Contains(raw, "\x1b[0m") {
		t.Errorf("expected output to contain red ANSI color for error line, got: %q", raw)
	}
}
