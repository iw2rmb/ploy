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
	"github.com/iw2rmb/ploy/internal/testutil/workflowkit"
)

var ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string {
	return ansiCSI.ReplaceAllString(s, "")
}

func TestEngine_refreshRepos_DecodesRunRepoResponseShape(t *testing.T) {
	t.Parallel()

	s := workflowkit.NewFollowStreamScenario()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method=%s, want GET", r.Method)
		}
		if want := "/v1/runs/" + s.RunID.String() + "/repos"; r.URL.Path != want {
			t.Fatalf("path=%s, want %s", r.URL.Path, want)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "repos": [
    {
      "run_id": "` + s.RunID.String() + `",
      "repo_id": "` + s.MigRepoID.String() + `",
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

	e := NewEngine(srv.Client(), baseURL, s.RunID, Config{MaxRetries: 1})
	if err := e.refreshRepos(context.Background()); err != nil {
		t.Fatalf("refreshRepos error: %v", err)
	}

	if len(e.repoOrder) != 1 || e.repoOrder[0] != s.MigRepoID {
		t.Fatalf("repoOrder=%v, want [%s]", e.repoOrder, s.MigRepoID.String())
	}
	if got := e.repoURLs[s.MigRepoID]; !strings.Contains(got, "example.com") {
		t.Fatalf("repoURLs[%s]=%q, want to contain example.com", s.MigRepoID.String(), got)
	}
}

func TestEngine_render_UsesStepAndNodeColumns(t *testing.T) {
	t.Parallel()

	s := workflowkit.NewFollowStreamScenario()
	nodeID := domaintypes.NodeID(domaintypes.NewNodeKey())

	var out strings.Builder
	e := NewEngine(nil, &url.URL{}, s.RunID, Config{Output: &out})
	e.repoOrder = []domaintypes.MigRepoID{s.MigRepoID}
	e.repoURLs[s.MigRepoID] = "example.com/org/repo"
	started := time.Now().Add(-1500 * time.Millisecond).UTC()
	e.repoJobs[s.MigRepoID] = []runs.RepoJobEntry{{
		JobID:       s.JobID,
		Name:        "mig-0",
		JobType:     "mig",
		JobImage:    "ubuntu:latest",
		NodeID:      &nodeID,
		Status:      domaintypes.JobStatusRunning,
		StartedAt:   &started,
		DisplayName: "human-label-should-not-render",
	}}

	e.render()

	st := stripANSI(out.String())
	if strings.Contains(st, "human-label-should-not-render") {
		t.Fatalf("render output included DisplayName, output=%q", st)
	}
	if strings.Contains(st, "mig-0") {
		t.Fatalf("render output included job name, output=%q", st)
	}
	if !strings.Contains(st, "Repos: 1") {
		t.Fatalf("render output missing repo count, output=%q", st)
	}
	if !strings.Contains(st, "Repo 1/1: example.com/org/repo") {
		t.Fatalf("render output missing repo block header, output=%q", st)
	}
	if !strings.Contains(st, "Step") || !strings.Contains(st, "Image") || !strings.Contains(st, "Node") {
		t.Fatalf("render output missing expected header, output=%q", st)
	}
	if strings.Contains(st, "Index") || strings.Contains(st, "NodeID") || strings.Contains(st, "Status") {
		t.Fatalf("render output included legacy header columns, output=%q", st)
	}
	want := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta("⣾") + `\s+mig\s+` + regexp.QuoteMeta(s.JobID.String()) + `\s+` + regexp.QuoteMeta(nodeID.String()) + `\s+ubuntu:latest\s+`)
	if !want.MatchString(st) {
		t.Fatalf("render output missing expected job row, output=%q", st)
	}
}

func TestStatusGlyph_RunningUsesConfiguredSpinnerFrames(t *testing.T) {
	t.Parallel()

	got := runs.StatusGlyph("running", 0)
	if got != "⣾" {
		t.Fatalf("StatusGlyph(running,0)=%q, want %q", got, "⣾")
	}
	got = runs.StatusGlyph("running", 1)
	if got != "⣷" {
		t.Fatalf("StatusGlyph(running,1)=%q, want %q", got, "⣷")
	}
}

func TestStatusGlyph_FailedAliasUsesFailGlyph(t *testing.T) {
	t.Parallel()

	if got := runs.StatusGlyph("fail", 0); got != "✗" {
		t.Fatalf("StatusGlyph(fail,0)=%q, want %q", got, "✗")
	}
	if got := runs.StatusGlyph("failed", 0); got != "✗" {
		t.Fatalf("StatusGlyph(failed,0)=%q, want %q", got, "✗")
	}
}

// TestEngine_render_DisplaysRepoLastError covers both the canonical "fail" status and
// the "failed" alias — ensuring error one-liners are rendered for both status strings.
func TestEngine_render_DisplaysRepoLastError(t *testing.T) {
	t.Parallel()

	stackGateErrMsg := `Stack Gate [inbound]: mismatch
  Expected: {language: java, tool: maven, release: "17"}
  Detected: {language: java, tool: maven, release: "11"}
  Evidence:
    - pom.xml: maven.compiler.release=11`

	tests := []struct {
		name      string
		jobStatus domaintypes.JobStatus
		errMsg    string
		wantLine  string
		checkANSI bool
	}{
		{
			name:      "fail status/stack gate mismatch",
			jobStatus: domaintypes.JobStatusFail,
			errMsg:    stackGateErrMsg,
			wantLine:  `└ Stack Gate [inbound]: mismatch Expected: {language: java, tool: maven, release: "17"} Detected: {language: java, tool: maven, release: "11"} Evidence: - pom.xml: maven.compiler.release=11`,
			checkANSI: true,
		},
		{
			name:      "failed alias/build error",
			jobStatus: domaintypes.JobStatus("failed"),
			errMsg:    "build failed: missing config",
			wantLine:  "└ build failed: missing config",
			checkANSI: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := workflowkit.NewFollowStreamScenario()

			var out strings.Builder
			e := NewEngine(nil, &url.URL{}, s.RunID, Config{Output: &out})
			e.repoOrder = []domaintypes.MigRepoID{s.MigRepoID}
			e.repoURLs[s.MigRepoID] = "example.com/org/repo"
			e.repoJobs[s.MigRepoID] = []runs.RepoJobEntry{{
				JobID:   s.JobID,
				Name:    "pre-gate",
				JobType: "pre_gate",
				Status:  tt.jobStatus,
			}}
			e.repoErrors[s.MigRepoID] = &tt.errMsg

			e.render()

			raw := out.String()
			plain := stripANSI(raw)

			if !strings.Contains(plain, tt.wantLine) {
				t.Errorf("expected output to contain %q, got: %q", tt.wantLine, plain)
			}
			if tt.checkANSI && (!strings.Contains(raw, "\x1b[31m") || !strings.Contains(raw, "\x1b[0m")) {
				t.Errorf("expected output to contain red ANSI color for error line, got: %q", raw)
			}
		})
	}
}
