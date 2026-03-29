package runs

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/assertx"
)

// singleJobReport builds a minimal RunReport with one repo and one job,
// reducing boilerplate in tests that only vary job-level fields.
func singleJobReport(migName string, repoStatus domaintypes.RunRepoStatus, job RunJobEntry) RunReport {
	repoID := domaintypes.NewMigRepoID()
	return RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   domaintypes.NewMigID(),
		MigName: migName,
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/" + migName + ".git",
				BaseRef:   "main",
				TargetRef: "ploy/" + migName,
				Status:    repoStatus,
				Attempt:   1,
				Jobs:      []RunJobEntry{job},
			},
		},
	}
}

func renderText(t *testing.T, report RunReport, opts TextRenderOptions) string {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderRunReportText(&buf, report, opts); err != nil {
		t.Fatalf("RenderRunReportText error: %v", err)
	}
	return buf.String()
}

func TestRenderRunReportTextHeadersAndArtifacts(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	preGateID := domaintypes.NewJobID()
	migJobID := domaintypes.NewJobID()

	report := RunReport{
		RunID:   runID,
		MigID:   migID,
		MigName: "java17-upgrade",
		SpecID:  specID,
		Repos: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/service.git",
				BaseRef:     "main",
				TargetRef:   "ploy/java17",
				Attempt:     1,
				Status:      "Running",
				BuildLogURL: "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs",
				Jobs: []RunJobEntry{
					{
						JobID:      preGateID,
						JobType:    "pre_gate",
						JobImage:   "ghcr.io/acme/pre-gate:1",
						Status:     "Running",
						DurationMs: 2450,
					},
					{
						JobID:      migJobID,
						JobType:    "mig",
						JobImage:   "ghcr.io/acme/mig:1",
						Status:     "Success",
						DurationMs: 3000,
						PatchURL:   "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs?download=true&diff_id=abc",
					},
				},
			},
		},
	}

	baseURL, err := url.Parse("https://example.test")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	out := renderText(t, report, TextRenderOptions{EnableOSC8: false, BaseURL: baseURL})
	assertx.Contains(t, out, "   Mig:   "+migID.String()+"   | java17-upgrade")
	assertx.Contains(t, out, "   Spec:  "+specID.String()+" | Download (https://example.test/v1/migs/"+migID.String()+"/specs/latest)")
	assertx.Contains(t, out, "   Repos: 1")
	assertx.Contains(t, out, "\n   Repos: 1\n   Run:   "+runID.String()+"\n\n")
	assertx.Contains(t, out, "   [1/1] github.com/acme/service (https://github.com/acme/service.git) main -> ploy/java17")
	assertx.Contains(t, out, "Artefacts")
	assertx.NotContains(t, out, "State")
	if strings.Count(out, "Logs (https://example.test/v1/runs/") != 1 {
		t.Fatalf("expected exactly one logs link in output, got: %q", out)
	}
	if strings.Count(out, "Patch (https://example.test/v1/runs/") != 1 {
		t.Fatalf("expected exactly one patch link in output, got: %q", out)
	}
	assertx.Contains(t, out, "Logs (https://example.test/v1/runs/")
	assertx.Contains(t, out, " | Patch (https://example.test/v1/runs/")
	assertx.Contains(t, out, "⣾")
}

func TestRenderRunReportTextExitOneLiners(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()
	repoID := domaintypes.NewMigRepoID()
	preGateID := domaintypes.NewJobID()
	healID := domaintypes.NewJobID()
	errText := "compile\nfailed at step 2"
	failCode := int32(137)
	healCode := int32(0)

	report := RunReport{
		RunID:   runID,
		MigID:   migID,
		MigName: "healing-run",
		SpecID:  specID,
		Repos: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/heal.git",
				BaseRef:     "main",
				TargetRef:   "ploy/heal",
				Attempt:     1,
				Status:      "Fail",
				LastError:   &errText,
				BuildLogURL: "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs",
				Jobs: []RunJobEntry{
					{
						JobID:      preGateID,
						JobType:    "pre_gate",
						JobImage:   "ghcr.io/acme/pre-gate:1",
						Status:     "Failed",
						ExitCode:   &failCode,
						DurationMs: 1500,
						Recovery:   &RunJobRecovery{ErrorKind: "infra"},
					},
					{
						JobID:         healID,
						JobType:       "heal",
						JobImage:      "ghcr.io/acme/heal:1",
						Status:        "Success",
						ExitCode:      &healCode,
						DurationMs:    1200,
						ActionSummary: "Applied import fix and retried build",
					},
				},
			},
		},
	}

	out := renderText(t, report, TextRenderOptions{EnableOSC8: false})
	assertx.Contains(t, out, "\x1b[91m✗\x1b[0m")
	assertx.Contains(t, out, "pre_gate")
	assertx.Contains(t, out, "└  Exit 137: \x1b[91minfra compile failed at step 2\x1b[0m")
	assertx.NotContains(t, out, "<infra>")
	assertx.Contains(t, out, "✓")
	assertx.Contains(t, out, "Heal")
	assertx.Contains(t, out, "└  Exit 0: Applied import fix and retried build")
}

func TestRenderRunReportTextExitOneLinerVariants(t *testing.T) {
	t.Parallel()

	failCode := int32(1)
	failCode42 := int32(42)
	longSummary := strings.Repeat("x", 210)

	prefix42 := "└  Exit 42: "
	indent42 := strings.Repeat(" ", len(prefix42))
	wrappedExpected := prefix42 + "\x1b[91m" + strings.Repeat("x", 100) + "\x1b[0m\n" +
		indent42 + "\x1b[91m" + strings.Repeat("x", 100) + "\x1b[0m\n" +
		indent42 + "\x1b[91m" + strings.Repeat("x", 10) + "\x1b[0m"

	tests := []struct {
		name       string
		job        RunJobEntry
		contains   []string
		notContain []string
	}{
		{
			name: "prefers bug summary over error",
			job: RunJobEntry{
				JobID:      domaintypes.NewJobID(),
				JobType:    "pre_gate",
				JobImage:   "ghcr.io/acme/pre-gate:1",
				Status:     "Failed",
				ExitCode:   &failCode,
				DurationMs: 750,
				BugSummary: "missing ; in Foo.java",
				Recovery:   &RunJobRecovery{ErrorKind: "code"},
			},
			contains:   []string{"└  Exit 1: \x1b[91mcode missing ; in Foo.java\x1b[0m", "0.8s"},
			notContain: []string{"<code>"},
		},
		{
			name: "defaults unknown error kind for re_gate",
			job: RunJobEntry{
				JobID:      domaintypes.NewJobID(),
				JobType:    "re_gate",
				JobImage:   "ghcr.io/acme/re-gate:1",
				Status:     "Failed",
				ExitCode:   &failCode,
				DurationMs: 1000,
				BugSummary: "re-gate failed",
			},
			contains:   []string{"└  Exit 1: \x1b[91munknown re-gate failed\x1b[0m"},
			notContain: []string{"<unknown>"},
		},
		{
			name: "wraps at 100 symbols",
			job: RunJobEntry{
				JobID:      domaintypes.NewJobID(),
				JobType:    "mig",
				JobImage:   "ghcr.io/acme/mig:1",
				Status:     "Failed",
				ExitCode:   &failCode42,
				DurationMs: 900,
				BugSummary: longSummary,
			},
			contains: []string{wrappedExpected},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			report := singleJobReport("exit-variant", "Fail", tc.job)
			out := renderText(t, report, TextRenderOptions{EnableOSC8: false})
			for _, needle := range tc.contains {
				assertx.Contains(t, out, needle)
			}
			for _, needle := range tc.notContain {
				assertx.NotContains(t, out, needle)
			}
		})
	}
}

func TestRenderRunReportTextOSC8OnAndOff(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	repoID := domaintypes.NewMigRepoID()
	logURL := "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs"
	patchURL := "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/diffs?download=true&diff_id=abc"
	baseURL, err := url.Parse("https://example.test")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}

	report := RunReport{
		RunID:   runID,
		MigID:   migID,
		MigName: "links-run",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/links.git",
				BaseRef:     "main",
				TargetRef:   "ploy/links",
				Status:      "Success",
				Attempt:     1,
				BuildLogURL: logURL,
				PatchURL:    patchURL,
				Jobs: []RunJobEntry{
					{
						JobID:       domaintypes.NewJobID(),
						JobType:     "mig",
						JobImage:    "ghcr.io/acme/mig:1",
						Status:      "Success",
						DurationMs:  1000,
						BuildLogURL: logURL,
						PatchURL:    patchURL,
					},
				},
			},
		},
	}

	plainOut := renderText(t, report, TextRenderOptions{EnableOSC8: false, AuthToken: "test-token", BaseURL: baseURL})
	assertx.Contains(t, plainOut, "Logs ("+logURL+"?auth_token=test-token)")
	assertx.Contains(t, plainOut, "Download (https://example.test/v1/migs/"+migID.String()+"/specs/latest?auth_token=test-token)")
	assertx.Contains(t, plainOut, "github.com/acme/links (https://github.com/acme/links.git)")
	assertx.NotContains(t, plainOut, "https://github.com/acme/links.git?auth_token=")
	assertx.Contains(t, plainOut, "Patch (https://example.test/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs?")
	assertx.Contains(t, plainOut, "auth_token=test-token")
	assertx.Contains(t, plainOut, "diff_id=abc")
	assertx.Contains(t, plainOut, "download=true")
	if strings.Contains(plainOut, "\x1b]8;;") {
		t.Fatalf("plain output unexpectedly contains OSC8 sequence: %q", plainOut)
	}

	linkedOut := renderText(t, report, TextRenderOptions{EnableOSC8: true, AuthToken: "test-token", BaseURL: baseURL})
	assertx.Contains(t, linkedOut, "\x1b]8;;"+logURL+"?auth_token=test-token")
	assertx.Contains(t, linkedOut, "\x1b]8;;https://example.test/v1/migs/"+migID.String()+"/specs/latest?auth_token=test-token")
	assertx.Contains(t, linkedOut, "\x1b]8;;https://github.com/acme/links.git\x1b\\github.com/acme/links\x1b]8;;\x1b\\")
	assertx.NotContains(t, linkedOut, "github.com/acme/links.git?auth_token=")
	assertx.Contains(t, linkedOut, "\x1b]8;;https://example.test/v1/runs/"+runID.String()+"/repos/"+repoID.String()+"/diffs?")
	assertx.Contains(t, linkedOut, "auth_token=test-token")
}

func TestRenderRunReportTextArtifactsHiddenForCancelledJobs(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	logURL := "https://example.test/v1/runs/" + runID.String() + "/repos/" + repoID.String() + "/logs"

	report := RunReport{
		RunID:   runID,
		MigID:   domaintypes.NewMigID(),
		MigName: "cancelled-run",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:      repoID,
				RepoURL:     "https://github.com/acme/cancelled.git",
				BaseRef:     "main",
				TargetRef:   "ploy/cancelled",
				Status:      "Cancelled",
				Attempt:     1,
				BuildLogURL: logURL,
				Jobs: []RunJobEntry{
					{
						JobID:       domaintypes.NewJobID(),
						JobType:     "mig",
						JobImage:    "ghcr.io/acme/mig:1",
						Status:      "Cancelled",
						DurationMs:  0,
						BuildLogURL: logURL,
					},
				},
			},
		},
	}

	out := renderText(t, report, TextRenderOptions{EnableOSC8: false})
	assertx.NotContains(t, out, "Logs (")
}

func TestRenderRunReportTextMigHeaderOnlyIDWhenNameMatches(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	report := RunReport{
		RunID:   domaintypes.NewRunID(),
		MigID:   migID,
		MigName: migID.String(),
		SpecID:  domaintypes.NewSpecID(),
		Repos:   []RunEntry{},
	}

	out := renderText(t, report, TextRenderOptions{})
	assertx.Contains(t, out, "   Mig:   "+migID.String()+"\n")
	firstLine := strings.SplitN(out, "\n", 2)[0]
	assertx.NotContains(t, firstLine, "|")
}

func TestRenderRunReportTextSpinnerFrameAndLiveDuration(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewMigRepoID()
	jobID := domaintypes.NewJobID()
	startedAt := time.Date(2026, time.February, 26, 10, 0, 0, 0, time.UTC)

	report := RunReport{
		RunID:   runID,
		MigID:   domaintypes.NewMigID(),
		MigName: "live-run",
		SpecID:  domaintypes.NewSpecID(),
		Repos: []RunEntry{
			{
				RepoID:    repoID,
				RepoURL:   "https://github.com/acme/live.git",
				BaseRef:   "main",
				TargetRef: "ploy/live",
				Status:    "Running",
				Attempt:   1,
				Jobs: []RunJobEntry{
					{
						JobID:      jobID,
						JobType:    "mig",
						JobImage:   "ghcr.io/acme/mig:live",
						Status:     "Running",
						StartedAt:  &startedAt,
						DurationMs: 0,
					},
				},
			},
		},
	}

	now := time.Date(2026, time.February, 26, 10, 0, 5, 0, time.UTC)

	tests := []struct {
		name    string
		frame   int
		glyph   string
	}{
		{"frame 0", 0, "⣾"},
		{"frame 1", 1, "⣷"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := renderText(t, report, TextRenderOptions{
				EnableOSC8:    false,
				SpinnerFrame:  tc.frame,
				LiveDurations: true,
				Now:           now,
			})
			assertx.Contains(t, out, tc.glyph)
			assertx.Contains(t, out, "5.0s")
		})
	}
}
